package config

import (
	"crypto/rand"
	"encoding/hex"
	"strconv"
	"time"
)

// Config 运行时配置（从 SQLite settings 表加载）
type Config struct {
	ListenAddr        string
	ListenPort        int
	AccessToken       string
	AuthEnabled       bool
	ModelSyncInterval time.Duration
	ProxyTimeout      time.Duration
	BreakerThreshold  int
	BreakerCooldown   time.Duration
	LogRetentionDays  int
	Debug             bool
	StartMinimized    bool
	// TokenDisplay Token 数值显示方式：auto（≥100 万显示为 M）| raw（原始千分位）
	TokenDisplay string
}

// DefaultConfig 返回默认配置
func DefaultConfig() *Config {
	return &Config{
		ListenAddr:        "127.0.0.1",
		ListenPort:        17880,
		AccessToken:       "sk-aiproxy",
		AuthEnabled:       true,
		ModelSyncInterval: time.Hour,
		ProxyTimeout:      120 * time.Second,
		BreakerThreshold:  5,
		BreakerCooldown:   30 * time.Second,
		LogRetentionDays:  365,
		Debug:             false,
		StartMinimized:    false,
		TokenDisplay:      "auto",
	}
}

// GenerateAccessToken 生成随机访问令牌（sk- + 32 位十六进制，共 35 字符）。
// 使用 crypto/rand 安全随机源，用于首次初始化数据库与设置页"随机生成"按钮。
func GenerateAccessToken() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		// crypto/rand 失败时回退到静态默认值，保证程序可运行
		return DefaultConfig().AccessToken
	}
	return "sk-" + hex.EncodeToString(b)
}

// ProxyAddr 返回完整监听地址
func (c *Config) ProxyAddr() string {
	return c.ListenAddr + ":" + strconv.Itoa(c.ListenPort)
}

// BaseURL 返回 OpenAI 兼容接口的完整基础地址（含 http:// 前缀与 /v1 路径）
func (c *Config) BaseURL() string {
	return "http://" + c.ProxyAddr() + "/v1"
}
