package proxy

import (
	"encoding/json"
	"net/http"
)

// openAIError OpenAI 格式错误响应
type openAIError struct {
	Error apiErrorBody `json:"error"`
}

type apiErrorBody struct {
	Message string `json:"message"`
	Type    string `json:"type"`
	Code    string `json:"code,omitempty"`
}

// writeOpenAIError 写出 OpenAI 格式错误
func writeOpenAIError(w http.ResponseWriter, status int, errType, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(openAIError{
		Error: apiErrorBody{
			Message: message,
			Type:    errType,
			Code:    errType,
		},
	})
}
