package store

import (
	"database/sql"
	"fmt"
	"time"

	"aiproxy/internal/models"
)

// ModelAliasStore 模型别名数据访问
type ModelAliasStore struct {
	db *sql.DB
}

// NewModelAliasStore 创建模型别名存储
func NewModelAliasStore(db *sql.DB) *ModelAliasStore {
	return &ModelAliasStore{db: db}
}

const aliasColumns = `id, name, targets, enabled, created_at, updated_at`

func parseAliasTime(s string) (time.Time, error) {
	if t, err := time.Parse(time.RFC3339, s); err == nil {
		return t, nil
	}
	return time.Parse(timeFormat, s)
}

// scanAlias 扫描一行别名记录。
func scanAlias(row interface{ Scan(...any) error }) (*models.ModelAlias, error) {
	var a models.ModelAlias
	var enabled int
	var createdAt, updatedAt string
	if err := row.Scan(&a.ID, &a.Name, &a.Targets, &enabled, &createdAt, &updatedAt); err != nil {
		return nil, err
	}
	a.Enabled = enabled == 1
	if t, err := parseAliasTime(createdAt); err == nil {
		a.CreatedAt = t.Unix()
	}
	if t, err := parseAliasTime(updatedAt); err == nil {
		a.UpdatedAt = t.Unix()
	}
	return &a, nil
}

// timeFormat 时间存储格式（与 parseTime 解析格式保持一致）。
const timeFormat = "2006-01-02 15:04:05"

// Create 创建别名
func (s *ModelAliasStore) Create(a *models.ModelAlias) (int64, error) {
	now := time.Now()
	result, err := s.db.Exec(
		`INSERT INTO model_aliases (name, targets, enabled, created_at, updated_at) VALUES (?, ?, ?, ?, ?)`,
		a.Name, a.Targets, a.Enabled, now.Format(timeFormat), now.Format(timeFormat),
	)
	if err != nil {
		return 0, fmt.Errorf("创建模型别名失败: %w", err)
	}
	id, err := result.LastInsertId()
	if err != nil {
		return 0, fmt.Errorf("获取模型别名 ID 失败: %w", err)
	}
	a.ID = id
	a.CreatedAt = now.Unix()
	a.UpdatedAt = now.Unix()
	return id, nil
}

// Get 获取单个别名
func (s *ModelAliasStore) Get(id int64) (*models.ModelAlias, error) {
	row := s.db.QueryRow(`SELECT `+aliasColumns+` FROM model_aliases WHERE id = ?`, id)
	a, err := scanAlias(row)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("模型别名不存在: %d", id)
		}
		return nil, err
	}
	return a, nil
}

// GetByName 按名称获取别名；不存在时返回 (nil, nil)。
func (s *ModelAliasStore) GetByName(name string) (*models.ModelAlias, error) {
	row := s.db.QueryRow(`SELECT `+aliasColumns+` FROM model_aliases WHERE name = ?`, name)
	a, err := scanAlias(row)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	return a, nil
}

// List 列出所有别名
func (s *ModelAliasStore) List() ([]*models.ModelAlias, error) {
	rows, err := s.db.Query(`SELECT ` + aliasColumns + ` FROM model_aliases ORDER BY id ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	aliases := []*models.ModelAlias{}
	for rows.Next() {
		a, err := scanAlias(rows)
		if err != nil {
			return nil, err
		}
		aliases = append(aliases, a)
	}
	return aliases, rows.Err()
}

// ListEnabledNames 列出所有启用别名名称（用于模型列表融合展示）
func (s *ModelAliasStore) ListEnabledNames() []string {
	rows, err := s.db.Query(`SELECT name FROM model_aliases WHERE enabled = 1 ORDER BY id ASC`)
	if err != nil {
		return nil
	}
	defer rows.Close()

	names := []string{}
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			continue
		}
		names = append(names, name)
	}
	return names
}

// ListEnabled 列出所有启用别名（路由使用）
func (s *ModelAliasStore) ListEnabled() ([]*models.ModelAlias, error) {
	rows, err := s.db.Query(`SELECT ` + aliasColumns + ` FROM model_aliases WHERE enabled = 1 ORDER BY id ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	aliases := []*models.ModelAlias{}
	for rows.Next() {
		a, err := scanAlias(rows)
		if err != nil {
			return nil, err
		}
		aliases = append(aliases, a)
	}
	return aliases, rows.Err()
}

// Update 更新别名（名称与目标模型）
func (s *ModelAliasStore) Update(a *models.ModelAlias) error {
	now := time.Now()
	_, err := s.db.Exec(
		`UPDATE model_aliases SET name = ?, targets = ?, enabled = ?, updated_at = ? WHERE id = ?`,
		a.Name, a.Targets, a.Enabled, now.Format(timeFormat), a.ID,
	)
	if err != nil {
		return fmt.Errorf("更新模型别名失败: %w", err)
	}
	a.UpdatedAt = now.Unix()
	return nil
}

// Delete 删除别名
func (s *ModelAliasStore) Delete(id int64) error {
	_, err := s.db.Exec(`DELETE FROM model_aliases WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("删除模型别名失败: %w", err)
	}
	return nil
}

// SetEnabled 启停别名
func (s *ModelAliasStore) SetEnabled(id int64, enabled bool) error {
	_, err := s.db.Exec(`UPDATE model_aliases SET enabled = ?, updated_at = ? WHERE id = ?`, enabled, time.Now().Format(timeFormat), id)
	return err
}
