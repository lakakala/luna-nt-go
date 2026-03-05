package server

import (
	"os"

	"gopkg.in/yaml.v3"
)

const (
	BindTypeTcp              = "tcp"
	BindTypeHttpProxy        = "http_proxy"
	BindTypeHttpReverseProxy = "http_reverse_proxy"
)

type ClientBind struct {
	ID   uint64 `yaml:"id"`
	Type string `yaml:"type"`

	BindAddr  string `yaml:"bind_addr"`
	LocalAddr string `yaml:"local_addr"`

	// for type = http_proxy
	HttpProxyBindAddr      string   `yaml:"http_proxy_bind_addr"`
	HttpProxyAllowHostList []string `yaml:"http_proxy_allow_hosts"`

	HttpReverseProxyBindAddr  string `yaml:"http_reverse_proxy_bind_addr"`
	HttpReverseProxyLocalAddr string `yaml:"http_reverse_proxy_local_addr"`
}

type ClientConfig struct {
	ID    uint64        `yaml:"id"`
	Binds []*ClientBind `yaml:"binds"`
}

type Config struct {
	Addr    string                   `yaml:"addr"`
	Token   string                   `yaml:"token"`
	Clients map[uint64]*ClientConfig `yaml:"clients"`
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
