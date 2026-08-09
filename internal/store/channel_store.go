package store

import (
	"database/sql"
	"fmt"
	"strings"
	"time"

	"aiproxy/internal/models"
)

// ChannelStore 渠道数据访问
type ChannelStore struct {
	db *sql.DB
}

// NewChannelStore 创建渠道存储
func NewChannelStore(db *sql.DB) *ChannelStore {
	return &ChannelStore{db: db}
}

const channelColumns = `id, name, type, base_url, api_keys, priority, enabled, status, last_error, last_success_at, created_at, updated_at`

// channelColumnsWithCount 在 channelColumns 基础上额外聚合出模型数（供 GUI/CLI 列表展示使用）。
// 子查询直接统计 channel_models 中该渠道的模型记录数，避免 N+1 查询。
const channelColumnsWithCount = `c.id, c.name, c.type, c.base_url, c.api_keys, c.priority, c.enabled, c.status, c.last_error, c.last_success_at, c.created_at, c.updated_at, (SELECT COUNT(*) FROM channel_models cm WHERE cm.channel_id = c.id) AS model_count`

// scanChannel 扫描不带模型数的渠道行（路由内部使用，避免子查询开销）。
func scanChannel(row interface{ Scan(...any) error }) (*models.Channel, error) {
	var c models.Channel
	var apiKeys string
	var enabled int
	var lastSuccessAt sql.NullTime
	var createdAt, updatedAt string

	if err := row.Scan(&c.ID, &c.Name, &c.Type, &c.BaseURL, &apiKeys, &c.Priority, &enabled, &c.Status, &c.LastError, &lastSuccessAt, &createdAt, &updatedAt); err != nil {
		return nil, err
	}
	return fillChannel(&c, apiKeys, enabled, lastSuccessAt, createdAt, updatedAt)
}

// scanChannelWithCount 扫描带模型数的渠道行（List/Get 使用）。
func scanChannelWithCount(row interface{ Scan(...any) error }) (*models.Channel, error) {
	var c models.Channel
	var apiKeys string
	var enabled int
	var lastSuccessAt sql.NullTime
	var createdAt, updatedAt string
	var modelCount int

	if err := row.Scan(&c.ID, &c.Name, &c.Type, &c.BaseURL, &apiKeys, &c.Priority, &enabled, &c.Status, &c.LastError, &lastSuccessAt, &createdAt, &updatedAt, &modelCount); err != nil {
		return nil, err
	}
	ch, err := fillChannel(&c, apiKeys, enabled, lastSuccessAt, createdAt, updatedAt)
	if err != nil {
		return nil, err
	}
	ch.ModelCount = modelCount
	return ch, nil
}

// fillChannel 将原始扫描字段解析填充到渠道对象。
func fillChannel(c *models.Channel, apiKeys string, enabled int, lastSuccessAt sql.NullTime, createdAt, updatedAt string) (*models.Channel, error) {
	c.Enabled = enabled == 1
	c.APIKeys = []string{}
	if err := c.SetAPIKeysFromJSON(apiKeys); err != nil {
		return nil, fmt.Errorf("解析 API Keys 失败: %w", err)
	}
	if lastSuccessAt.Valid {
		t := lastSuccessAt.Time
		c.LastSuccessAt = &t
	}
	if t, err := parseTime(createdAt); err == nil {
		c.CreatedAt = t
	}
	if t, err := parseTime(updatedAt); err == nil {
		c.UpdatedAt = t
	}
	return c, nil
}

func parseTime(s string) (time.Time, error) {
	return time.Parse("2006-01-02 15:04:05", s)
}

// Create 创建渠道
func (s *ChannelStore) Create(c *models.Channel) (int64, error) {
	now := time.Now()
	result, err := s.db.Exec(
		`INSERT INTO channels (name, type, base_url, api_keys, priority, enabled, status, last_error, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		c.Name, c.Type, c.BaseURL, c.APIKeysJSON(), c.Priority, c.Enabled, c.Status, c.LastError, now, now,
	)
	if err != nil {
		return 0, fmt.Errorf("创建渠道失败: %w", err)
	}
	id, err := result.LastInsertId()
	if err != nil {
		return 0, fmt.Errorf("获取渠道 ID 失败: %w", err)
	}
	c.ID = id
	c.CreatedAt = now
	c.UpdatedAt = now
	return id, nil
}

// Get 获取单个渠道（含模型数）
func (s *ChannelStore) Get(id int64) (*models.Channel, error) {
	row := s.db.QueryRow(`SELECT `+channelColumnsWithCount+` FROM channels c WHERE c.id = ?`, id)
	c, err := scanChannelWithCount(row)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("渠道不存在: %d", id)
		}
		return nil, err
	}
	return c, nil
}

// List 列出所有渠道（含模型数）
func (s *ChannelStore) List() ([]*models.Channel, error) {
	rows, err := s.db.Query(`SELECT ` + channelColumnsWithCount + ` FROM channels c ORDER BY c.priority ASC, c.id ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	channels := []*models.Channel{}
	for rows.Next() {
		c, err := scanChannelWithCount(rows)
		if err != nil {
			return nil, err
		}
		channels = append(channels, c)
	}
	return channels, rows.Err()
}

// ListEnabled 列出所有启用渠道
func (s *ChannelStore) ListEnabled() ([]*models.Channel, error) {
	rows, err := s.db.Query(`SELECT ` + channelColumns + ` FROM channels WHERE enabled = 1 ORDER BY priority ASC, id ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	channels := []*models.Channel{}
	for rows.Next() {
		c, err := scanChannel(rows)
		if err != nil {
			return nil, err
		}
		channels = append(channels, c)
	}
	return channels, rows.Err()
}

// Update 更新渠道
func (s *ChannelStore) Update(c *models.Channel) error {
	now := time.Now()
	_, err := s.db.Exec(
		`UPDATE channels SET name = ?, type = ?, base_url = ?, api_keys = ?, priority = ?, enabled = ?, status = ?, last_error = ?, last_success_at = ?, updated_at = ? WHERE id = ?`,
		c.Name, c.Type, c.BaseURL, c.APIKeysJSON(), c.Priority, c.Enabled, c.Status, c.LastError, c.LastSuccessAt, now, c.ID,
	)
	if err != nil {
		return fmt.Errorf("更新渠道失败: %w", err)
	}
	c.UpdatedAt = now
	return nil
}

// Delete 删除渠道及其模型映射
func (s *ChannelStore) Delete(id int64) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if _, err := tx.Exec(`DELETE FROM channel_models WHERE channel_id = ?`, id); err != nil {
		return err
	}
	if _, err := tx.Exec(`DELETE FROM channels WHERE id = ?`, id); err != nil {
		return err
	}
	return tx.Commit()
}

// SetEnabled 启停渠道
func (s *ChannelStore) SetEnabled(id int64, enabled bool) error {
	_, err := s.db.Exec(`UPDATE channels SET enabled = ?, updated_at = ? WHERE id = ?`, enabled, time.Now(), id)
	return err
}

// UpdateStatus 更新渠道状态与错误信息
func (s *ChannelStore) UpdateStatus(id int64, status models.ChannelStatus, lastError string, successTime *time.Time) error {
	sqlStr := `UPDATE channels SET status = ?, last_error = ?, updated_at = ?`
	args := []any{status, lastError, time.Now()}
	if successTime != nil {
		sqlStr += `, last_success_at = ?`
		args = append(args, *successTime)
	}
	sqlStr += ` WHERE id = ?`
	args = append(args, id)
	_, err := s.db.Exec(sqlStr, args...)
	return err
}

// CountModels 统计渠道模型数
func (s *ChannelStore) CountModels(channelID int64) (int, error) {
	var n int
	err := s.db.QueryRow(`SELECT COUNT(*) FROM channel_models WHERE channel_id = ?`, channelID).Scan(&n)
	return n, err
}

// StringList 将渠道列表转为字符串展示（名称:类型）
func StringList(channels []*models.Channel) []string {
	out := make([]string, 0, len(channels))
	for _, c := range channels {
		name := c.Name
		if name == "" {
			name = fmt.Sprintf("渠道#%d", c.ID)
		}
		if strings.TrimSpace(name) == "" {
			name = fmt.Sprintf("渠道#%d", c.ID)
		}
		out = append(out, name)
	}
	return out
}
