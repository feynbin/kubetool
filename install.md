#!/bin/fish

apt install curl wget unzip zip -y
swapoff -a
sed -i '/swap/s/^/#/' /etc/fstab
modprobe br_netfilter overlay
tee /etc/modules-load.d/k8s.conf <<EOF
br_netfilter
overlay
EOF
tee /etc/sysctl.d/k8s.conf <<EOF
net.bridge.bridge-nf-call-ip6tables = 1
net.bridge.bridge-nf-call-iptables = 1
net.ipv4.ip_forward = 1
EOF
sysctl --system
# 获取最新 Kubernetes 版本
set K8S_VERSION (curl -L https://dl.k8s.io/release/stable.txt)
curl -L -o kubectl "https://dl.k8s.io/release/$K8S_VERSION/bin/linux/amd64/kubectl"
curl -L -o kubeadm "https://dl.k8s.io/release/$K8S_VERSION/bin/linux/amd64/kubeadm"
curl -L -o kubelet "https://dl.k8s.io/release/$K8S_VERSION/bin/linux/amd64/kubelet"


# 获取最新 Cilium CLI 版本
set CILIUM_CLI_VERSION (curl -s https://api.github.com/repos/cilium/cilium-cli/tags | grep -oP '"name": "\K(v[0-9]+\.[0-9]+\.[0-9]+(-[a-z0-9\.]+)?)' | head -1)
curl -L -o cilium-linux-amd64.tar.gz "https://github.com/cilium/cilium-cli/releases/download/$CILIUM_CLI_VERSION/cilium-linux-amd64.tar.gz"

# 获取最新 containerd 版本
set CONTAINERD_VERSION (curl -s https://api.github.com/repos/containerd/containerd/tags | grep -oP '"name": "\K(v[0-9]+\.[0-9]+\.[0-9]+)' | head -1)
set CONTAINERD_VERSION_NO_V (string replace -r '^v' '' $CONTAINERD_VERSION)
curl -L -o containerd-linux-amd64.tar.gz "https://github.com/containerd/containerd/releases/download/$CONTAINERD_VERSION/containerd-$CONTAINERD_VERSION_NO_V-linux-amd64.tar.gz"

# 获取最新 runc 版本
set RUNC_VERSION (curl -s https://api.github.com/repos/opencontainers/runc/tags | grep -oP '"name": "\K(v[0-9]+\.[0-9]+\.[0-9]+)' | head -1)
curl -L -o runc "https://github.com/opencontainers/runc/releases/download/$RUNC_VERSION/runc.amd64"

# 获取最新 Helm 版本
set HELM_VERSION (curl -s https://api.github.com/repos/helm/helm/tags | grep -oP '"name": "\K(v[0-9]+\.[0-9]+\.[0-9]+)' | head -1)
curl -L -o helm-linux-amd64.tar.gz "https://get.helm.sh/helm-$HELM_VERSION-linux-amd64.tar.gz"

# crictl - 可以自动获取版本或使用固定版本
set CRICTL_VERSION (curl -s https://api.github.com/repos/kubernetes-sigs/cri-tools/tags | grep -oP '"name": "\K(v[0-9]+\.[0-9]+\.[0-9]+)' | head -1)
curl -L -o crictl-linux-amd64.tar.gz "https://github.com/kubernetes-sigs/cri-tools/releases/download/$CRICTL_VERSION/crictl-$CRICTL_VERSION-linux-amd64.tar.gz"

tar -xzf cilium-linux-amd64.tar.gz
tar -xzf containerd-linux-amd64.tar.gz
tar -xzf helm-linux-amd64.tar.gz
tar -xzf crictl-linux-amd64.tar.gz
chmod +x kubectl kubeadm kubelet cilium runc
test -d bin; and chmod +x bin/*
test -d linux-amd64; and chmod +x linux-amd64/helm
mv kubectl kubeadm kubelet cilium runc /usr/local/bin/
test -d bin; and mv bin/* /usr/local/bin/
test -d linux-amd64; and mv linux-amd64/helm /usr/local/bin/
test -f crictl; and mv crictl /usr/local/bin/

set registries "docker.io" "registry.k8s.io" "quay.io" "gcr.io" "ghcr.io"
set CERT_D_DIR "/etc/containerd/certs.d"

for reg in $registries
mkdir -p $CERT_D_DIR/$reg

    switch $reg
        case docker.io
            set mirrors "docker.1ms.run" "docker.m.daocloud.io" "docker.nju.edu.cn"
        case registry.k8s.io
            set mirrors "k8s.m.daocloud.io" "k8s.nju.edu.cn"
        case quay.io
            set mirrors "quay.m.daocloud.io" "quay.nju.edu.cn"
        case gcr.io
            set mirrors "gcr.m.daocloud.io" "gcr.nju.edu.cn"
        case ghcr.io
            set mirrors "ghcr.1ms.run" "ghcr.m.daocloud.io" "ghcr.nju.edu.cn"
    end

    echo "server = \"https://$reg\"" > $CERT_D_DIR/$reg/hosts.toml
    for mirror in $mirrors
        echo "[host.\"https://$mirror\"]" >> $CERT_D_DIR/$reg/hosts.toml
        echo "  capabilities = [\"pull\", \"resolve\"]" >> $CERT_D_DIR/$reg/hosts.toml
    end
end
bash -c '
mkdir -p /etc/containerd
containerd config default | sed "s/SystemdCgroup = false/SystemdCgroup = true/" > /etc/containerd/config.toml

cat > /etc/systemd/system/containerd.service << EOF
[Unit]
Description=containerd container runtime
Documentation=https://containerd.io
After=network.target local-fs.target

[Service]
ExecStartPre=-/sbin/modprobe overlay
ExecStart=/usr/local/bin/containerd
Type=notify
Delegate=yes
KillMode=process
Restart=always
RestartSec=5
LimitNPROC=infinity
LimitCORE=infinity
LimitNOFILE=infinity
TasksMax=infinity
OOMScoreAdjust=-999

[Install]
WantedBy=multi-user.target
EOF

cat > /etc/systemd/system/kubelet.service << EOF
[Unit]
Description=kubelet: The Kubernetes Node Agent
Documentation=https://kubernetes.io/docs/home/
Wants=network-online.target
After=network-online.target
Requires=containerd.service
After=containerd.service

[Service]
ExecStart=/usr/local/bin/kubelet
Restart=always
StartLimitInterval=0
RestartSec=10

[Install]
WantedBy=multi-user.target
EOF

mkdir -p /etc/systemd/system/kubelet.service.d
cat > /etc/systemd/system/kubelet.service.d/10-kubeadm.conf << EOF
[Service]
Environment="KUBELET_KUBECONFIG_ARGS=--bootstrap-kubeconfig=/etc/kubernetes/bootstrap-kubelet.conf --kubeconfig=/etc/kubernetes/kubelet.conf"
Environment="KUBELET_CONFIG_ARGS=--config=/var/lib/kubelet/config.yaml"
Environment="KUBELET_KUBEADM_ARGS=--container-runtime-endpoint=unix:///run/containerd/containerd.sock --pod-infra-container-image=registry.k8s.io/pause:3.9"
EnvironmentFile=-/var/lib/kubelet/kubeadm-flags.env
EnvironmentFile=-/etc/default/kubelet
ExecStart=
ExecStart=/usr/local/bin/kubelet $KUBELET_KUBECONFIG_ARGS $KUBELET_CONFIG_ARGS $KUBELET_KUBEADM_ARGS $KUBELET_EXTRA_ARGS
EOF

systemctl daemon-reload
systemctl enable --now containerd kubelet
'

set local_ip (ip route get 1 | awk '{print $7}' | head -1)

systemctl daemon-reload
systemctl enable --now containerd
systemctl enable --now kubelet

kubeadm init --kubernetes-version=$K8S_VERSION --apiserver-advertise-address=$local_ip --pod-network-cidr=10.244.0.0/16 --image-repository=k8s.m.daocloud.io --skip-phases=addon/kube-proxy

mkdir -p $HOME/.kube
cp -i /etc/kubernetes/admin.conf $HOME/.kube/config
chown (id -u):(id -g) $HOME/.kube/config

cilium install --set kubeProxyReplacement=true