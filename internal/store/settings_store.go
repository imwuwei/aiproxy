package store

import (
	"database/sql"
	"strconv"
	"time"

	"aiproxy/internal/config"
	"aiproxy/internal/models"
)

// SettingsStore 设置数据访问
type SettingsStore struct {
	db *sql.DB
}

// NewSettingsStore 创建设置存储
func NewSettingsStore(db *sql.DB) *SettingsStore {
	return &SettingsStore{db: db}
}

// Get 获取单个设置值
func (s *SettingsStore) Get(key string) (string, error) {
	var v string
	err := s.db.QueryRow(`SELECT value FROM settings WHERE key = ?`, key).Scan(&v)
	if err == sql.ErrNoRows {
		return "", nil
	}
	return v, err
}

// Set 写入设置值
func (s *SettingsStore) Set(key, value string) error {
	_, err := s.db.Exec(`
		INSERT INTO settings (key, value) VALUES (?, ?)
		ON CONFLICT(key) DO UPDATE SET value = excluded.value`, key, value)
	return err
}

// Load 从数据库加载完整配置（缺失项使用默认值）
func (s *SettingsStore) Load() (*config.Config, error) {
	def := config.DefaultConfig()
	c := &config.Config{
		ListenAddr:        def.ListenAddr,
		ListenPort:        def.ListenPort,
		AccessToken:       def.AccessToken,
		AuthEnabled:       def.AuthEnabled,
		ModelSyncInterval: def.ModelSyncInterval,
		ProxyTimeout:      def.ProxyTimeout,
		BreakerThreshold:  def.BreakerThreshold,
		BreakerCooldown:   def.BreakerCooldown,
		LogRetentionDays:  def.LogRetentionDays,
		TokenDisplay:      def.TokenDisplay,
	}

	if v, err := s.Get(models.SettingsListenAddr); err == nil && v != "" {
		c.ListenAddr = v
	}
	if v, err := s.Get(models.SettingsListenPort); err == nil && v != "" {
		if n, e := strconv.Atoi(v); e == nil && n > 0 {
			c.ListenPort = n
		}
	}
	if v, err := s.Get(models.SettingsAccessToken); err == nil && v != "" {
		c.AccessToken = v
	}
	if v, err := s.Get(models.SettingsAuthEnabled); err == nil && v != "" {
		if b, e := strconv.ParseBool(v); e == nil {
			c.AuthEnabled = b
		}
	}
	if v, err := s.Get(models.SettingsModelSyncInterval); err == nil && v != "" {
		if n, e := strconv.Atoi(v); e == nil && n > 0 {
			c.ModelSyncInterval = time.Duration(n) * time.Minute
		}
	}
	if v, err := s.Get(models.SettingsProxyTimeoutSeconds); err == nil && v != "" {
		if n, e := strconv.Atoi(v); e == nil && n > 0 {
			c.ProxyTimeout = time.Duration(n) * time.Second
		}
	}
	if v, err := s.Get(models.SettingsBreakerThreshold); err == nil && v != "" {
		if n, e := strconv.Atoi(v); e == nil && n > 0 {
			c.BreakerThreshold = n
		}
	}
	if v, err := s.Get(models.SettingsBreakerCooldownSec); err == nil && v != "" {
		if n, e := strconv.Atoi(v); e == nil && n > 0 {
			c.BreakerCooldown = time.Duration(n) * time.Second
		}
	}
	if v, err := s.Get(models.SettingsLogRetentionDays); err == nil && v != "" {
		if n, e := strconv.Atoi(v); e == nil && n > 0 {
			c.LogRetentionDays = n
		}
	}
	if v, err := s.Get(models.SettingsDebug); err == nil && v != "" {
		if b, e := strconv.ParseBool(v); e == nil {
			c.Debug = b
		}
	}
	if v, err := s.Get(models.SettingsStartMinimized); err == nil && v != "" {
		if b, e := strconv.ParseBool(v); e == nil {
			c.StartMinimized = b
		}
	}
	if v, err := s.Get(models.SettingsTokenDisplay); err == nil && v != "" {
		if v == "auto" || v == "raw" {
			c.TokenDisplay = v
		}
	}
	return c, nil
}
