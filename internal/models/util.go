package models

import "encoding/json"

// marshalJSON 序列化为 JSON 字符串
func marshalJSON(v any) string {
	b, err := json.Marshal(v)
	if err != nil {
		return "[]"
	}
	return string(b)
}

// unmarshalJSON 从 JSON 字符串解析
func unmarshalJSON[T any](s string) (T, error) {
	var v T
	if err := json.Unmarshal([]byte(s), &v); err != nil {
		return v, err
	}
	return v, nil
}
