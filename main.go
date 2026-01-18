package main

import (
	"fmt"
	"os"
	"time"
)

func main() {
	// 打印到控制台
	fmt.Println("Hello from kubetool!")

	// 同时写入文件
	file, err := os.Create("/data/kubetool.txt")
	if err != nil {
		fmt.Printf("创建文件失败: %v\n", err)
		return
	}
	defer func(file *os.File) {
		err := file.Close()
		if err != nil {

		}
	}(file)

	content := "kubetool 构建成功！\n版本: 1.0.0\n构建时间: n" + time.Now().Format("2006-01-02 15:04:05")
	_, err = file.WriteString(content)
	if err != nil {
		fmt.Printf("写入文件失败: %v\n", err)
		return
	}

	fmt.Println("文件 /data/kubetool.txt 已生成")
}
