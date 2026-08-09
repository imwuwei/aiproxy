package relay

import (
	"context"
	"errors"
	"io"
	"net/http"

	"aiproxy/internal/models"
)

// ErrStreamIncomplete 表示流式响应未收到结束标记（[DONE]/message_stop）即提前结束。
// 通常由客户端断开或上游异常中断引起，此时 token 统计不完整，不应记为成功。
var ErrStreamIncomplete = errors.New("stream ended before completion marker")

// UsageAccumulator 用量累计器（流式场景从 chunk 累计 token）
type UsageAccumulator struct {
	PromptTokens     int64
	CompletionTokens int64
}

// Add 累加用量（增量语义，用于非流式一次性完整值）
func (u *UsageAccumulator) Add(prompt, completion int64) {
	u.PromptTokens += prompt
	u.CompletionTokens += completion
}

// Set 用上游返回的累计值快照覆盖当前用量（快照语义）。
// 流式场景中 usage/usageMetadata 是截至当前的累计值而非增量，
// 必须用最后一次出现的值覆盖，否则会重复计数。
func (u *UsageAccumulator) Set(prompt, completion int64) {
	u.PromptTokens = prompt
	u.CompletionTokens = completion
}

// Relay 中继接口：统一不同厂商的请求/响应格式差异
type Relay interface {
	// Name 返回厂商类型
	Name() models.ChannelType

	// BuildChatRequest 将 OpenAI 格式聊天请求转换为厂商请求
	BuildChatRequest(ctx context.Context, channel *models.Channel, reqBody []byte, keyIdx int) (*http.Request, error)

	// BuildModelsRequest 构造厂商模型列表请求
	BuildModelsRequest(ctx context.Context, channel *models.Channel, keyIdx int) (*http.Request, error)

	// ParseModelsResponse 解析厂商模型列表响应，返回模型 ID 列表
	ParseModelsResponse(resp *http.Response) ([]string, error)

	// TransformStream 将厂商流式响应转换为 OpenAI 格式 SSE 流。
	// 返回 nil 表示厂商已是 OpenAI 格式，可直接透传。
	// usage 用于累计流式 token。
	// 正常结束时必须收到厂商结束标记（[DONE]/message_stop）才返回 nil；
	// 若流提前中断（io.EOF 但未见结束标记）应返回 ErrStreamIncomplete，
	// 以便上层将本次请求标记为不完整而非成功。
	TransformStream(body io.Reader, writer io.Writer, usage *UsageAccumulator) error

	// TransformResponse 将厂商非流式响应转换为 OpenAI 格式。
	// usage 用于收集 token 用量。
	TransformResponse(body []byte, usage *UsageAccumulator) ([]byte, error)

	// ParseError 解析厂商错误响应，返回可读的错误消息
	ParseError(body []byte) string
}

// registry 厂商注册表
var registry = map[models.ChannelType]Relay{}

// Register 注册中继实现
func Register(r Relay) {
	registry[r.Name()] = r
}

// Get 获取中继实现
func Get(typ models.ChannelType) (Relay, bool) {
	r, ok := registry[typ]
	return r, ok
}
