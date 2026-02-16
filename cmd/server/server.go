package main

import (
	"fmt"
	"log"

	"github.com/lakakala/luna-nt-go/server"
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
			if err := runServer(configPath); err != nil {
				fmt.Printf("start client failed err %s", err)
			}
		},
	}

	rootCmd.Flags().StringVar(&configPath, "config", "", "")

	if err := rootCmd.Execute(); err != nil {
		log.Fatalf("CLI 执行失败：%v", err)
	}
}

func runServer(configPath string) error {

	// 根命令的执行逻辑
	fmt.Printf("config path %s", configPath)
	config, err := server.Parse(configPath)
	if err != nil {
		return err
	}

	return server.RunServer(config)
}
