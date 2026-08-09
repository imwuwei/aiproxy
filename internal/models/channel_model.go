package models

import "time"

// ChannelModel 渠道-模型映射
type ChannelModel struct {
	ID        int64     `json:"id"`
	ChannelID int64     `json:"channel_id"`
	Model     string    `json:"model"`
	CreatedAt time.Time `json:"created_at"`
}
