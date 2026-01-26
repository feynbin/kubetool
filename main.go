package main

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/cavaliergopher/grab/v3"
)

type Config struct {
	UseProxy     bool
	ProxyURL     string
	GitHubMirror string
	httpClient   *http.Client
	grabClient   *grab.Client
}

func (c *Config) init() {
	c.httpClient = &http.Client{
		Timeout: 60 * time.Second,
	}

	if c.UseProxy && c.ProxyURL != "" {
		proxyURL, err := url.Parse(c.ProxyURL)
		if err == nil {
			c.httpClient.Transport = &http.Transport{
				Proxy: http.ProxyURL(proxyURL),
			}
		}
	}

	c.grabClient = grab.NewClient()
	c.grabClient.HTTPClient = c.httpClient
}

func (c *Config) formatURL(originalURL string) string {
	if c.GitHubMirror != "" && strings.Contains(originalURL, "https://github.com") {
		return strings.Replace(originalURL, "https://github.com", c.GitHubMirror, 1)
	}
	return originalURL
}

var components = map[string]struct {
	versionURL string
	urlPattern string
	hashURL    string
}{
	"kubeadm": {
		"https://dl.k8s.io/release/stable.txt",
		"https://dl.k8s.io/release/%s/bin/linux/amd64/kubeadm",
		"https://dl.k8s.io/release/%s/bin/linux/amd64/kubeadm.sha256",
	},
	"kubelet": {
		"https://dl.k8s.io/release/stable.txt",
		"https://dl.k8s.io/release/%s/bin/linux/amd64/kubelet",
		"https://dl.k8s.io/release/%s/bin/linux/amd64/kubelet.sha256",
	},
	"kubectl": {
		"https://dl.k8s.io/release/stable.txt",
		"https://dl.k8s.io/release/%s/bin/linux/amd64/kubectl",
		"https://dl.k8s.io/release/%s/bin/linux/amd64/kubectl.sha256",
	},
	"runc": {
		"https://api.github.com/repos/opencontainers/runc/releases/latest",
		"https://github.com/opencontainers/runc/releases/download/%s/runc.amd64",
		"https://github.com/opencontainers/runc/releases/download/%s/runc.sha256sum",
	},
	"containerd": {
		"https://api.github.com/repos/containerd/containerd/releases/latest",
		"https://github.com/containerd/containerd/releases/download/%s/containerd-%s-linux-amd64.tar.gz",
		"https://github.com/containerd/containerd/releases/download/%s/containerd-%s-linux-amd64.tar.gz.sha256sum",
	},
	"crictl": {
		"https://api.github.com/repos/kubernetes-sigs/cri-tools/releases/latest",
		"https://github.com/kubernetes-sigs/cri-tools/releases/download/%s/crictl-v%s-linux-amd64.tar.gz",
		"https://github.com/kubernetes-sigs/cri-tools/releases/download/%s/crictl-v%s-linux-amd64.tar.gz.sha256",
	},
	"cilium": {
		"https://api.github.com/repos/cilium/cilium-cli/releases/latest",
		"https://github.com/cilium/cilium-cli/releases/download/%s/cilium-linux-amd64.tar.gz",
		"https://github.com/cilium/cilium-cli/releases/download/%s/cilium-linux-amd64.tar.gz.sha256sum",
	},
	"helm": {
		"https://api.github.com/repos/helm/helm/releases/latest",
		"https://get.helm.sh/helm-%s-linux-amd64.tar.gz",
		"https://get.helm.sh/helm-%s-linux-amd64.tar.gz.sha256sum",
	},
}

func getLatestVersion(config *Config, versionURL string) (string, error) {
	versionURL = config.formatURL(versionURL)

	resp, err := config.httpClient.Get(versionURL)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if strings.Contains(versionURL, "api.github.com") {
		body, err := io.ReadAll(resp.Body)
		if err != nil {
			return "", err
		}
		version := extractVersionFromJSON(body)
		if version == "" {
			return "", fmt.Errorf("failed to extract version from GitHub API")
		}
		return version, nil
	} else {
		version, err := io.ReadAll(resp.Body)
		if err != nil {
			return "", err
		}
		verStr := strings.TrimSpace(string(version))
		return verStr, nil
	}
}

func extractVersionFromJSON(data []byte) string {
	str := string(data)
	if idx := strings.Index(str, `"tag_name":`); idx != -1 {
		start := idx + len(`"tag_name":"`)
		end := strings.Index(str[start:], `"`)
		if end != -1 {
			tag := str[start : start+end]
			return tag
		}
	}
	return ""
}

func parsePGPSignedHashFile(content string, targetFile string) (string, error) {
	scanner := bufio.NewScanner(strings.NewReader(content))
	inMessage := false
	inSignature := false

	for scanner.Scan() {
		line := scanner.Text()

		if strings.HasPrefix(line, "-----BEGIN PGP SIGNED MESSAGE-----") {
			inMessage = true
			continue
		}

		if strings.HasPrefix(line, "-----BEGIN PGP SIGNATURE-----") {
			inSignature = true
			break
		}

		if inMessage && !inSignature {
			if strings.HasPrefix(line, "Hash: ") {
				continue
			}

			if strings.TrimSpace(line) == "" {
				continue
			}

			parts := strings.Fields(line)
			if len(parts) >= 2 {
				hash := parts[0]
				filename := parts[1]

				if filename == targetFile {
					return hash, nil
				}
			}
		}
	}

	return "", fmt.Errorf("hash not found for file: %s", targetFile)
}

func getRemoteHash(config *Config, hashURL string, targetFile string) (string, error) {
	hashURL = config.formatURL(hashURL)

	resp, err := config.httpClient.Get(hashURL)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return "", fmt.Errorf("failed to fetch hash: HTTP %d from %s", resp.StatusCode, hashURL)
	}

	content, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	contentStr := string(content)

	if strings.Contains(contentStr, "-----BEGIN PGP SIGNED MESSAGE-----") {
		return parsePGPSignedHashFile(contentStr, targetFile)
	}

	hashStr := strings.TrimSpace(contentStr)
	parts := strings.Fields(hashStr)
	if len(parts) > 0 {
		return parts[0], nil
	}
	return hashStr, nil
}

func calculateFileHash(filePath string) (string, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return "", err
	}
	defer file.Close()

	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return "", err
	}

	return hex.EncodeToString(hash.Sum(nil)), nil
}

func fileExists(filePath string) bool {
	info, err := os.Stat(filePath)
	if os.IsNotExist(err) {
		return false
	}
	return !info.IsDir()
}

func getTargetFileName(name, version string) string {
	switch name {
	case "kubeadm", "kubelet", "kubectl":
		return name
	case "runc":
		return "runc.amd64"
	case "containerd":
		return fmt.Sprintf("containerd-%s-linux-amd64.tar.gz", strings.TrimPrefix(version, "v"))
	case "crictl":
		return fmt.Sprintf("crictl-v%s-linux-amd64.tar.gz", strings.TrimPrefix(version, "v"))
	case "cilium":
		return "cilium-linux-amd64.tar.gz"
	case "helm":
		return fmt.Sprintf("helm-v%s-linux-amd64.tar.gz", strings.TrimPrefix(version, "v"))
	default:
		return ""
	}
}

func getFinalFileName(name string) string {
	switch name {
	case "kubeadm", "kubelet", "kubectl", "runc", "crictl", "cilium", "helm":
		return name
	case "containerd":
		return "containerd.tar.gz"
	default:
		return name
	}
}

func downloadComponent(config *Config, name, version, binDir string) error {
	comp, exists := components[name]
	if !exists {
		return fmt.Errorf("component %s not found", name)
	}

	var downloadURL, hashURL string
	switch name {
	case "containerd":
		cleanVersion := strings.TrimPrefix(version, "v")
		downloadURL = fmt.Sprintf(comp.urlPattern, version, cleanVersion)
		hashURL = fmt.Sprintf(comp.hashURL, version, cleanVersion)
	case "crictl":
		cleanVersion := strings.TrimPrefix(version, "v")
		downloadURL = fmt.Sprintf(comp.urlPattern, version, cleanVersion)
		hashURL = fmt.Sprintf(comp.hashURL, version, cleanVersion)
	default:
		downloadURL = fmt.Sprintf(comp.urlPattern, version)
		hashURL = fmt.Sprintf(comp.hashURL, version)
	}

	downloadURL = config.formatURL(downloadURL)
	hashURL = config.formatURL(hashURL)

	targetFile := getTargetFileName(name, version)
	fmt.Printf("%s %s:\n", name, version)
	fmt.Printf("  URL: %s\n", downloadURL)

	tempDir := "./temp"
	if err := os.MkdirAll(tempDir, 0755); err != nil {
		return fmt.Errorf("failed to create temp directory: %v", err)
	}

	tempFile := filepath.Join(tempDir, filepath.Base(downloadURL))
	finalFile := filepath.Join(binDir, getFinalFileName(name))

	expectedHash, err := getRemoteHash(config, hashURL, targetFile)
	if err != nil {
		return fmt.Errorf("failed to get remote hash: %v", err)
	}

	if fileExists(finalFile) {
		localHash, err := calculateFileHash(finalFile)
		if err == nil && localHash == expectedHash {
			fmt.Printf("  ✓ Already up to date\n")
			return nil
		}
	}

	fmt.Printf("  Downloading...")
	req, err := grab.NewRequest(tempFile, downloadURL)
	if err != nil {
		return err
	}

	resp := config.grabClient.Do(req)

Loop:
	for {
		select {
		case <-resp.Done:
			break Loop
		}
	}

	if err := resp.Err(); err != nil {
		return fmt.Errorf("download failed: %v", err)
	}

	actualHash, err := calculateFileHash(tempFile)
	if err != nil {
		return fmt.Errorf("failed to calculate hash: %v", err)
	}

	if actualHash != expectedHash {
		os.Remove(tempFile)
		return fmt.Errorf("hash mismatch: expected %s, got %s", expectedHash, actualHash)
	}

	if err := os.Rename(tempFile, finalFile); err != nil {
		if err := copyFile(tempFile, finalFile); err != nil {
			return fmt.Errorf("failed to move file: %v", err)
		}
		os.Remove(tempFile)
	}

	fmt.Printf(" ✓ Downloaded\n")
	return nil
}

func copyFile(src, dst string) error {
	source, err := os.Open(src)
	if err != nil {
		return err
	}
	defer source.Close()

	destination, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer destination.Close()

	_, err = io.Copy(destination, source)
	return err
}

func main() {
	proxyFlag := flag.String("proxy", "", "Proxy URL (e.g., http://127.0.0.1:7890)")
	mirrorFlag := flag.String("mirror", "", "GitHub mirror URL (e.g., https://github.feynbin.cn)")
	flag.Parse()

	config := &Config{
		UseProxy:     *proxyFlag != "",
		ProxyURL:     *proxyFlag,
		GitHubMirror: *mirrorFlag,
	}
	config.init()

	if config.UseProxy {
		fmt.Printf("Using proxy: %s\n", config.ProxyURL)
	}
	if config.GitHubMirror != "" {
		fmt.Printf("Using GitHub mirror: %s\n", config.GitHubMirror)
	}

	binDir := "./bin"
	if err := os.MkdirAll(binDir, 0755); err != nil {
		fmt.Printf("Failed to create bin directory: %v\n", err)
		return
	}

	componentsList := []string{"kubeadm", "kubelet", "kubectl", "runc", "containerd", "crictl", "cilium", "helm"}

	for _, comp := range componentsList {
		version, err := getLatestVersion(config, components[comp].versionURL)
		if err != nil {
			fmt.Printf("Failed to get latest version for %s: %v\n", comp, err)
			continue
		}

		fmt.Printf("\n[%s]\n", strings.ToUpper(comp))
		if err := downloadComponent(config, comp, version, binDir); err != nil {
			fmt.Printf("  ✗ Error: %v\n", err)
		}
	}

	fmt.Println("\n✓ All downloads completed!")
}
