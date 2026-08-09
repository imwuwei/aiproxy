package models

// ModelSource 模型-渠道绑定的来源
type ModelSource string

const (
	// ModelSourceSync 自动同步拉取的绑定（可被覆盖）
	ModelSourceSync ModelSource = "sync"
	// ModelSourceCustom 用户手动添加的绑定（不受同步影响）
	ModelSourceCustom ModelSource = "custom"
	// ModelSourceExcluded 用户排除的同步绑定（同步时跳过，不再自动加回）
	ModelSourceExcluded ModelSource = "excluded"
)

// CustomModel 自定义模型元数据。
// 仅存储用户主动创建的模型信息（名称/描述），
// 渠道绑定关系仍存储在 channel_models 表中（source='custom'）。
type CustomModel struct {
	ID          int64  `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	CreatedAt   int64  `json:"created_at"`
	UpdatedAt   int64  `json:"updated_at"`
}

// ModelBinding 模型的渠道绑定信息（含来源标记与渠道状态）。
// 供 GUI/CLI 展示模型的渠道绑定关系时使用。
type ModelBinding struct {
	ChannelID      int64  `json:"channel_id"`
	ChannelName    string `json:"channel_name"`
	Source         string `json:"source"`          // sync | custom | excluded
	ChannelEnabled bool   `json:"channel_enabled"` // 渠道是否启用
}
