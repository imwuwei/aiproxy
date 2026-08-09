package models

import (
	"strconv"
	"strings"
	"time"
)

// ModelAlias 模型别名：将一个别名映射到多个真实模型。
// 客户端请求使用别名（如 "all"、"pro"），代理按权重/轮询将请求路由到目标模型，
// 实现"一个别名代理多个模型"的聚合与分发。
type ModelAlias struct {
	ID   int64  `json:"id"`
	Name string `json:"name"` // 别名，全局唯一，如 "all"
	// Targets 目标模型配置列表（JSON 字符串，内部使用）。
	// 结构见 ModelAliasTarget。使用 JSON 存储而非独立表，便于原子更新整个别名。
	Targets string `json:"targets"`
	// Enabled 是否启用。禁用后别名不作为路由目标，但保留配置。
	Enabled bool `json:"enabled"`
	// CreatedAt 创建时间（Unix 秒）。
	CreatedAt int64 `json:"created_at"`
	// UpdatedAt 最后更新时间（Unix 秒）。
	UpdatedAt int64 `json:"updated_at"`
}

// ModelAliasTarget 别名目标模型配置。
type ModelAliasTarget struct {
	Model   string `json:"model"`           // 真实模型名，如 "gpt-4o"
	Weight  int    `json:"weight"`          // 权重（>=1），按权重进行加权轮询
	Timeout int    `json:"timeout_seconds"` // 可选：单目标超时（秒），0 表示沿用全局超时
}

// ModelAliasTargetList 目标模型配置列表（用于 JSON 序列化的外部形态）。
type ModelAliasTargetList []ModelAliasTarget

// NewModelAlias 创建别名实体（不含持久化）。
func NewModelAlias(name string, targets []ModelAliasTarget, enabled bool) *ModelAlias {
	now := time.Now().Unix()
	return &ModelAlias{
		Name:      name,
		Targets:   marshalJSON(ModelAliasTargetList(targets)),
		Enabled:   enabled,
		CreatedAt: now,
		UpdatedAt: now,
	}
}

// ParseTargets 解析目标模型列表。
func (a *ModelAlias) ParseTargets() (ModelAliasTargetList, error) {
	return unmarshalJSON[ModelAliasTargetList](a.Targets)
}

// ParseAliasTargets 解析目标模型配置字符串（GUI/CLI 通用入口）。
func ParseAliasTargets(targets string) (ModelAliasTargetList, error) {
	return unmarshalJSON[ModelAliasTargetList](targets)
}

// Render 生成目标列表的展示文本，如 gpt-4o(2), claude-3-5-sonnet。
// 权重为 1 时省略权重显示。
func (l ModelAliasTargetList) Render() string {
	var sb strings.Builder
	for i, t := range l {
		if i > 0 {
			sb.WriteString(", ")
		}
		sb.WriteString(t.Model)
		if t.Weight > 1 {
			sb.WriteString("(")
			sb.WriteString(strconv.Itoa(t.Weight))
			sb.WriteString(")")
		}
	}
	return sb.String()
}
