//go:build windows

package singleinst

import "golang.org/x/sys/windows"

// init 注册 Windows 原生弹窗提示：新实例重复运行时显示"请勿重复运行"。
func init() {
	warnAlreadyRunning = func() {
		title := windows.StringToUTF16Ptr("AIProxy")
		msg := windows.StringToUTF16Ptr("AIProxy 已在运行，请勿重复运行。")
		// MB_TOPMOST 保证弹窗置顶可见；MB_SETFOREGROUND 将弹窗带到前台
		_, _ = windows.MessageBox(0, msg, title, windows.MB_OK|windows.MB_ICONINFORMATION|windows.MB_TOPMOST|windows.MB_SETFOREGROUND)
	}
}
