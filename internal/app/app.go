// Package app 提供 GUI 版与 CLI 版共用的应用引导逻辑：
// 数据库打开、数据访问层初始化、代理服务与模型同步服务创建、后台日志清理。
package app

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"time"

	"aiproxy/internal/config"
	"aiproxy/internal/database"
	"aiproxy/internal/modelsync"
	"aiproxy/internal/proxy"
	"aiproxy/internal/store"
)

// Components 应用运行时组件（GUI 版与 CLI 版共用）
type Components struct {
	DB               *database.DB
	ChannelStore     *store.ChannelStore
	ModelStore       *store.ChannelModelStore
	CustomModelStore *store.CustomModelStore
	UsageStore       *store.UsageStore
	SettingsStore    *store.SettingsStore
	AliasStore       *store.ModelAliasStore
	ProxySrv         *proxy.Server
	ModelSync        *modelsync.Service
	// Config 初始化时加载的配置（cfgLoader 在加载失败时回退到该值）
	Config *config.Config
}

// DBPath 解析数据库路径：
// 优先使用 AIPROXY_DB 环境变量，默认可执行文件同目录 aiproxy.db。
func DBPath() string {
	dbPath := os.Getenv("AIPROXY_DB")
	if dbPath == "" {
		exe, err := os.Executable()
		if err == nil {
			dbPath = filepath.Join(filepath.Dir(exe), "aiproxy.db")
		} else {
			dbPath = "aiproxy.db"
		}
	}
	return dbPath
}

// New 打开数据库并初始化全部数据访问层与核心服务。
// 返回的 Components 在程序退出前应调用 Close。
func New(dbPath string) (*Components, error) {
	if dbPath == "" {
		dbPath = DBPath()
	}
	db, err := database.Open(dbPath)
	if err != nil {
		return nil, fmt.Errorf("初始化数据库失败: %w", err)
	}

	channelStore := store.NewChannelStore(db.DB)
	modelStore := store.NewChannelModelStore(db.DB)
	customModelStore := store.NewCustomModelStore(db.DB)
	usageStore := store.NewUsageStore(db.DB)
	settingsStore := store.NewSettingsStore(db.DB)
	aliasStore := store.NewModelAliasStore(db.DB)

	// 初始配置：加载失败视为致命错误（数据库不可用）
	cfg, err := settingsStore.Load()
	if err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("加载配置失败: %w", err)
	}

	// cfgLoader 在服务启动/重启时读取最新配置；加载失败回退到初始 cfg
	cfgLoader := func() *config.Config {
		c, err := settingsStore.Load()
		if err != nil {
			return cfg
		}
		return c
	}
	proxySrv := proxy.NewServer(cfgLoader, channelStore, modelStore, usageStore, aliasStore)
	syncService := modelsync.New(modelStore, channelStore, settingsStore)

	return &Components{
		DB:               db,
		ChannelStore:     channelStore,
		ModelStore:       modelStore,
		CustomModelStore: customModelStore,
		UsageStore:       usageStore,
		SettingsStore:    settingsStore,
		AliasStore:       aliasStore,
		ProxySrv:         proxySrv,
		ModelSync:        syncService,
		Config:           cfg,
	}, nil
}

// Close 关闭数据库连接。可重复调用（后续调用仅记录日志）。
func (c *Components) Close() {
	if c == nil || c.DB == nil {
		return
	}
	if err := c.DB.Close(); err != nil {
		log.Printf("[app] 关闭数据库: %v", err)
	}
	c.DB = nil
}

// LoadConfig 重新读取最新配置；失败时回退到初始配置。
func (c *Components) LoadConfig() *config.Config {
	cfg, err := c.SettingsStore.Load()
	if err != nil {
		return c.Config
	}
	return cfg
}

// StartLogCleaner 后台定时清理过期请求日志。
// 保留天数从运行时配置读取（LogRetentionDays），默认 365 天；
// 每次清理时动态读取最新配置，修改后在下一个清理周期生效。
// 启动时立即清理一次，之后每小时清理一次；任何失败仅记录日志，不影响主程序。
func (c *Components) StartLogCleaner() {
	const cleanupInterval = time.Hour

	cleanup := func() {
		retentionDays := 365
		if cfg := c.LoadConfig(); cfg != nil && cfg.LogRetentionDays > 0 {
			retentionDays = cfg.LogRetentionDays
		}
		cutoff := time.Now().AddDate(0, 0, -retentionDays)
		n, err := c.UsageStore.DeleteBefore(cutoff)
		if err != nil {
			log.Printf("[cleaner] 清理过期请求日志失败: %v", err)
			return
		}
		if n > 0 {
			log.Printf("[cleaner] 已清理 %d 条超过 %d 天的请求日志", n, retentionDays)
		}
	}

	// 启动时立即清理一次，避免历史积压
	cleanup()

	go func() {
		ticker := time.NewTicker(cleanupInterval)
		defer ticker.Stop()
		for range ticker.C {
			cleanup()
		}
	}()
}
