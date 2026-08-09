//go:build !windows

package tray

import (
	"github.com/getlantern/systray"
)

// run 非 Windows 平台回退到 getlantern/systray。
// 该库无法区分图标左键/右键（点击均弹出菜单），因此 LeftClick 不生效。
func run(opts Options) {
	systray.Run(func() {
		if len(opts.Icon) > 0 {
			systray.SetIcon(opts.Icon)
		}
		if opts.Title != "" {
			systray.SetTitle(opts.Title)
		}
		if opts.Tooltip != "" {
			systray.SetTooltip(opts.Tooltip)
		}

		mShow := systray.AddMenuItem("显示主窗口", "显示 AIProxy 主窗口")
		systray.AddSeparator()
		mQuit := systray.AddMenuItem("退出", "退出 AIProxy")

		go func() {
			for {
				select {
				case <-mShow.ClickedCh:
					if opts.Show != nil {
						opts.Show()
					}
				case <-mQuit.ClickedCh:
					if opts.Quit != nil {
						opts.Quit()
					}
					systray.Quit()
					return
				}
			}
		}()
	}, nil)
}

// quit 请求退出托盘。
func quit() {
	systray.Quit()
}
