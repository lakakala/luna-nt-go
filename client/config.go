package client

import (
	"os"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Addr     string `yaml:"addr"`
	Token    string `yaml:"token"`
	ClientID uint64 `yaml:"client_id"`
}

func Parse(path string) (*Config, error) {
	yamlFile, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	// 3. 初始化 Config 实例（指针类型，用于接收解析结果）
	var config Config

	// 4. 反序列化：将 YAML 字节流解析到 Go 结构体中
	err = yaml.Unmarshal(yamlFile, &config)
	if err != nil {
		return nil, err
	}

	return &config, nil
}
