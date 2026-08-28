//go:build !cli

package wailsapp

import (
	"context"
	"embed"
	"log"

	"aiproxy/internal/app"
	"aiproxy/internal/singleinst"
	"aiproxy/internal/tray"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	"github.com/wailsapp/wails/v2/pkg/options/windows"
	"github.com/wailsapp/wails/v2/pkg/runtime"
)

// 前端静态资源目录：frontend/ 下的文件在构建时嵌入二进制。
//
//go:embed all:frontend
var assets embed.FS

// Run 启动 Wails GUI。
// comps 由 main 初始化；trayIcon 为系统托盘图标字节（PNG，取自仓库根目录 assets/aiproxy.png，
// 由 main 包嵌入后传入；Windows 下 internal/tray 会转换为 .ico 加载，其他平台直接用于 systray）。
func Run(comps *app.Components, inst *singleinst.Instance, trayIcon []byte) {
	appGUI := NewApp(comps.Config, comps.SettingsStore, comps.ChannelStore, comps.ModelStore, comps.CustomModelStore, comps.UsageStore, comps.AliasStore, comps.ProxySrv, comps.ModelSync)
	appGUI.SetSingleInstance(inst)

	// 组装 Wails 应用
	// 启动时默认最小化：开启后主窗口启动即隐藏到系统托盘（任务栏不显示），
	// 仅保留托盘图标，点击托盘或菜单"显示主窗口"可恢复。
	err := wails.Run(&options.App{
		Title:             "AIProxy - OpenAI API 代理",
		Width:             1180,
		Height:            720,
		MinWidth:          900,
		MinHeight:         600,
		HideWindowOnClose: true, // 关闭窗口隐藏到托盘，继续后台运行
		StartHidden:       comps.Config.StartMinimized,
		AssetServer: &assetserver.Options{
			Assets: assets,
		},
		BackgroundColour:         options.NewRGBA(0xf5, 0xf5, 0xf5, 0xff),
		EnableDefaultContextMenu: true,
		OnStartup: func(ctx context.Context) {
			appGUI.init(ctx)

			// 启动系统托盘（独立 goroutine）
			// Windows 托盘：左键单击显示主界面，右键弹出功能菜单（显示主窗口/退出）。
			// 其他平台回退至 menu 风格托盘。
			go tray.Run(tray.Options{
				Icon:      trayIcon,
				Title:     "AIProxy",
				Tooltip:   "AIProxy - OpenAI API 代理",
				LeftClick: func() { runtime.WindowShow(ctx) },
				Show:      func() { runtime.WindowShow(ctx) },
				Quit: func() {
					// 停止服务并退出
					appGUI.modelSync.Stop()
					_ = appGUI.proxySrv.Stop()
					if appGUI.instance != nil {
						appGUI.instance.Release()
					}
					tray.Quit()
					runtime.Quit(ctx)
				},
			})
		},
		OnShutdown: func(ctx context.Context) {
			appGUI.shutdown(ctx)
		},
		Bind: []interface{}{
			appGUI,
		},
		Windows: &windows.Options{
			Theme: windows.SystemDefault,
		},
	})

	if err != nil {
		log.Printf("[wailsapp] Wails 运行失败: %v", err)
	}
}
