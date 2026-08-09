package store

import (
	"database/sql"
	"fmt"

	"aiproxy/internal/models"
)

// ChannelModelStore 渠道-模型映射数据访问
type ChannelModelStore struct {
	db *sql.DB
}

// NewChannelModelStore 创建渠道模型存储
func NewChannelModelStore(db *sql.DB) *ChannelModelStore {
	return &ChannelModelStore{db: db}
}

// Replace 覆盖写入渠道的自动同步模型映射。
// 只删除并重建 source='sync' 的记录，保留 custom（手动添加）和 excluded（用户排除）。
// excluded 模型会被从 modelsList 中过滤掉，不再自动加回。
func (s *ChannelModelStore) Replace(channelID int64, modelsList []string) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	// 读取用户排除的模型集合
	excludedSet := map[string]struct{}{}
	rows, err := tx.Query(`SELECT model FROM channel_models WHERE channel_id = ? AND source = 'excluded'`, channelID)
	if err != nil {
		return fmt.Errorf("读取排除模型失败: %w", err)
	}
	for rows.Next() {
		var m string
		if err := rows.Scan(&m); err != nil {
			rows.Close()
			return err
		}
		excludedSet[m] = struct{}{}
	}
	rows.Close()

	// 过滤掉 excluded 的模型
	filtered := make([]string, 0, len(modelsList))
	for _, m := range modelsList {
		if _, excluded := excludedSet[m]; !excluded {
			filtered = append(filtered, m)
		}
	}

	// 只删除 sync 记录，保留 custom 和 excluded
	if _, err := tx.Exec(`DELETE FROM channel_models WHERE channel_id = ? AND source = 'sync'`, channelID); err != nil {
		return fmt.Errorf("清空同步模型失败: %w", err)
	}
	stmt, err := tx.Prepare(`INSERT OR IGNORE INTO channel_models (channel_id, model, created_at, source) VALUES (?, ?, datetime('now'), 'sync')`)
	if err != nil {
		return err
	}
	defer stmt.Close()

	for _, m := range filtered {
		if _, err := stmt.Exec(channelID, m); err != nil {
			return fmt.Errorf("写入渠道模型失败: %w", err)
		}
	}
	return tx.Commit()
}

// ListByChannel 获取渠道的模型列表（不含 excluded）
func (s *ChannelModelStore) ListByChannel(channelID int64) ([]string, error) {
	rows, err := s.db.Query(`SELECT model FROM channel_models WHERE channel_id = ? AND source != 'excluded' ORDER BY model`, channelID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	modelsList := []string{}
	for rows.Next() {
		var m string
		if err := rows.Scan(&m); err != nil {
			return nil, err
		}
		modelsList = append(modelsList, m)
	}
	return modelsList, rows.Err()
}

// ListChannelsByModel 获取支持指定模型的所有启用渠道（按优先级升序），排除 excluded 绑定
func (s *ChannelModelStore) ListChannelsByModel(model string) ([]*models.Channel, error) {
	rows, err := s.db.Query(`
		SELECT c.id, c.name, c.type, c.base_url, c.api_keys, c.priority, c.enabled, c.status, c.last_error, c.last_success_at, c.created_at, c.updated_at
		FROM channel_models cm
		JOIN channels c ON c.id = cm.channel_id
		WHERE cm.model = ? AND c.enabled = 1 AND cm.source != 'excluded'
		ORDER BY c.priority ASC, c.id ASC`, model)
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

// ListAllModels 聚合所有启用渠道的模型（去重），排除 excluded 绑定
func (s *ChannelModelStore) ListAllModels() ([]string, error) {
	rows, err := s.db.Query(`
		SELECT DISTINCT cm.model
		FROM channel_models cm
		JOIN channels c ON c.id = cm.channel_id
		WHERE c.enabled = 1 AND cm.source != 'excluded'
		ORDER BY cm.model`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	modelsList := []string{}
	for rows.Next() {
		var m string
		if err := rows.Scan(&m); err != nil {
			return nil, err
		}
		modelsList = append(modelsList, m)
	}
	return modelsList, rows.Err()
}

// CountChannelsForModel 统计支持某模型的启用渠道数（排除 excluded 绑定）
func (s *ChannelModelStore) CountChannelsForModel(model string) (int, error) {
	var n int
	err := s.db.QueryRow(`
		SELECT COUNT(*)
		FROM channel_models cm
		JOIN channels c ON c.id = cm.channel_id
		WHERE cm.model = ? AND c.enabled = 1 AND cm.source != 'excluded'`, model).Scan(&n)
	return n, err
}

// GetModelBindings 查询模型的所有渠道绑定（含来源标记与渠道状态）。
// 返回所有渠道的绑定关系，包括 excluded（用于前端展示"已排除"状态）。
func (s *ChannelModelStore) GetModelBindings(model string) ([]*models.ModelBinding, error) {
	rows, err := s.db.Query(`
		SELECT cm.channel_id, COALESCE(c.name, ''), cm.source, COALESCE(c.enabled, 0)
		FROM channel_models cm
		LEFT JOIN channels c ON c.id = cm.channel_id
		WHERE cm.model = ?
		ORDER BY cm.source ASC, cm.channel_id ASC`, model)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	bindings := []*models.ModelBinding{}
	for rows.Next() {
		var b models.ModelBinding
		if err := rows.Scan(&b.ChannelID, &b.ChannelName, &b.Source, &b.ChannelEnabled); err != nil {
			return nil, err
		}
		bindings = append(bindings, &b)
	}
	return bindings, rows.Err()
}

// SetBindings 设置模型的渠道绑定关系（全量覆盖）。
// 根据当前绑定与目标渠道列表计算 diff：
//   - 新增绑定（目标有、当前无）-> INSERT source='custom'
//   - 移除绑定（当前有、目标无）：
//   - 若当前 source='sync' -> UPDATE source='excluded'（下次同步跳过）
//   - 若当前 source='custom' -> DELETE（直接删除手动绑定）
//   - 若当前 source='excluded' -> 保持 excluded（仍不绑定）
//   - 恢复绑定（当前 excluded、目标有）-> DELETE（让下次同步重新加回）
func (s *ChannelModelStore) SetBindings(model string, channelIDs []int64) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	// 读取当前绑定
	current := map[int64]string{} // channelID -> source
	rows, err := tx.Query(`SELECT channel_id, source FROM channel_models WHERE model = ?`, model)
	if err != nil {
		return err
	}
	for rows.Next() {
		var chID int64
		var src string
		if err := rows.Scan(&chID, &src); err != nil {
			rows.Close()
			return err
		}
		current[chID] = src
	}
	rows.Close()

	// 目标渠道集合
	target := map[int64]bool{}
	for _, id := range channelIDs {
		target[id] = true
	}

	// 处理需要移除的绑定
	for chID, src := range current {
		if target[chID] {
			continue // 保留
		}
		// 需要移除
		if src == string(models.ModelSourceSync) {
			// sync -> excluded（下次同步跳过）
			if _, err := tx.Exec(`UPDATE channel_models SET source = 'excluded' WHERE model = ? AND channel_id = ?`, model, chID); err != nil {
				return fmt.Errorf("排除绑定失败: %w", err)
			}
		} else if src == string(models.ModelSourceCustom) {
			// custom -> 直接删除
			if _, err := tx.Exec(`DELETE FROM channel_models WHERE model = ? AND channel_id = ?`, model, chID); err != nil {
				return fmt.Errorf("删除绑定失败: %w", err)
			}
		}
		// excluded 保持不变
	}

	// 处理需要新增/恢复的绑定
	for chID := range target {
		src, exists := current[chID]
		if !exists {
			// 新增 custom 绑定
			if _, err := tx.Exec(`INSERT OR IGNORE INTO channel_models (channel_id, model, created_at, source) VALUES (?, ?, datetime('now'), 'custom')`, chID, model); err != nil {
				return fmt.Errorf("添加绑定失败: %w", err)
			}
		} else if src == string(models.ModelSourceExcluded) {
			// 恢复：删除 excluded 记录，让下次同步重新加回
			if _, err := tx.Exec(`DELETE FROM channel_models WHERE model = ? AND channel_id = ?`, model, chID); err != nil {
				return fmt.Errorf("恢复绑定失败: %w", err)
			}
		}
		// sync/custom 已存在且保留，无需操作
	}

	return tx.Commit()
}

// DeleteAllBindings 删除模型的所有绑定（用于删除自定义模型时级联清理）。
func (s *ChannelModelStore) DeleteAllBindings(model string) error {
	_, err := s.db.Exec(`DELETE FROM channel_models WHERE model = ?`, model)
	return err
}

// ListAllModelsRaw 聚合所有模型（含所有渠道，不论启用状态），排除 excluded 绑定。
// 供模型管理页展示全部已配置模型（含已停用渠道的）。
func (s *ChannelModelStore) ListAllModelsRaw() ([]string, error) {
	rows, err := s.db.Query(`
		SELECT DISTINCT cm.model
		FROM channel_models cm
		WHERE cm.source != 'excluded'
		ORDER BY cm.model`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	modelsList := []string{}
	for rows.Next() {
		var m string
		if err := rows.Scan(&m); err != nil {
			return nil, err
		}
		modelsList = append(modelsList, m)
	}
	return modelsList, rows.Err()
}
