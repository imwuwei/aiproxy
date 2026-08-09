package proxy

import (
	"net/http"
	"time"

	"aiproxy/internal/models"
	"aiproxy/internal/proxy/relay"
)

// recordUsage 记录请求用量
func (s *Server) recordUsage(r *http.Request, model string, start time.Time, usage *relay.UsageAccumulator, statusCode int, errMsg string, reqErr error, channelID int64, channelName string) {
	rec := &models.UsageRecord{
		// Round(0) 去除 time.Now() 携带的单调时钟（monotonic clock），
		// 避免 SQLite 按 time.Time.String() 存储时带上 " m=+..." 后缀。
		CreatedAt:   time.Now().Round(0),
		ChannelID:   channelID,
		ChannelName: channelName,
		Model:       model,
		StatusCode:  statusCode,
		DurationMs:  time.Since(start).Milliseconds(),
		Error:       errMsg,
	}
	if usage != nil {
		rec.PromptTokens = usage.PromptTokens
		rec.CompletionTokens = usage.CompletionTokens
		rec.TotalTokens = usage.PromptTokens + usage.CompletionTokens
		rec.IsSuccess = statusCode >= 200 && statusCode < 300
	} else {
		rec.IsSuccess = false
		if rec.Error == "" && reqErr != nil {
			rec.Error = reqErr.Error()
		}
	}
	if rec.Error == "" {
		rec.Error = http.StatusText(statusCode)
	}
	_ = s.usage.Insert(rec)
}
