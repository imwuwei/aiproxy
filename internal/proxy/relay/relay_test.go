package relay

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"aiproxy/internal/models"
)

// ---------- 工具函数 ----------

// openAIReqBody 构造 OpenAI 聊天请求体
func openAIReqBody(stream bool, extra map[string]any) []byte {
	m := map[string]any{
		"model":    "gpt-4o",
		"messages": []any{map[string]any{"role": "user", "content": "hi"}},
		"stream":   stream,
	}
	for k, v := range extra {
		m[k] = v
	}
	b, _ := json.Marshal(m)
	return b
}

type captureWriter struct {
	buf bytes.Buffer
}

func (w *captureWriter) Write(p []byte) (int, error) {
	return w.buf.Write(p)
}

func makeChannel() *models.Channel {
	return &models.Channel{
		Type:    models.ChannelTypeOpenAICompatible,
		BaseURL: "https://api.example.com",
		APIKeys: []string{"sk-test"},
	}
}

// sseStreamFromLines 将多行 "data: ..." 拼接成 SSE 输入流
func sseStreamFromLines(lines ...string) io.Reader {
	var sb strings.Builder
	for _, l := range lines {
		sb.WriteString("data: " + l + "\n\n")
	}
	return strings.NewReader(sb.String())
}

// ---------- ensureStreamUsage 注入测试 ----------

func TestEnsureStreamUsage_StreamingNoOptions_Inject(t *testing.T) {
	body := openAIReqBody(true, nil)
	out, err := ensureStreamUsage(body)
	if err != nil {
		t.Fatalf("ensureStreamUsage error: %v", err)
	}
	var v struct {
		Stream        bool           `json:"stream"`
		StreamOptions map[string]any `json:"stream_options"`
	}
	if err := json.Unmarshal(out, &v); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if !v.Stream {
		t.Fatalf("stream 应保持 true")
	}
	inc, ok := v.StreamOptions["include_usage"].(bool)
	if !ok || !inc {
		t.Fatalf("应注入 stream_options.include_usage=true，实际: %v", v.StreamOptions)
	}
}

func TestEnsureStreamUsage_NonStream_NoInject(t *testing.T) {
	body := openAIReqBody(false, nil)
	out, err := ensureStreamUsage(body)
	if err != nil {
		t.Fatalf("ensureStreamUsage error: %v", err)
	}
	if string(out) != string(body) {
		t.Fatalf("非流式请求不应被修改:\n原: %s\n新: %s", body, out)
	}
}

func TestEnsureStreamUsage_ExistingOptions_Preserved(t *testing.T) {
	body := openAIReqBody(true, map[string]any{
		"stream_options": map[string]any{"include_usage": false},
	})
	out, err := ensureStreamUsage(body)
	if err != nil {
		t.Fatalf("ensureStreamUsage error: %v", err)
	}
	var v struct {
		StreamOptions map[string]any `json:"stream_options"`
	}
	if err := json.Unmarshal(out, &v); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	inc, ok := v.StreamOptions["include_usage"].(bool)
	if !ok || inc {
		t.Fatalf("客户端已有 stream_options 不应被覆盖，实际: %v", v.StreamOptions)
	}
}

func TestEnsureStreamUsage_StillAcceptedByBuildRequest(t *testing.T) {
	r := &OpenAICompatible{}
	req, err := r.BuildChatRequest(context.Background(), makeChannel(), openAIReqBody(true, nil), 0)
	if err != nil {
		t.Fatalf("BuildChatRequest error: %v", err)
	}
	reqBody, err := io.ReadAll(req.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	var v struct {
		StreamOptions map[string]any `json:"stream_options"`
	}
	if err := json.Unmarshal(reqBody, &v); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	inc, ok := v.StreamOptions["include_usage"].(bool)
	if !ok || !inc {
		t.Fatalf("BuildChatRequest 应注入 include_usage，实际: %v", v.StreamOptions)
	}
}

// ---------- UsageAccumulator Add/Set 语义 ----------

func TestUsageAccumulator_AddIsIncremental(t *testing.T) {
	u := &UsageAccumulator{}
	u.Add(10, 20)
	u.Add(5, 5)
	if u.PromptTokens != 15 || u.CompletionTokens != 25 {
		t.Fatalf("Add 应累加，实际 prompt=%d completion=%d", u.PromptTokens, u.CompletionTokens)
	}
}

func TestUsageAccumulator_SetIsSnapshot(t *testing.T) {
	u := &UsageAccumulator{}
	u.Set(10, 20)
	u.Set(30, 40)
	if u.PromptTokens != 30 || u.CompletionTokens != 40 {
		t.Fatalf("Set 应覆盖，实际 prompt=%d completion=%d", u.PromptTokens, u.CompletionTokens)
	}
}

// ---------- OpenAI 兼容流式 usage 提取 ----------

func TestOpenAICompatible_TransformStream_UsageSet(t *testing.T) {
	r := &OpenAICompatible{}
	// 模拟个别厂商多 chunk 携带 usage：取最后一个快照，不应累加
	stream := sseStreamFromLines(
		`{"id":"1","choices":[{"delta":{"content":"a"}}],"usage":{"prompt_tokens":10,"completion_tokens":5}}`,
		`{"id":"2","choices":[{"delta":{"content":"b"}}],"usage":{"prompt_tokens":10,"completion_tokens":9}}`,
		`{"id":"3","choices":[{"delta":{"content":"c"}}],"usage":{"prompt_tokens":10,"completion_tokens":20}}`,
		`[DONE]`,
	)
	usage := &UsageAccumulator{}
	w := &captureWriter{}
	if err := r.TransformStream(stream, w, usage); err != nil {
		t.Fatalf("TransformStream error: %v", err)
	}
	if usage.PromptTokens != 10 || usage.CompletionTokens != 20 {
		t.Fatalf("应取最后一个 usage 快照，实际 prompt=%d completion=%d", usage.PromptTokens, usage.CompletionTokens)
	}
	if !strings.Contains(w.buf.String(), "data: [DONE]") {
		t.Fatalf("输出应包含 [DONE]")
	}
}

func TestOpenAICompatible_TransformStream_EOFNoDone_ErrStreamIncomplete(t *testing.T) {
	r := &OpenAICompatible{}
	// 没有 [DONE] 直接 EOF → 应返回 ErrStreamIncomplete
	stream := sseStreamFromLines(`{"id":"1","choices":[{"delta":{"content":"a"}}]}`)
	usage := &UsageAccumulator{}
	w := &captureWriter{}
	if err := r.TransformStream(stream, w, usage); err != ErrStreamIncomplete {
		t.Fatalf("应返回 ErrStreamIncomplete，实际: %v", err)
	}
}

// ---------- Gemini 流式 usage 提取（累计快照） ----------

func TestGemini_TransformStream_UsageSet(t *testing.T) {
	r := &Gemini{}
	// Gemini 多个 chunk 携带 usageMetadata（累计值），必须取最后一个
	stream := sseStreamFromLines(
		`{"candidates":[{"content":{"parts":[{"text":"Hello"}]}}],"usageMetadata":{"promptTokenCount":100,"candidatesTokenCount":5}}`,
		`{"candidates":[{"content":{"parts":[{"text":" world"}]}}],"usageMetadata":{"promptTokenCount":100,"candidatesTokenCount":12}}`,
		`{"candidates":[{"content":{"parts":[{"text":"!"}]},"finishReason":"STOP"}],"usageMetadata":{"promptTokenCount":100,"candidatesTokenCount":15}}`,
	)
	usage := &UsageAccumulator{}
	w := &captureWriter{}
	if err := r.TransformStream(stream, w, usage); err != nil {
		t.Fatalf("TransformStream error: %v", err)
	}
	if usage.PromptTokens != 100 || usage.CompletionTokens != 15 {
		t.Fatalf("应取最后一个 usageMetadata 快照，实际 prompt=%d completion=%d", usage.PromptTokens, usage.CompletionTokens)
	}
	if !strings.Contains(w.buf.String(), "data: [DONE]") {
		t.Fatalf("输出应包含 [DONE]")
	}
}

func TestGemini_TransformStream_EOFNoFinish_ErrStreamIncomplete(t *testing.T) {
	r := &Gemini{}
	// candidates 始终无 finishReason，直接 EOF → 不完整
	stream := sseStreamFromLines(
		`{"candidates":[{"content":{"parts":[{"text":"Hi"}]}}],"usageMetadata":{"promptTokenCount":5,"candidatesTokenCount":1}}`,
	)
	usage := &UsageAccumulator{}
	w := &captureWriter{}
	if err := r.TransformStream(stream, w, usage); err != ErrStreamIncomplete {
		t.Fatalf("应返回 ErrStreamIncomplete，实际: %v", err)
	}
}

// ---------- Anthropic 流式 usage 提取（覆盖模型） ----------

func TestAnthropic_TransformStream_UsageSet(t *testing.T) {
	r := &Anthropic{}
	// message_start 占位 output_tokens=1，后续 message_delta 最终 output_tokens=50
	stream := sseStreamFromLines(
		`{"type":"message_start","message":{"usage":{"input_tokens":25,"output_tokens":1}}}`,
		`{"type":"content_block_delta","delta":{"type":"text_delta","text":"Hello"}}`,
		`{"type":"usage","usage":{"input_tokens":25,"output_tokens":50}}`,
		`{"type":"message_delta","usage":{"input_tokens":25,"output_tokens":50}}`,
		`{"type":"message_stop"}`,
	)
	usage := &UsageAccumulator{}
	w := &captureWriter{}
	if err := r.TransformStream(stream, w, usage); err != nil {
		t.Fatalf("TransformStream error: %v", err)
	}
	// 覆盖模型：不应得到 1+50+50=101，而应是最后一次快照 50
	if usage.PromptTokens != 25 || usage.CompletionTokens != 50 {
		t.Fatalf("应取最后的 usage 快照（覆盖模型），实际 prompt=%d completion=%d", usage.PromptTokens, usage.CompletionTokens)
	}
	if !strings.Contains(w.buf.String(), "data: [DONE]") {
		t.Fatalf("输出应包含 [DONE]")
	}
}

func TestAnthropic_TransformStream_EOFNoStop_ErrStreamIncomplete(t *testing.T) {
	r := &Anthropic{}
	// 只有 message_start，无 message_stop 直接 EOF → 不完整
	stream := sseStreamFromLines(
		`{"type":"message_start","message":{"usage":{"input_tokens":5,"output_tokens":1}}}`,
		`{"type":"content_block_delta","delta":{"type":"text_delta","text":"Hi"}}`,
	)
	usage := &UsageAccumulator{}
	w := &captureWriter{}
	if err := r.TransformStream(stream, w, usage); err != ErrStreamIncomplete {
		t.Fatalf("应返回 ErrStreamIncomplete，实际: %v", err)
	}
}

// ---------- 非流式响应提取保持 Add（单次完整值） ----------

func TestOpenAICompatible_TransformResponse_UsageAdd(t *testing.T) {
	r := &OpenAICompatible{}
	body := []byte(`{"id":"1","object":"chat.completion","usage":{"prompt_tokens":10,"completion_tokens":20,"total_tokens":30}}`)
	usage := &UsageAccumulator{}
	if _, err := r.TransformResponse(body, usage); err != nil {
		t.Fatalf("TransformResponse error: %v", err)
	}
	if usage.PromptTokens != 10 || usage.CompletionTokens != 20 {
		t.Fatalf("非流式应提取 usage，实际 prompt=%d completion=%d", usage.PromptTokens, usage.CompletionTokens)
	}
}

func TestGemini_TransformResponse_UsageAdd(t *testing.T) {
	r := &Gemini{}
	body := []byte(`{"candidates":[{"content":{"parts":[{"text":"hi"}]},"finishReason":"STOP"}],"usageMetadata":{"promptTokenCount":7,"candidatesTokenCount":3}}`)
	usage := &UsageAccumulator{}
	if _, err := r.TransformResponse(body, usage); err != nil {
		t.Fatalf("TransformResponse error: %v", err)
	}
	if usage.PromptTokens != 7 || usage.CompletionTokens != 3 {
		t.Fatalf("非流式应提取 usageMetadata，实际 prompt=%d completion=%d", usage.PromptTokens, usage.CompletionTokens)
	}
}

func TestAnthropic_TransformResponse_UsageAdd(t *testing.T) {
	r := &Anthropic{}
	body := []byte(`{"id":"msg_1","content":[{"type":"text","text":"hi"}],"model":"claude-3","usage":{"input_tokens":9,"output_tokens":4},"stop_reason":"end_turn"}`)
	usage := &UsageAccumulator{}
	if _, err := r.TransformResponse(body, usage); err != nil {
		t.Fatalf("TransformResponse error: %v", err)
	}
	if usage.PromptTokens != 9 || usage.CompletionTokens != 4 {
		t.Fatalf("非流式应提取 usage，实际 prompt=%d completion=%d", usage.PromptTokens, usage.CompletionTokens)
	}
}

// ---------- 确保 TransformStream 返回的 HTTP 请求体可被消费 ----------

func TestOpenAICompatible_BuildChatRequest_ReadableBody(t *testing.T) {
	r := &OpenAICompatible{}
	req, err := r.BuildChatRequest(context.Background(), makeChannel(), openAIReqBody(true, nil), 0)
	if err != nil {
		t.Fatalf("BuildChatRequest error: %v", err)
	}
	if req.Method != http.MethodPost {
		t.Fatalf("method: %s", req.Method)
	}
	if req.URL.Path != "/v1/chat/completions" {
		t.Fatalf("path: %s", req.URL.Path)
	}
}

// ---------- joinAPIV1URL 版本号保留测试 ----------

func TestJoinAPIV1URL_KeepsExistingVersion(t *testing.T) {
	cases := []struct {
		name string
		base string
		want string
	}{
		{"无版本号自动补 v1", "https://api.example.com", "https://api.example.com/v1/models"},
		{"保留 v1", "https://api.example.com/v1", "https://api.example.com/v1/models"},
		{"保留 v2", "https://api.example.com/v2", "https://api.example.com/v2/models"},
		{"保留 v3", "https://api.example.com/v3", "https://api.example.com/v3/models"},
		{"保留多位数 v10", "https://api.example.com/v10", "https://api.example.com/v10/models"},
		{"v2 尾斜杠", "https://api.example.com/v2/", "https://api.example.com/v2/models"},
		{"大写 V1", "https://api.example.com/V1", "https://api.example.com/V1/models"},
		{"v1beta 不误判补 v1", "https://api.example.com/v1beta", "https://api.example.com/v1beta/v1/models"},
		{"带前缀路径保留 v3", "https://api.example.com/api/v3", "https://api.example.com/api/v3/models"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := joinAPIV1URL(tc.base, "/models")
			if got != tc.want {
				t.Fatalf("joinAPIV1URL(%q, \"/models\") = %q, want %q", tc.base, got, tc.want)
			}
		})
	}
}

func TestHasVersionSuffix(t *testing.T) {
	cases := []struct {
		base string
		want bool
	}{
		{"https://api.example.com", false},
		{"https://api.example.com/", false},
		{"https://api.example.com/v1", true},
		{"https://api.example.com/v2", true},
		{"https://api.example.com/v10", true},
		{"https://api.example.com/v1beta", false},
		{"https://api.example.com/version", false},
		{"https://api.example.com/v", false},
		{"https://api.example.com/api/v3", true},
	}
	for _, tc := range cases {
		t.Run(tc.base, func(t *testing.T) {
			if got := hasVersionSuffix(tc.base); got != tc.want {
				t.Fatalf("hasVersionSuffix(%q) = %v, want %v", tc.base, got, tc.want)
			}
		})
	}
}
