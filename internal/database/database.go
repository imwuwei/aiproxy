package database

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"time"

	_ "modernc.org/sqlite"

	"aiproxy/internal/config"
	"aiproxy/internal/models"
)

// DB 数据库封装
type DB struct {
	*sql.DB
}

// Open 打开（或创建）SQLite 数据库并初始化 schema
func Open(path string) (*DB, error) {
	if path == "" {
		path = "aiproxy.db"
	}
	// 确保目录存在
	if dir := filepath.Dir(path); dir != "." && dir != "" {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return nil, fmt.Errorf("创建数据目录失败: %w", err)
		}
	}

	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("打开数据库失败: %w", err)
	}
	// 连接池设置：SQLite 单写者，限制并发
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	db.SetConnMaxLifetime(0)

	// WAL 模式提升并发
	if _, err := db.Exec(`PRAGMA journal_mode=WAL;`); err != nil {
		return nil, fmt.Errorf("启用 WAL 失败: %w", err)
	}
	if _, err := db.Exec(`PRAGMA busy_timeout=5000;`); err != nil {
		return nil, fmt.Errorf("设置 busy_timeout 失败: %w", err)
	}

	d := &DB{DB: db}
	if err := d.migrate(); err != nil {
		return nil, err
	}
	if err := d.seedDefaults(); err != nil {
		return nil, err
	}
	return d, nil
}

// migrate 建表迁移
func (d *DB) migrate() error {
	schema := `
CREATE TABLE IF NOT EXISTS channels (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	name TEXT NOT NULL,
	type TEXT NOT NULL,
	base_url TEXT NOT NULL,
	api_keys TEXT NOT NULL DEFAULT '[]',
	priority INTEGER NOT NULL DEFAULT 0,
	enabled INTEGER NOT NULL DEFAULT 1,
	status TEXT NOT NULL DEFAULT 'offline',
	last_error TEXT NOT NULL DEFAULT '',
	last_success_at DATETIME,
	created_at DATETIME NOT NULL,
	updated_at DATETIME NOT NULL
);

CREATE TABLE IF NOT EXISTS channel_models (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	channel_id INTEGER NOT NULL,
	model TEXT NOT NULL,
	created_at DATETIME NOT NULL,
	UNIQUE(channel_id, model)
);
CREATE INDEX IF NOT EXISTS idx_channel_models_model ON channel_models(model);
CREATE INDEX IF NOT EXISTS idx_channel_models_channel ON channel_models(channel_id);

CREATE TABLE IF NOT EXISTS custom_models (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	name TEXT NOT NULL UNIQUE,
	description TEXT NOT NULL DEFAULT '',
	created_at DATETIME NOT NULL,
	updated_at DATETIME NOT NULL
);

CREATE TABLE IF NOT EXISTS usage_records (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	created_at DATETIME NOT NULL,
	channel_id INTEGER NOT NULL,
	channel_name TEXT NOT NULL DEFAULT '',
	model TEXT NOT NULL,
	prompt_tokens INTEGER NOT NULL DEFAULT 0,
	completion_tokens INTEGER NOT NULL DEFAULT 0,
	total_tokens INTEGER NOT NULL DEFAULT 0,
	is_success INTEGER NOT NULL DEFAULT 1,
	status_code INTEGER NOT NULL DEFAULT 200,
	duration_ms INTEGER NOT NULL DEFAULT 0,
	error TEXT NOT NULL DEFAULT ''
);
CREATE INDEX IF NOT EXISTS idx_usage_created_at ON usage_records(created_at);
CREATE INDEX IF NOT EXISTS idx_usage_channel ON usage_records(channel_id);
CREATE INDEX IF NOT EXISTS idx_usage_model ON usage_records(model);

CREATE TABLE IF NOT EXISTS model_aliases (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	name TEXT NOT NULL UNIQUE,
	targets TEXT NOT NULL DEFAULT '[]',
	enabled INTEGER NOT NULL DEFAULT 1,
	created_at DATETIME NOT NULL,
	updated_at DATETIME NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_model_aliases_name ON model_aliases(name);

CREATE TABLE IF NOT EXISTS settings (
	key TEXT PRIMARY KEY,
	value TEXT NOT NULL
);
`
	if _, err := d.Exec(schema); err != nil {
		return fmt.Errorf("建表失败: %w", err)
	}

	// 增量迁移：为已有 channel_models 表添加 source 列（存量库升级）
	if err := d.migrateChannelModelsSource(); err != nil {
		return fmt.Errorf("迁移 channel_models.source 失败: %w", err)
	}
	return nil
}

// migrateChannelModelsSource 为 channel_models 表添加 source 列（如不存在）。
// source 取值：sync（自动同步）、custom（手动添加）、excluded（用户排除的同步绑定）。
func (d *DB) migrateChannelModelsSource() error {
	var hasSource bool
	row := d.QueryRow(`SELECT COUNT(*) FROM pragma_table_info('channel_models') WHERE name = 'source'`)
	if err := row.Scan(&hasSource); err != nil {
		return err
	}
	if hasSource {
		return nil
	}
	if _, err := d.Exec(`ALTER TABLE channel_models ADD COLUMN source TEXT NOT NULL DEFAULT 'sync'`); err != nil {
		return err
	}
	if _, err := d.Exec(`CREATE INDEX IF NOT EXISTS idx_channel_models_source ON channel_models(source)`); err != nil {
		return err
	}
	return nil
}

// seedDefaults 写入默认设置（仅当不存在时）
func (d *DB) seedDefaults() error {
	def := config.DefaultConfig()
	// 首次初始化时访问令牌使用安全随机值，避免使用可预测的静态默认值
	accessToken := config.GenerateAccessToken()
	defaults := map[string]string{
		models.SettingsListenAddr:          def.ListenAddr,
		models.SettingsListenPort:          strconv.Itoa(def.ListenPort),
		models.SettingsAccessToken:         accessToken,
		models.SettingsAuthEnabled:         strconv.FormatBool(def.AuthEnabled),
		models.SettingsModelSyncInterval:   strconv.FormatInt(int64(def.ModelSyncInterval.Minutes()), 10),
		models.SettingsProxyTimeoutSeconds: strconv.FormatInt(int64(def.ProxyTimeout.Seconds()), 10),
		models.SettingsBreakerThreshold:    strconv.Itoa(def.BreakerThreshold),
		models.SettingsBreakerCooldownSec:  strconv.FormatInt(int64(def.BreakerCooldown.Seconds()), 10),
		models.SettingsDebug:               strconv.FormatBool(def.Debug),
	}
	for k, v := range defaults {
		if _, err := d.Exec(
			`INSERT OR IGNORE INTO settings (key, value) VALUES (?, ?)`, k, v,
		); err != nil {
			return fmt.Errorf("写入默认设置 %s 失败: %w", k, err)
		}
	}
	return nil
}

// Now 返回当前时间（统一时间源）
func Now() time.Time {
	return time.Now()
}
