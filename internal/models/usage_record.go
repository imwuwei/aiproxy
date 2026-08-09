package models

import "time"

// UsageRecord 请求用量记录
type UsageRecord struct {
	ID        int64     `json:"id"`
	CreatedAt time.Time `json:"created_at"`
	// CreatedAtRaw 数据库中原样存储的 created_at 字符串（不做二次格式化，展示时优先使用）
	CreatedAtRaw     string `json:"created_at_raw"`
	ChannelID        int64  `json:"channel_id"`
	ChannelName      string `json:"channel_name"`
	Model            string `json:"model"`
	PromptTokens     int64  `json:"prompt_tokens"`
	CompletionTokens int64  `json:"completion_tokens"`
	TotalTokens      int64  `json:"total_tokens"`
	IsSuccess        bool   `json:"is_success"`
	StatusCode       int    `json:"status_code"`
	DurationMs       int64  `json:"duration_ms"`
	Error            string `json:"error"`
}

// DailyStat 按日统计结果
type DailyStat struct {
	Date             string `json:"date"`
	Count            int64  `json:"count"`
	PromptTokens     int64  `json:"prompt_tokens"`
	CompletionTokens int64  `json:"completion_tokens"`
	TotalTokens      int64  `json:"total_tokens"`
	SuccessCount     int64  `json:"success_count"`
	FailCount        int64  `json:"fail_count"`
}

// SummaryStat 汇总统计结果（时间段内整体）
type SummaryStat struct {
	Count            int64 `json:"count"`
	PromptTokens     int64 `json:"prompt_tokens"`
	CompletionTokens int64 `json:"completion_tokens"`
	TotalTokens      int64 `json:"total_tokens"`
	SuccessCount     int64 `json:"success_count"`
	FailCount        int64 `json:"fail_count"`
}

// ModelStat 按模型统计结果
type ModelStat struct {
	Model            string `json:"model"`
	Count            int64  `json:"count"`
	PromptTokens     int64  `json:"prompt_tokens"`
	CompletionTokens int64  `json:"completion_tokens"`
	TotalTokens      int64  `json:"total_tokens"`
	SuccessCount     int64  `json:"success_count"`
	FailCount        int64  `json:"fail_count"`
}

// ChannelStat 按渠道统计结果
type ChannelStat struct {
	ChannelID        int64  `json:"channel_id"`
	ChannelName      string `json:"channel_name"`
	Count            int64  `json:"count"`
	PromptTokens     int64  `json:"prompt_tokens"`
	CompletionTokens int64  `json:"completion_tokens"`
	TotalTokens      int64  `json:"total_tokens"`
	SuccessCount     int64  `json:"success_count"`
	FailCount        int64  `json:"fail_count"`
}

// TokenUsage 按模型聚合的 token 用量
type TokenUsage struct {
	Model            string `json:"model"`
	PromptTokens     int64  `json:"prompt_tokens"`
	CompletionTokens int64  `json:"completion_tokens"`
	TotalTokens      int64  `json:"total_tokens"`
}
