package main

import (
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/lakakala/luna-nt-go/client"
	"github.com/spf13/cobra"
)

func main() {

	var configPath string

	// 1. 定义根命令（CLI 入口命令）
	var rootCmd = &cobra.Command{
		Use:   "mycli",                                // 命令名称
		Short: "一个简单的 Cobra CLI 演示程序",                 // 简短描述
		Long:  `这是一个基于 Cobra 构建的 CLI 演示程序，支持子命令和参数解析`, // 详细描述
		Run: func(cmd *cobra.Command, args []string) {
			if err := runClient(configPath); err != nil {
				fmt.Printf("start client failed err %s", err)
			}
		},
	}

	rootCmd.Flags().StringVar(&configPath, "config", "", "")

	if err := rootCmd.Execute(); err != nil {
		log.Fatalf("CLI 执行失败：%v", err)
	}
}

func runClient(configPath string) error {

	// 根命令的执行逻辑
	fmt.Printf("config path %s", configPath)
	config, err := client.Parse(configPath)
	if err != nil {
		return err
	}

	client.StartClient(config)

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	sig := <-sigChan
	fmt.Printf("\n接收到信号: %v，开始优雅退出...\n", sig)
	client.StopClient()

	return nil
}
