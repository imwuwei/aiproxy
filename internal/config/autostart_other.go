//go:build !windows

package config

// AutoStartSupported 当前平台是否支持开机自启动。
// 目前仅 Windows 通过 HKCU Run 注册表实现，其他平台返回 false。
const AutoStartSupported = false

// IsAutoStartEnabled 非 Windows 平台不支持开机自启动，始终返回 false。
func IsAutoStartEnabled() bool {
	return false
}

// SetAutoStartEnabled 非 Windows 平台不支持开机自启动，返回错误提示。
func SetAutoStartEnabled(enabled bool) error {
	return ErrAutoStartUnsupported
}
