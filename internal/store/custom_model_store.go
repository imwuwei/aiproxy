package store

import (
	"database/sql"
	"fmt"
	"time"

	"aiproxy/internal/models"
)

// CustomModelStore 自定义模型元数据数据访问。
// 仅存储用户主动创建的模型信息（名称/描述），
// 渠道绑定关系存储在 channel_models 表中（source='custom'）。
type CustomModelStore struct {
	db *sql.DB
}

// NewCustomModelStore 创建自定义模型存储
func NewCustomModelStore(db *sql.DB) *CustomModelStore {
	return &CustomModelStore{db: db}
}

// Create 创建自定义模型元数据
func (s *CustomModelStore) Create(name, description string) (int64, error) {
	now := time.Now()
	result, err := s.db.Exec(
		`INSERT INTO custom_models (name, description, created_at, updated_at) VALUES (?, ?, ?, ?)`,
		name, description, now.Format(timeFormat), now.Format(timeFormat),
	)
	if err != nil {
		return 0, fmt.Errorf("创建自定义模型失败: %w", err)
	}
	id, err := result.LastInsertId()
	if err != nil {
		return 0, fmt.Errorf("获取自定义模型 ID 失败: %w", err)
	}
	return id, nil
}

// Update 更新自定义模型描述
func (s *CustomModelStore) Update(name, description string) error {
	now := time.Now()
	result, err := s.db.Exec(
		`UPDATE custom_models SET description = ?, updated_at = ? WHERE name = ?`,
		description, now.Format(timeFormat), name,
	)
	if err != nil {
		return fmt.Errorf("更新自定义模型失败: %w", err)
	}
	n, _ := result.RowsAffected()
	if n == 0 {
		return fmt.Errorf("自定义模型不存在: %s", name)
	}
	return nil
}

// Delete 删除自定义模型元数据。
// 注意：调用方应同时调用 ChannelModelStore.DeleteAllBindings 清理渠道绑定。
func (s *CustomModelStore) Delete(name string) error {
	_, err := s.db.Exec(`DELETE FROM custom_models WHERE name = ?`, name)
	if err != nil {
		return fmt.Errorf("删除自定义模型失败: %w", err)
	}
	return nil
}

// GetByName 按名称获取自定义模型；不存在时返回 (nil, nil)。
func (s *CustomModelStore) GetByName(name string) (*models.CustomModel, error) {
	row := s.db.QueryRow(`SELECT id, name, description, created_at, updated_at FROM custom_models WHERE name = ?`, name)
	m, err := scanCustomModel(row)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	return m, nil
}

// List 列出所有自定义模型
func (s *CustomModelStore) List() ([]*models.CustomModel, error) {
	rows, err := s.db.Query(`SELECT id, name, description, created_at, updated_at FROM custom_models ORDER BY name ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	list := []*models.CustomModel{}
	for rows.Next() {
		m, err := scanCustomModel(rows)
		if err != nil {
			return nil, err
		}
		list = append(list, m)
	}
	return list, rows.Err()
}

// ListNames 列出所有自定义模型名称
func (s *CustomModelStore) ListNames() ([]string, error) {
	rows, err := s.db.Query(`SELECT name FROM custom_models ORDER BY name ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	names := []string{}
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, err
		}
		names = append(names, name)
	}
	return names, rows.Err()
}

// scanCustomModel 扫描一行自定义模型记录。
func scanCustomModel(row interface{ Scan(...any) error }) (*models.CustomModel, error) {
	var m models.CustomModel
	var createdAt, updatedAt string
	if err := row.Scan(&m.ID, &m.Name, &m.Description, &createdAt, &updatedAt); err != nil {
		return nil, err
	}
	if t, err := parseAliasTime(createdAt); err == nil {
		m.CreatedAt = t.Unix()
	}
	if t, err := parseAliasTime(updatedAt); err == nil {
		m.UpdatedAt = t.Unix()
	}
	return &m, nil
}
