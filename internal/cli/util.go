package cli

import (
	"encoding/json"
	"os"
	"strings"

	"aiproxy/internal/app"
)

// openApp 打开数据库并初始化组件（管理命令共用），调用方负责 Close。
func openApp(dbPath string) (*app.Components, error) {
	return app.New(dbPath)
}

// printJSON 输出 JSON（含 err 时输出 {"error": ...}）。
// 用于管理命令，输出到 stdout，便于脚本化处理。
func printJSON(v any) error {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}

// printJSONError 输出错误 JSON。
func printJSONError(err error) {
	_ = printJSON(map[string]string{"error": err.Error()})
}

// maskKeys 脱敏 API Keys：sk- 开头的保留前缀与末 4 位，其余保留首 2 位与末 2 位。
func maskKeys(keys []string) []string {
	out := make([]string, 0, len(keys))
	for _, k := range keys {
		out = append(out, maskKey(k))
	}
	return out
}

func maskKey(k string) string {
	if k == "" {
		return "(空)"
	}
	r := []rune(k)
	if len(r) <= 8 {
		return "****"
	}
	head, tail := 4, 4
	if strings.HasPrefix(k, "sk-") {
		head = 3 // 保留 "sk-" 前缀
		tail = 4
	}
	return string(r[:head]) + "****" + string(r[len(r)-tail:])
}

// joinKeys 拼接 API Keys（多 key 以逗号分隔，用于展示）。
func joinKeys(keys []string) string {
	return strings.Join(keys, ",")
}
