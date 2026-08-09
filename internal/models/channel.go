package models

import "time"

// ChannelType 厂商类型
type ChannelType string

const (
	ChannelTypeOpenAICompatible ChannelType = "openai-compatible"
	ChannelTypeAnthropic        ChannelType = "anthropic"
	ChannelTypeGemini           ChannelType = "gemini"
	ChannelTypeCustom           ChannelType = "custom"
)

// ChannelStatus 渠道状态
type ChannelStatus string

const (
	ChannelStatusOnline  ChannelStatus = "online"
	ChannelStatusOffline ChannelStatus = "offline"
	ChannelStatusCooling ChannelStatus = "cooling"
)

// Channel 厂商渠道
type Channel struct {
	ID            int64         `json:"id"`
	Name          string        `json:"name"`
	Type          ChannelType   `json:"type"`
	BaseURL       string        `json:"base_url"`
	APIKeys       []string      `json:"api_keys"`
	Priority      int           `json:"priority"`
	Enabled       bool          `json:"enabled"`
	Status        ChannelStatus `json:"status"`
	LastError     string        `json:"last_error"`
	LastSuccessAt *time.Time    `json:"last_success_at"`
	ModelCount    int           `json:"model_count"`
	CreatedAt     time.Time     `json:"created_at"`
	UpdatedAt     time.Time     `json:"updated_at"`
}

// APIKeysJSON 返回 API Keys 的 JSON 字符串（用于存储）
func (c *Channel) APIKeysJSON() string {
	return marshalJSON(c.APIKeys)
}

// SetAPIKeysFromJSON 从 JSON 字符串解析 API Keys
func (c *Channel) SetAPIKeysFromJSON(s string) error {
	keys, err := unmarshalJSON[[]string](s)
	if err != nil {
		return err
	}
	c.APIKeys = keys
	return nil
}

// GetCurrentKey 轮询获取当前可用的 API Key（按请求次数轮询）
func (c *Channel) GetCurrentKey(counter int) string {
	if len(c.APIKeys) == 0 {
		return ""
	}
	return c.APIKeys[counter%len(c.APIKeys)]
}
