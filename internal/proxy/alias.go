package proxy

import (
	"bytes"
	"encoding/json"
	"net/http"

	"aiproxy/internal/models"
)

// maxWeightSeqLen 加权轮询展开序列长度上限，防止极端权重导致内存膨胀。
const maxWeightSeqLen = 1000

// resolveAlias 检查 model 是否为已启用的模型别名。
// 返回 (nil, nil) 表示普通模型，走原有路由逻辑。
// 未找到、未启用、无有效目标或目标解析失败时同样返回 (nil, nil)。
func (s *Server) resolveAlias(model string) (*models.ModelAlias, models.ModelAliasTargetList) {
	if s.aliases == nil {
		return nil, nil
	}
	a, err := s.aliases.GetByName(model)
	if err != nil || a == nil || !a.Enabled {
		return nil, nil
	}
	targets, err := a.ParseTargets()
	if err != nil {
		return nil, nil
	}
	valid := make(models.ModelAliasTargetList, 0, len(targets))
	for _, t := range targets {
		if t.Model == "" {
			continue
		}
		if t.Weight < 1 {
			t.Weight = 1
		}
		valid = append(valid, t)
	}
	if len(valid) == 0 {
		return nil, nil
	}
	return a, valid
}

// weightedSeq 将目标列表按权重展开为轮询序列。
// 例如 [{gpt-4o,2},{claude,1}] → [gpt-4o,gpt-4o,claude]。
func weightedSeq(targets models.ModelAliasTargetList) []string {
	var seq []string
	total := 0
	for _, t := range targets {
		if total >= maxWeightSeqLen {
			break
		}
		n := t.Weight
		if n > maxWeightSeqLen-total {
			n = maxWeightSeqLen - total
		}
		for i := 0; i < n; i++ {
			seq = append(seq, t.Model)
		}
		total += n
	}
	if len(seq) == 0 {
		for _, t := range targets {
			seq = append(seq, t.Model)
		}
	}
	return seq
}

// nextAliasIndex 轮询获取别名的起始目标下标（加权轮询起点）。
func (s *Server) nextAliasIndex(aliasName string) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.aliasIdx == nil {
		s.aliasIdx = make(map[string]int64)
	}
	idx := s.aliasIdx[aliasName]
	s.aliasIdx[aliasName] = idx + 1
	return int(idx)
}

// mergeAliases 将已启用的模型别名合入模型列表，供 GET /v1/models 对外展示。
// 别名与现有模型重名时跳过（保留真实模型）；aliases 为 nil 时直接返回原列表。
func (s *Server) mergeAliases(modelList []string) []string {
	if s.aliases == nil {
		return modelList
	}
	aliasNames := s.aliases.ListEnabledNames()
	if len(aliasNames) == 0 {
		return modelList
	}
	set := make(map[string]struct{}, len(modelList)+len(aliasNames))
	for _, m := range modelList {
		set[m] = struct{}{}
	}
	merged := modelList
	for _, a := range aliasNames {
		if _, dup := set[a]; dup {
			continue
		}
		set[a] = struct{}{}
		merged = append(merged, a)
	}
	return merged
}

// rewriteModel 将请求体中的 model 字段替换为目标模型名。
// 用于别名请求：body 携带别名，向上游转发前改写为真实模型名。
func rewriteModel(body []byte, to string) ([]byte, error) {
	var m map[string]json.RawMessage
	if err := json.Unmarshal(body, &m); err != nil {
		return nil, err
	}
	modelJSON, err := json.Marshal(to)
	if err != nil {
		return nil, err
	}
	m["model"] = modelJSON
	return json.Marshal(m)
}

// modelRestoreWriter 响应写入包装器：将上游返回的真实模型名还原为别名。
// 同时兼容过滤流式（SSE）与非流式 JSON 响应。
type modelRestoreWriter struct {
	http.ResponseWriter
	realModel string
	alias     string
}

// Write 替换响应字节中的 "model":"<真实模型>" 为 "model":"<别名>"。
// 覆盖 OpenAI 标准紧凑输出与带空格两种常见格式，精确匹配不误伤。
func (w *modelRestoreWriter) Write(p []byte) (int, error) {
	seps := [][]byte{[]byte(`"model":"`), []byte(`"model": "`)}
	for _, sep := range seps {
		p = bytes.ReplaceAll(p, append(append([]byte{}, sep...), []byte(w.realModel+`"`)...), append(append([]byte{}, sep...), []byte(w.alias+`"`)...))
	}
	return w.ResponseWriter.Write(p)
}

// Flush 转发 Flush，保证 SSE 流式场景下 http.Flusher 断言成立。
func (w *modelRestoreWriter) Flush() {
	if f, ok := w.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}
