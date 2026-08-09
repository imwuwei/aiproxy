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

// OpenAICompatible 支持 OpenAI 格式的直通中继
type OpenAICompatible struct{}

func init() {
	Register(&OpenAICompatible{})
}

// Name 返回厂商类型
func (r *OpenAICompatible) Name() models.ChannelType {
	return models.ChannelTypeOpenAICompatible
}

// joinURL 拼接 baseURL 与路径
func joinURL(base, path string) string {
	return strings.TrimRight(base, "/") + path
}

// joinAPIV1URL 拼接 baseURL 与 OpenAI API 路径，避免版本号重复。
// 若 base 已以 /vN（如 /v1、/v2、/v3）结尾，则保留原有版本号直接拼接
// （如 https://api.openai.com/v2 + /models → https://api.openai.com/v2/models）；
// 否则自动补上 /v1（如 https://api.openai.com + /v1/models）。
func joinAPIV1URL(base, path string) string {
	base = strings.TrimRight(base, "/")
	if !hasVersionSuffix(base) {
		base += "/v1"
	}
	return base + path
}

// hasVersionSuffix 判断 base 是否以 /vN（N 为纯数字）结尾，如 /v1、/v2、/v10。
// 用于识别 BaseURL 中已携带的 API 版本号，避免重复拼接 /v1。
func hasVersionSuffix(base string) bool {
	idx := strings.LastIndex(base, "/")
	if idx < 0 || idx == len(base)-1 {
		return false
	}
	seg := base[idx+1:]
	if len(seg) < 2 || (seg[0] != 'v' && seg[0] != 'V') {
		return false
	}
	for _, c := range seg[1:] {
		if c < '0' || c > '9' {
			return false
		}
	}
	return true
}

// BuildChatRequest 构造 OpenAI 格式聊天请求（直接透传）
func (r *OpenAICompatible) BuildChatRequest(ctx context.Context, channel *models.Channel, reqBody []byte, keyIdx int) (*http.Request, error) {
	if isChatCompletionRequest(reqBody) {
		// 流式请求注入 stream_options.include_usage，保证能取到 token 用量
		body, err := ensureStreamUsage(reqBody)
		if err != nil {
			return nil, err
		}
		return r.buildRequest(ctx, channel, "/chat/completions", body, keyIdx)
	}
	return r.buildRequest(ctx, channel, "/embeddings", reqBody, keyIdx)
}

// ensureStreamUsage 保证流式请求能返回 usage：
// 标准 OpenAI 流式响应默认不含 usage，必须携带 stream_options: {"include_usage": true}
// 时最后一个 chunk 才会带 usage。这里仅在客户端未显式提供 stream_options 时注入，
// 尊重客户端已有设置。非流式或非 chat/completions 请求不处理。
func ensureStreamUsage(body []byte) ([]byte, error) {
	var v struct {
		Stream        bool            `json:"stream"`
		StreamOptions json.RawMessage `json:"stream_options"`
	}
	if err := json.Unmarshal(body, &v); err != nil {
		// 非 JSON 或解析失败：保持原样
		return body, nil
	}
	// 未开启流式，或客户端已显式提供 stream_options：不注入
	if !v.Stream || len(v.StreamOptions) > 0 {
		return body, nil
	}
	// 用 map 重新组装，注入 stream_options
	var m map[string]any
	if err := json.Unmarshal(body, &m); err != nil {
		return body, nil
	}
	m["stream_options"] = map[string]any{
		"include_usage": true,
	}
	out, err := json.Marshal(m)
	if err != nil {
		return body, nil
	}
	return out, nil
}

func (r *OpenAICompatible) buildRequest(ctx context.Context, channel *models.Channel, path string, reqBody []byte, keyIdx int) (*http.Request, error) {
	url := joinAPIV1URL(channel.BaseURL, path)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(reqBody))
	if err != nil {
		return nil, err
	}
	key := channel.GetCurrentKey(keyIdx)
	if key != "" {
		req.Header.Set("Authorization", "Bearer "+key)
	}
	req.Header.Set("Content-Type", "application/json")
	// 同时接受 SSE 流式与 JSON 响应：请求体带 stream=true 时上游将按 SSE 返回
	req.Header.Set("Accept", "text/event-stream, application/json")
	return req, nil
}

// isChatCompletionRequest 根据请求体判断是否为聊天补全（含嵌入请求）
func isChatCompletionRequest(body []byte) bool {
	var v struct {
		Input any `json:"input"`
	}
	if json.Unmarshal(body, &v) == nil && v.Input != nil {
		return false
	}
	return true
}

// BuildModelsRequest 构造模型列表请求
func (r *OpenAICompatible) BuildModelsRequest(ctx context.Context, channel *models.Channel, keyIdx int) (*http.Request, error) {
	url := joinAPIV1URL(channel.BaseURL, "/models")
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	key := channel.GetCurrentKey(keyIdx)
	if key != "" {
		req.Header.Set("Authorization", "Bearer "+key)
	}
	req.Header.Set("Accept", "application/json")
	return req, nil
}

// ParseModelsResponse 解析 OpenAI 格式模型列表
func (r *OpenAICompatible) ParseModelsResponse(resp *http.Response) ([]string, error) {
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

// sseData 解析 SSE 数据行
type sseData struct {
	Data []byte
	Done bool
}

// readSSE 读取一条 SSE 数据
func readSSE(reader *bufio.Reader) (*sseData, error) {
	for {
		line, err := reader.ReadBytes('\n')
		if err != nil {
			return nil, err
		}
		line = bytes.TrimSpace(line)
		if len(line) == 0 {
			continue
		}
		if !bytes.HasPrefix(line, []byte("data:")) {
			continue
		}
		data := bytes.TrimSpace(line[len("data:"):])
		if bytes.Equal(data, []byte("[DONE]")) {
			return &sseData{Done: true}, nil
		}
		return &sseData{Data: data}, nil
	}
}

// TransformStream 透传 SSE 流并累计 usage（OpenAI 格式，直接复制）
func (r *OpenAICompatible) TransformStream(body io.Reader, writer io.Writer, usage *UsageAccumulator) error {
	reader := bufio.NewReader(body)
	for {
		s, err := readSSE(reader)
		if err != nil {
			if err == io.EOF {
				// 未收到 [DONE] 即 EOF，视为流不完整，由上层将本次请求记为失败。
				return ErrStreamIncomplete
			}
			return err
		}
		if s.Done {
			if _, err := writer.Write([]byte("data: [DONE]\n\n")); err != nil {
				return err
			}
			return nil
		}
		// 从 chunk 提取 usage。
		// usage 携带的是累计值快照（标准 OpenAI 仅在最后一个 chunk 出现一次），
		// 必须用 Set 覆盖而不是 Add 累加，避免个别多 chunk 携带 usage 的兼容厂商被重复计数。
		var chunk struct {
			Usage *struct {
				PromptTokens     int64 `json:"prompt_tokens"`
				CompletionTokens int64 `json:"completion_tokens"`
			} `json:"usage"`
		}
		if json.Unmarshal(s.Data, &chunk) == nil && chunk.Usage != nil {
			usage.Set(chunk.Usage.PromptTokens, chunk.Usage.CompletionTokens)
		}
		if _, err := fmt.Fprintf(writer, "data: %s\n\n", s.Data); err != nil {
			return err
		}
	}
}

// TransformResponse 非流式响应直接返回，提取 usage
func (r *OpenAICompatible) TransformResponse(body []byte, usage *UsageAccumulator) ([]byte, error) {
	var v struct {
		Usage *struct {
			PromptTokens     int64 `json:"prompt_tokens"`
			CompletionTokens int64 `json:"completion_tokens"`
		} `json:"usage"`
	}
	if json.Unmarshal(body, &v) == nil && v.Usage != nil {
		usage.Add(v.Usage.PromptTokens, v.Usage.CompletionTokens)
	}
	return body, nil
}

// ParseError 解析 OpenAI 格式错误
func (r *OpenAICompatible) ParseError(body []byte) string {
	var v struct {
		Error *struct {
			Message string `json:"message"`
			Type    string `json:"type"`
		} `json:"error"`
	}
	if json.Unmarshal(body, &v) == nil && v.Error != nil {
		if v.Error.Message != "" {
			return v.Error.Message
		}
	}
	return strings.TrimSpace(string(body))
}
