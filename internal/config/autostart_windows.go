//go:build windows

package config

import (
	"fmt"
	"os"

	"golang.org/x/sys/windows/registry"
)

// runKeyPath 当前用户开机启动注册表键（HKCU，用户级无需管理员权限）。
// 与 NSIS 安装脚本中的 RUN_KEY 保持一致。
const runKeyPath = `Software\Microsoft\Windows\CurrentVersion\Run`

// runValueName 注册表值名，与 NSIS 安装脚本写入的名称一致。
const runValueName = "AIProxy"

// AutoStartSupported 当前平台是否支持开机自启动。
const AutoStartSupported = true

// IsAutoStartEnabled 读取当前用户的开机自启动注册表项。
// 返回 true 表示已启用自启动；注册表项不存在或读取失败返回 false。
func IsAutoStartEnabled() bool {
	k, err := registry.OpenKey(registry.CURRENT_USER, runKeyPath, registry.QUERY_VALUE)
	if err != nil {
		return false
	}
	defer k.Close()

	if _, _, err := k.GetStringValue(runValueName); err != nil {
		return false
	}
	return true
}

// SetAutoStartEnabled 设置或取消当前用户的开机自启动。
// 启用：写入 HKCU\...\Run 下名为 "AIProxy" 的字符串值，指向当前可执行文件。
//   - 幂等：注册表中已存在相同路径值时直接返回，不重复写入。
//
// 禁用：删除该注册表值（不存在时视为成功）。
func SetAutoStartEnabled(enabled bool) error {
	k, err := registry.OpenKey(registry.CURRENT_USER, runKeyPath, registry.SET_VALUE|registry.QUERY_VALUE)
	if err != nil {
		return fmt.Errorf("打开开机启动注册表失败: %w", err)
	}
	defer k.Close()

	if !enabled {
		// 删除值；值不存在时返回 ErrNotExist，视为已禁用，不算错误
		if err := k.DeleteValue(runValueName); err != nil && err != registry.ErrNotExist {
			return fmt.Errorf("关闭开机自启动失败: %w", err)
		}
		return nil
	}

	exe, err := os.Executable()
	if err != nil {
		return fmt.Errorf("获取程序路径失败: %w", err)
	}
	// 带引号包裹路径，防止路径含空格时启动失败
	value := `"` + exe + `"`

	// 幂等检查：注册表中已存在相同路径时直接返回，避免重复写入
	if cur, _, err := k.GetStringValue(runValueName); err == nil && cur == value {
		return nil
	}

	if err := k.SetStringValue(runValueName, value); err != nil {
		return fmt.Errorf("设置开机自启动失败: %w", err)
	}
	return nil
}
