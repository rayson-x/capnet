package cap

import (
	"os"
	"os/user"

	"gopkg.in/yaml.v3"
)

// Config 是 node 的能力配置表:加一条能力 = 加一个条目。
type Config struct {
	NodeID       string      `yaml:"node_id"`
	Listen       string      `yaml:"listen"` // 如 127.0.0.1:8080 或 tailnet 地址;空 = 随机端口
	Capabilities []CapConfig `yaml:"capabilities"`
}

type CapConfig struct {
	Name   string `yaml:"name"`
	Kind   string `yaml:"kind"`  // plugin | forward
	Local  string `yaml:"local"` // forward:本机已有服务地址
	Probe  string `yaml:"probe"` // forward:探活路径
	Schema string `yaml:"schema"`
}

// LoadConfig 读取 capabilities.yaml。
func LoadConfig(path string) (Config, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return Config{}, err
	}
	var c Config
	if err := yaml.Unmarshal(b, &c); err != nil {
		return Config{}, err
	}
	if c.NodeID == "" {
		c.NodeID = defaultNodeID()
	}
	if c.Listen == "" {
		c.Listen = "127.0.0.1:0" // 随机端口,用于测试/默认
	}
	return c, nil
}

func defaultNodeID() string {
	if u, err := user.Current(); err == nil && u.Username != "" {
		return u.Username
	}
	h, err := os.Hostname()
	if err != nil {
		return "node"
	}
	return h
}
