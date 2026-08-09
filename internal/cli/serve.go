package cli

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
)

// runServe 前台启动代理服务：
// 启动代理服务（HTTP）与模型同步定时任务，以及后台日志清理；
// 监听 SIGINT/SIGTERM，收到信号后优雅停止（先停模型同步，再停代理，最后关闭数据库）。
func runServe(dbPath string, args []string) error {
	fs := newFlagSet("serve", "serve [--db <path>]")
	if err := fs.Parse(args); err != nil {
		return err
	}

	comps, err := openApp(dbPath)
	if err != nil {
		return err
	}
	defer comps.Close()

	cfg := comps.LoadConfig()
	log.Printf("[cli] 数据库已加载: %s", dbPath)
	comps.StartLogCleaner()

	if err := comps.ProxySrv.Start(); err != nil {
		return fmt.Errorf("启动代理服务失败: %w", err)
	}
	comps.ModelSync.Start()
	log.Printf("[cli] 模型同步服务已启动（间隔 %v）", comps.ModelSync.RefreshInterval())
	log.Printf("[cli] 代理服务运行中: http://%s（Ctrl+C 停止）", cfg.ProxyAddr())

	// 等待退出信号，优雅停止
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	<-ctx.Done()

	log.Printf("[cli] 收到退出信号，正在停止服务...")
	comps.ModelSync.Stop()
	if err := comps.ProxySrv.Stop(); err != nil {
		log.Printf("[cli] 停止代理服务失败: %v", err)
	}
	log.Printf("[cli] 已退出")
	return nil
}
