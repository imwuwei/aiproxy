package models

// SettingsKey 设置键常量。
// 注意：每新增一个键，需同步在 SettingsStore.Load() 中加载。
const (
	SettingsListenAddr          = "listen_addr"
	SettingsListenPort          = "listen_port"
	SettingsAccessToken         = "access_token"
	SettingsAuthEnabled         = "auth_enabled"
	SettingsModelSyncInterval   = "model_sync_interval_minutes"
	SettingsProxyTimeoutSeconds = "proxy_timeout_seconds"
	SettingsBreakerThreshold    = "breaker_threshold"
	SettingsBreakerCooldownSec  = "breaker_cooldown_seconds"
	SettingsLogRetentionDays    = "log_retention_days"
	SettingsDebug               = "debug"
	SettingsStartMinimized      = "start_minimized"
	// SettingsTokenDisplay Token 数值显示方式：auto（≥100 万显示为 M）| raw（原始千分位）
	SettingsTokenDisplay = "token_display"
)

// Settings 键值对
type Settings struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}
