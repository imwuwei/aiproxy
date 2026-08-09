package proxy

import (
	"encoding/json"
	"net/http"

	"aiproxy/internal/proxy/relay"
)

// handleModels 处理模型列表请求（GET /v1/models）
func (s *Server) handleModels(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeOpenAIError(w, http.StatusMethodNotAllowed, "method_not_allowed", "仅支持 GET 请求")
		return
	}

	modelList, err := s.models.ListAllModels()
	if err != nil {
		writeOpenAIError(w, http.StatusInternalServerError, "internal_error", "查询模型列表失败: "+err.Error())
		return
	}
	// 融合已启用的模型别名：别名可作为模型名对外提供
	modelList = s.mergeAliases(modelList)
	s.debugf("GET /v1/models 返回 %d 个模型", len(modelList))

	// OpenAI 格式响应
	type modelObj struct {
		ID      string `json:"id"`
		Object  string `json:"object"`
		Created int64  `json:"created"`
		OwnedBy string `json:"owned_by"`
	}
	data := make([]modelObj, 0, len(modelList))
	for _, m := range modelList {
		data = append(data, modelObj{
			ID:      m,
			Object:  "model",
			Created: relay.NowUnix(),
			OwnedBy: "aiproxy",
		})
	}
	resp := map[string]any{
		"object": "list",
		"data":   data,
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resp)
}
