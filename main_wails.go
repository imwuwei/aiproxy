//go:build !cli

package main

import (
	"log"
	"os"

	"aiproxy/internal/app"
	"aiproxy/internal/singleinst"
	"aiproxy/internal/wailsapp"
)

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
	wailsapp.Run(comps, inst)

	// wails.Run 返回后正常退出
	comps.Close()
}
