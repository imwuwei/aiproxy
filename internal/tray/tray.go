// Package tray 提供跨平台系统托盘封装。
// Windows 使用自研 Win32 实现（区分左键单击显示主窗口、右键弹出功能菜单）；
// 其他平台回退到 getlantern/systray（无独立左键事件，仅支持菜单操作）。
package tray

// Options 托盘配置。
type Options struct {
	// Icon 托盘图标内容（Windows 需要 .ico 格式字节）。
	Icon []byte
	// Title 托盘标题（部分平台展示，如 Linux）。
	Title string
	// Tooltip 悬停提示。
	Tooltip string

	// LeftClick 图标左键单击回调。
	// 注意：非 Windows 平台（systray 回退）无法区分左键，此回调不生效。
	LeftClick func()
	// Show 点击菜单"显示主窗口"时调用。
	Show func()
	// Quit 点击菜单"退出"时调用。
	Quit func()
}

// Run 启动托盘并进入事件循环，阻塞直到 Quit 被调用。
// 应放在独立 goroutine 中运行，避免阻塞调用方。
func Run(opts Options) {
	run(opts)
}

// Quit 请求退出托盘事件循环，Run 将返回。
func Quit() {
	quit()
}
