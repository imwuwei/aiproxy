//go:build !windows

package singleinst

import "log"

// init 注册非 Windows 平台的提示实现。
// Linux/macOS 桌面环境无统一的原生弹窗 API，使用日志输出提示。
func init() {
	warnAlreadyRunning = func() {
		log.Printf("[singleinst] AIProxy 已在运行，请勿重复运行。")
	}
}
