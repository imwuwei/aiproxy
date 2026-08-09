package relay

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"aiproxy/internal/models"
)

// openaiMessage OpenAI 消息结构
type openaiMessage struct {
	Role    string `json:"role"`
	Content any    `json:"content"`
}

// openaiChatRequest OpenAI 聊天请求
type openaiChatRequest struct {
	Model       string           `json:"model"`
	Messages    []openaiMessage  `json:"messages"`
	Stream      bool             `json:"stream"`
	Temperature *float64         `json:"temperature,omitempty"`
	MaxTokens   *int             `json:"max_tokens,omitempty"`
	TopP        *float64         `json:"top_p,omitempty"`
	TopK        *int             `json:"top_k,omitempty"`
	Stop        *json.RawMessage `json:"stop,omitempty"`
}

// contentToString 将内容字段转为字符串
func contentToString(c any) string {
	switch v := c.(type) {
	case string:
		return v
	case []any:
		parts := []string{}
		for _, item := range v {
			if m, ok := item.(map[string]any); ok {
				if t, ok := m["text"].(string); ok {
					parts = append(parts, t)
				}
			}
		}
		return strings.Join(parts, "\n")
	default:
		b, _ := json.Marshal(c)
		return string(b)
	}
}

// Anthropic 适配器：OpenAI 格式 ↔ Anthropic messages 格式
type Anthropic struct{}

func init() {
	Register(&Anthropic{})
}

// Name 返回厂商类型
func (r *Anthropic) Name() models.ChannelType {
	return models.ChannelTypeAnthropic
}

// anthropicRequest Anthropic messages 请求
type anthropicRequest struct {
	Model       string             `json:"model"`
	Messages    []anthropicMessage `json:"messages"`
	MaxTokens   int                `json:"max_tokens"`
	Stream      bool               `json:"stream,omitempty"`
	Temperature *float64           `json:"temperature,omitempty"`
	TopP        *float64           `json:"top_p,omitempty"`
	TopK        *int               `json:"top_k,omitempty"`
	StopSeq     []string           `json:"stop_sequences,omitempty"`
	System      string             `json:"system,omitempty"`
}

type anthropicMessage struct {
	Role    string `json:"role"`
	Content any    `json:"content"`
}

// BuildChatRequest 构造 Anthropic 请求
func (r *Anthropic) BuildChatRequest(ctx context.Context, channel *models.Channel, reqBody []byte, keyIdx int) (*http.Request, error) {
	var oa openaiChatRequest
	if err := json.Unmarshal(reqBody, &oa); err != nil {
		return nil, fmt.Errorf("解析 OpenAI 请求失败: %w", err)
	}

	ar := anthropicRequest{
		Model:       oa.Model,
		Stream:      oa.Stream,
		Temperature: oa.Temperature,
		TopP:        oa.TopP,
		TopK:        oa.TopK,
		MaxTokens:   4096,
	}
	if oa.MaxTokens != nil && *oa.MaxTokens > 0 {
		ar.MaxTokens = *oa.MaxTokens
	}
	if oa.Stop != nil {
		var stops []string
		if json.Unmarshal(*oa.Stop, &stops) == nil {
			ar.StopSeq = stops
		} else {
			var single string
			if json.Unmarshal(*oa.Stop, &single) == nil {
				ar.StopSeq = []string{single}
			}
		}
	}
	// system 提取
	var systemParts []string
	for _, m := range oa.Messages {
		if m.Role == "system" {
			systemParts = append(systemParts, contentToString(m.Content))
			continue
		}
		am := anthropicMessage{Role: m.Role, Content: m.Content}
		ar.Messages = append(ar.Messages, am)
	}
	if len(systemParts) > 0 {
		ar.System = strings.Join(systemParts, "\n\n")
	}

	body, err := json.Marshal(ar)
	if err != nil {
		return nil, err
	}
	url := joinAPIV1URL(channel.BaseURL, "/messages")
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	key := channel.GetCurrentKey(keyIdx)
	if key != "" {
		req.Header.Set("X-Api-Key", key)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	return req, nil
}

// BuildModelsRequest 构造 Anthropic 模型列表请求
func (r *Anthropic) BuildModelsRequest(ctx context.Context, channel *models.Channel, keyIdx int) (*http.Request, error) {
	url := joinAPIV1URL(channel.BaseURL, "/models")
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	key := channel.GetCurrentKey(keyIdx)
	if key != "" {
		req.Header.Set("X-Api-Key", key)
	}
	req.Header.Set("Accept", "application/json")
	return req, nil
}

// ParseModelsResponse 解析 Anthropic 模型列表
func (r *Anthropic) ParseModelsResponse(resp *http.Response) ([]string, error) {
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return nil, fmt.Errorf("模型列表请求失败(%d): %s", resp.StatusCode, r.ParseError(body))
	}
	var v struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&v); err != nil {
		return nil, fmt.Errorf("解析模型列表失败: %w", err)
	}
	modelsList := []string{}
	for _, m := range v.Data {
		if m.ID != "" {
			modelsList = append(modelsList, m.ID)
		}
	}
	return modelsList, nil
}

// anthropicStreamEvent Anthropic SSE 事件
type anthropicStreamEvent struct {
	Type  string `json:"type"`
	Delta *struct {
		Type         string `json:"type"`
		Text         string `json:"text"`
		ContentBlock string `json:"content_block"`
		Index        int    `json:"index"`
	} `json:"delta"`
	Message *struct {
		Usage *struct {
			InputTokens  int64 `json:"input_tokens"`
			OutputTokens int64 `json:"output_tokens"`
		} `json:"usage"`
	} `json:"message"`
	Usage *struct {
		InputTokens  int64 `json:"input_tokens"`
		OutputTokens int64 `json:"output_tokens"`
	} `json:"usage"`
}

// TransformStream 将 Anthropic SSE 流转换为 OpenAI 格式
func (r *Anthropic) TransformStream(body io.Reader, writer io.Writer, usage *UsageAccumulator) error {
	reader := bufio.NewReader(body)
	for {
		s, err := readSSE(reader)
		if err != nil {
			if err == io.EOF {
				// 未收到 message_stop 即 EOF，视为流不完整，由上层将本次请求记为失败。
				return ErrStreamIncomplete
			}
			return err
		}
		var ev anthropicStreamEvent
		if err := json.Unmarshal(s.Data, &ev); err != nil {
			continue
		}
		// usage 事件携带的是累计值快照（input_tokens 恒定、output_tokens 递增至最终值），
		// 必须用 Set 覆盖而不是 Add 累加：
		//   - message_start 的 usage.output_tokens 恒为 1（占位初始值）
		//   - 后续 usage/message_delta 事件为整段输出的最终累计值
		// 最后一次 Set 即得到最终正确结果。
		if ev.Usage != nil {
			usage.Set(ev.Usage.InputTokens, ev.Usage.OutputTokens)
			continue
		}
		if ev.Message != nil && ev.Message.Usage != nil {
			usage.Set(ev.Message.Usage.InputTokens, ev.Message.Usage.OutputTokens)
		}
		switch ev.Type {
		case "message_start":
			// 输出一个空 chunk
			chunk := map[string]any{
				"id":      "chatcmpl-anthropic",
				"object":  "chat.completion.chunk",
				"model":   "",
				"choices": []any{},
			}
			b, _ := json.Marshal(chunk)
			if _, err := fmt.Fprintf(writer, "data: %s\n\n", b); err != nil {
				return err
			}
		case "content_block_delta":
			if ev.Delta != nil && ev.Delta.Type == "text_delta" {
				chunk := map[string]any{
					"id":     "chatcmpl-anthropic",
					"object": "chat.completion.chunk",
					"choices": []any{
						map[string]any{
							"index": 0,
							"delta": map[string]any{"content": ev.Delta.Text},
						},
					},
				}
				b, _ := json.Marshal(chunk)
				if _, err := fmt.Fprintf(writer, "data: %s\n\n", b); err != nil {
					return err
				}
			}
		case "message_stop":
			if _, err := writer.Write([]byte("data: [DONE]\n\n")); err != nil {
				return err
			}
			return nil
		}
	}
}

// TransformResponse 将 Anthropic 非流式响应转换为 OpenAI 格式
func (r *Anthropic) TransformResponse(body []byte, usage *UsageAccumulator) ([]byte, error) {
	var ar struct {
		ID      string `json:"id"`
		Content []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content"`
		Model string `json:"model"`
		Usage *struct {
			InputTokens  int64 `json:"input_tokens"`
			OutputTokens int64 `json:"output_tokens"`
		} `json:"usage"`
		StopReason *string `json:"stop_reason"`
	}
	if err := json.Unmarshal(body, &ar); err != nil {
		return nil, fmt.Errorf("解析 Anthropic 响应失败: %w", err)
	}
	if ar.Usage != nil {
		usage.Add(ar.Usage.InputTokens, ar.Usage.OutputTokens)
	}

	content := ""
	for _, c := range ar.Content {
		if c.Type == "text" {
			content += c.Text
		}
	}
	finishReason := "stop"
	if ar.StopReason != nil && *ar.StopReason != "" {
		finishReason = *ar.StopReason
	}

	resp := map[string]any{
		"id":      ar.ID,
		"object":  "chat.completion",
		"created": nowUnix(),
		"model":   ar.Model,
		"choices": []any{
			map[string]any{
				"index":         0,
				"message":       map[string]any{"role": "assistant", "content": content},
				"finish_reason": finishReason,
			},
		},
		"usage": map[string]any{
			"prompt_tokens":     usage.PromptTokens,
			"completion_tokens": usage.CompletionTokens,
			"total_tokens":      usage.PromptTokens + usage.CompletionTokens,
		},
	}
	return json.Marshal(resp)
}

// ParseError 解析 Anthropic 错误
func (r *Anthropic) ParseError(body []byte) string {
	var v struct {
		Error *struct {
			Message string `json:"message"`
			Type    string `json:"type"`
		} `json:"error"`
	}
	if json.Unmarshal(body, &v) == nil && v.Error != nil && v.Error.Message != "" {
		return v.Error.Message
	}
	return strings.TrimSpace(string(body))
}
