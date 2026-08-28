//go:build !cli

package main

import (
	"embed"
	"log"
	"os"

	"aiproxy/internal/app"
	"aiproxy/internal/singleinst"
	"aiproxy/internal/wailsapp"
)

// 系统托盘图标：采用仓库根目录 assets/aiproxy.png。
// 受 go:embed 限制（不能引用包目录之外的文件），在包 main（位于仓库根）中以目录形式嵌入 assets/，
// 运行时取出 assets/aiproxy.png 的字节传入 wailsapp；Windows 下 internal/tray 会将其转换为 .ico 加载，
// 其他平台直接供 systray 使用。读取失败时不致命，托盘回退默认图标。
//
//go:embed assets
var assetFS embed.FS

// main Wails 版桌面 GUI 入口（默认构建，无标签）。
// CLI 版本编译方式：go build -tags cli
func main() {
	log.SetFlags(log.LstdFlags | log.Lshortfile)

	// 单实例锁：同名进程已运行时，通知已有实例显示主窗口并退出当前进程，避免多开
	inst, err := singleinst.TryLock("desktop")
	if err != nil {
		if err == singleinst.ErrAlreadyRunning {
			singleinst.ShowAlreadyRunningMessage()
			log.Printf("[main] 已有 AIProxy 实例在运行，已通知其显示主窗口，当前进程退出")
		} else {
			log.Printf("[main] 单实例锁检查失败: %v", err)
		}
		os.Exit(0)
	}
	// 注意：不在此处 defer inst.Release()，
	// 正常退出路径（GUI 关闭）中显式释放。

	// 数据库路径：优先使用环境变量，默认可执行文件同目录 aiproxy.db
	dbPath := app.DBPath()

	// 初始化数据库与全部核心组件（数据访问层、代理服务、模型同步服务）
	comps, err := app.New(dbPath)
	if err != nil {
		log.Fatalf("[main] %v", err)
	}

	// 后台定时清理过期请求日志（保留天数可在设置中配置，默认 365 天）
	comps.StartLogCleaner()

	// 启动 Wails GUI（自动启动代理服务与模型同步）
	// 读取系统托盘图标字节：取 assets/aiproxy.png；失败则不传图标（托盘回退默认图标，非致命）
	trayIcon, err := assetFS.ReadFile("assets/aiproxy.png")
	if err != nil {
		log.Printf("[main] 读取托盘图标 assets/aiproxy.png 失败: %v", err)
	}
	wailsapp.Run(comps, inst, trayIcon)

	// wails.Run 返回后正常退出
	comps.Close()
}
