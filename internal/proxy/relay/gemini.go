package relay

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"aiproxy/internal/models"
)

// Gemini 适配器：OpenAI 格式 ↔ Gemini generateContent 格式
type Gemini struct{}

func init() {
	Register(&Gemini{})
}

// Name 返回厂商类型
func (r *Gemini) Name() models.ChannelType {
	return models.ChannelTypeGemini
}

// geminiPart Gemini 内容部分
type geminiPart struct {
	Text string `json:"text,omitempty"`
}

func openAIContentToGeminiParts(content any) []geminiPart {
	parts := []geminiPart{}
	switch v := content.(type) {
	case string:
		parts = append(parts, geminiPart{Text: v})
	case []any:
		for _, item := range v {
			if m, ok := item.(map[string]any); ok {
				if text, ok := m["text"].(string); ok {
					parts = append(parts, geminiPart{Text: text})
				}
			}
		}
	}
	return parts
}

// BuildChatRequest 构造 Gemini 请求
func (r *Gemini) BuildChatRequest(ctx context.Context, channel *models.Channel, reqBody []byte, keyIdx int) (*http.Request, error) {
	var oa openaiChatRequest
	if err := json.Unmarshal(reqBody, &oa); err != nil {
		return nil, fmt.Errorf("解析 OpenAI 请求失败: %w", err)
	}

	var systemText string
	contents := []map[string]any{}
	for _, m := range oa.Messages {
		if m.Role == "system" {
			systemText = contentToString(m.Content)
			continue
		}
		role := m.Role
		if role == "assistant" {
			role = "model"
		}
		contents = append(contents, map[string]any{
			"role":  role,
			"parts": openAIContentToGeminiParts(m.Content),
		})
	}

	req := map[string]any{}
	if systemText != "" {
		req["systemInstruction"] = map[string]any{
			"parts": []map[string]any{{"text": systemText}},
		}
	}
	req["contents"] = contents

	genConfig := map[string]any{}
	if oa.Temperature != nil {
		genConfig["temperature"] = *oa.Temperature
	}
	if oa.TopP != nil {
		genConfig["topP"] = *oa.TopP
	}
	if oa.TopK != nil {
		genConfig["topK"] = *oa.TopK
	}
	if oa.MaxTokens != nil {
		genConfig["maxOutputTokens"] = *oa.MaxTokens
	}
	if len(genConfig) > 0 {
		req["generationConfig"] = genConfig
	}

	body, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}

	endpoint := ":generateContent"
	if oa.Stream {
		endpoint = ":streamGenerateContent?alt=sse"
	}
	modelPath := "/v1beta/models/" + url.PathEscape(oa.Model) + endpoint
	fullURL := joinURL(channel.BaseURL, modelPath)

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, fullURL, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	key := channel.GetCurrentKey(keyIdx)
	if key != "" {
		httpReq.Header.Set("X-Goog-Api-Key", key)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "application/json")
	return httpReq, nil
}

// geminiListModels 模型列表项
func (r *Gemini) BuildModelsRequest(ctx context.Context, channel *models.Channel, keyIdx int) (*http.Request, error) {
	url := joinURL(channel.BaseURL, "/v1beta/models")
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	key := channel.GetCurrentKey(keyIdx)
	if key != "" {
		req.Header.Set("X-Goog-Api-Key", key)
	}
	req.Header.Set("Accept", "application/json")
	return req, nil
}

// ParseModelsResponse 解析 Gemini 模型列表
func (r *Gemini) ParseModelsResponse(resp *http.Response) ([]string, error) {
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return nil, fmt.Errorf("模型列表请求失败(%d): %s", resp.StatusCode, r.ParseError(body))
	}
	var v struct {
		Models []struct {
			Name string `json:"name"`
		} `json:"models"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&v); err != nil {
		return nil, fmt.Errorf("解析模型列表失败: %w", err)
	}
	modelsList := []string{}
	for _, m := range v.Models {
		// name 形如 "models/gemini-2.0-flash"
		name := m.Name
		if idx := strings.LastIndex(name, "/"); idx >= 0 {
			name = name[idx+1:]
		}
		if name != "" {
			modelsList = append(modelsList, name)
		}
	}
	return modelsList, nil
}

// geminiChunk Gemini 流式 chunk
type geminiChunk struct {
	Candidates []struct {
		Content *struct {
			Parts []struct {
				Text string `json:"text"`
			} `json:"parts"`
		} `json:"content"`
		FinishReason string `json:"finishReason"`
	} `json:"candidates"`
	UsageMetadata *struct {
		PromptTokenCount     int64 `json:"promptTokenCount"`
		CandidatesTokenCount int64 `json:"candidatesTokenCount"`
		TotalTokenCount      int64 `json:"totalTokenCount"`
	} `json:"usageMetadata"`
}

// TransformStream 将 Gemini SSE 流转换为 OpenAI 格式
func (r *Gemini) TransformStream(body io.Reader, writer io.Writer, usage *UsageAccumulator) error {
	reader := bufio.NewReader(body)
	for {
		s, err := readSSE(reader)
		if err != nil {
			if err == io.EOF {
				// 未收到 streamGenerateContent 的流结束标记（FINISH_REASON）即 EOF，
				// 视为流不完整，由上层将本次请求记为失败。
				return ErrStreamIncomplete
			}
			return err
		}
		var c geminiChunk
		if err := json.Unmarshal(s.Data, &c); err != nil {
			continue
		}
		// usageMetadata 是截至当前 chunk 的累计值快照而非增量，
		// 必须用 Set 覆盖而不是 Add 累加，否则多 chunk 携带时会重复计数。
		if c.UsageMetadata != nil {
			usage.Set(c.UsageMetadata.PromptTokenCount, c.UsageMetadata.CandidatesTokenCount)
		}
		content := ""
		if len(c.Candidates) > 0 && c.Candidates[0].Content != nil {
			for _, p := range c.Candidates[0].Content.Parts {
				content += p.Text
			}
		}
		finish := ""
		if len(c.Candidates) > 0 {
			finish = c.Candidates[0].FinishReason
		}
		chunk := map[string]any{
			"id":     "chatcmpl-gemini",
			"object": "chat.completion.chunk",
			"model":  "",
			"choices": []any{
				map[string]any{
					"index":         0,
					"delta":         map[string]any{"content": content},
					"finish_reason": normalizeFinishReason(finish),
				},
			},
		}
		b, _ := json.Marshal(chunk)
		if _, err := fmt.Fprintf(writer, "data: %s\n\n", b); err != nil {
			return err
		}
		// Gemini 流结束标记：最后一个 chunk 的 candidates[0].finishReason 非空。
		// 输出 [DONE] 并正常结束；若一直未出现结束标记直至 EOF，则视为流不完整。
		if finish != "" {
			if _, err := writer.Write([]byte("data: [DONE]\n\n")); err != nil {
				return err
			}
			return nil
		}
	}
}

func normalizeFinishReason(reason string) any {
	switch reason {
	case "":
		return nil
	case "STOP":
		return "stop"
	case "MAX_TOKENS":
		return "length"
	default:
		return "stop"
	}
}

// TransformResponse 将 Gemini 非流式响应转换为 OpenAI 格式
func (r *Gemini) TransformResponse(body []byte, usage *UsageAccumulator) ([]byte, error) {
	var g struct {
		Candidates []struct {
			Content *struct {
				Parts []struct {
					Text string `json:"text"`
				} `json:"parts"`
			} `json:"content"`
			FinishReason string `json:"finishReason"`
		} `json:"candidates"`
		UsageMetadata *struct {
			PromptTokenCount     int64 `json:"promptTokenCount"`
			CandidatesTokenCount int64 `json:"candidatesTokenCount"`
		} `json:"usageMetadata"`
	}
	if err := json.Unmarshal(body, &g); err != nil {
		return nil, fmt.Errorf("解析 Gemini 响应失败: %w", err)
	}
	if g.UsageMetadata != nil {
		usage.Add(g.UsageMetadata.PromptTokenCount, g.UsageMetadata.CandidatesTokenCount)
	}

	content := ""
	if len(g.Candidates) > 0 && g.Candidates[0].Content != nil {
		for _, p := range g.Candidates[0].Content.Parts {
			content += p.Text
		}
	}
	finish := "stop"
	if len(g.Candidates) > 0 && g.Candidates[0].FinishReason != "" {
		if r := normalizeFinishReason(g.Candidates[0].FinishReason); r != nil {
			if s, ok := r.(string); ok {
				finish = s
			}
		}
	}

	resp := map[string]any{
		"id":      "chatcmpl-gemini",
		"object":  "chat.completion",
		"created": nowUnix(),
		"model":   "",
		"choices": []any{
			map[string]any{
				"index":         0,
				"message":       map[string]any{"role": "assistant", "content": content},
				"finish_reason": finish,
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

// ParseError 解析 Gemini 错误
func (r *Gemini) ParseError(body []byte) string {
	var v struct {
		Error *struct {
			Message string `json:"message"`
			Status  string `json:"status"`
		} `json:"error"`
	}
	if json.Unmarshal(body, &v) == nil && v.Error != nil {
		if v.Error.Message != "" {
			return v.Error.Message
		}
		return v.Error.Status
	}
	return strings.TrimSpace(string(body))
}
