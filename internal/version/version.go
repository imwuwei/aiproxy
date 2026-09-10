// Package version 提供编译时注入的应用版本信息。
//
// 版本信息通过构建时 -ldflags "-X" 注入，无需改动源码即可在每次构建时
// 自动写入当前 tag 版本号、构建时间与 Git 提交哈希：
//
//	go build -ldflags "\
//		-X aiproxy/internal/version.Version=v1.2.3 \
//		-X aiproxy/internal/version.BuildTime=2026-09-11\ 10:00:00 \
//		-X aiproxy/internal/version.GitCommit=abc1234" .
//
// 未注入（如 go run 或未配置 ldflags 的构建）时使用默认占位值，
// 前端设置页底部与 CLI 的 version 命令统一通过本包读取。
package version

var (
	// Version 语义化版本号（约定带 v 前缀，如 v0.1.3）。
	// 未注入时回退为 "dev"（开发构建）。
	Version = "dev"

	// BuildTime 构建时间（UTC，格式 YYYY-MM-DD HH:MM:SS）。
	BuildTime = "unknown"

	// GitCommit 构建时的 Git 短提交哈希。
	GitCommit = "unknown"
)

// Info 汇总一份完整的版本信息。
type Info struct {
	Version   string `json:"version"`
	BuildTime string `json:"build_time"`
	GitCommit string `json:"git_commit"`
}

// Get 返回当前版本信息。
func Get() Info {
	return Info{
		Version:   Version,
		BuildTime: BuildTime,
		GitCommit: GitCommit,
	}
}
