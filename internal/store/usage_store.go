package store

import (
	"database/sql"
	"fmt"
	"strings"
	"time"

	"aiproxy/internal/models"
)

// UsageStore 用量记录数据访问
type UsageStore struct {
	db *sql.DB
}

// NewUsageStore 创建用量存储
func NewUsageStore(db *sql.DB) *UsageStore {
	return &UsageStore{db: db}
}

// Insert 插入用量记录
func (s *UsageStore) Insert(r *models.UsageRecord) error {
	_, err := s.db.Exec(
		`INSERT INTO usage_records (created_at, channel_id, channel_name, model, prompt_tokens, completion_tokens, total_tokens, is_success, status_code, duration_ms, error)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		r.CreatedAt, r.ChannelID, r.ChannelName, r.Model, r.PromptTokens, r.CompletionTokens, r.TotalTokens, r.IsSuccess, r.StatusCode, r.DurationMs, r.Error,
	)
	if err != nil {
		return fmt.Errorf("插入用量记录失败: %w", err)
	}
	return nil
}

// DistinctModels 获取用量记录中出现过的所有模型（去重，用于筛选下拉）
func (s *UsageStore) DistinctModels() ([]string, error) {
	rows, err := s.db.Query(`SELECT DISTINCT model FROM usage_records ORDER BY model`)
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

// ListRecent 获取最近的请求记录（limit 条）
func (s *UsageStore) ListRecent(limit int, channelID int64, model string) ([]*models.UsageRecord, error) {
	query := `SELECT id, created_at, channel_id, channel_name, model, prompt_tokens, completion_tokens, total_tokens, is_success, status_code, duration_ms, error
		FROM usage_records WHERE 1=1`
	args := []any{}

	if channelID > 0 {
		query += ` AND channel_id = ?`
		args = append(args, channelID)
	}
	if model != "" {
		query += ` AND model = ?`
		args = append(args, model)
	}
	query += ` ORDER BY id DESC LIMIT ?`
	args = append(args, limit)

	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	records := []*models.UsageRecord{}
	for rows.Next() {
		r, err := scanUsageRecord(rows)
		if err != nil {
			return nil, err
		}
		records = append(records, r)
	}
	return records, rows.Err()
}

func scanUsageRecord(row interface{ Scan(...any) error }) (*models.UsageRecord, error) {
	var r models.UsageRecord
	var isSuccess int
	var createdAt string

	if err := row.Scan(&r.ID, &createdAt, &r.ChannelID, &r.ChannelName, &r.Model, &r.PromptTokens, &r.CompletionTokens, &r.TotalTokens, &isSuccess, &r.StatusCode, &r.DurationMs, &r.Error); err != nil {
		return nil, err
	}
	r.IsSuccess = isSuccess == 1
	r.CreatedAtRaw = createdAt
	r.CreatedAt = parseUsageTime(createdAt)
	return &r, nil
}

// parseUsageTime 解析 modernc.org/sqlite 驱动可能写入的多种时间格式。
// 默认（未设置 _time_format）驱动用 time.Time.String() 的格式存储：
//
//	"2026-08-05 14:19:39.123456789 +0800 CST"
//
// 若 time.Now() 携带单调时钟（monotonic clock，recordUsage 写入时产生的），
// String() 还会追加 " m=+..." 后缀，例如：
//
//	"2026-08-05 14:19:39.123456789 +0800 CST m=+0.123456789"
//
// 该后缀无法用固定 layout 解析，需先在解析前截掉。
// 若设置了 _time_format=sqlite 则格式为：
//
//	"2026-08-05 14:19:39.123456789-07:00"
//
// 另兼容精确到秒的格式。
func parseUsageTime(s string) time.Time {
	// 截掉 time.Time.String() 输出的单调时钟后缀 " m=+..."
	if i := strings.Index(s, " m="); i > 0 {
		s = s[:i]
	}
	for _, layout := range []string{
		"2006-01-02 15:04:05.999999999-07:00",     // _time_format=sqlite
		"2006-01-02 15:04:05.999999999 -0700 MST", // time.Time.String() 默认格式
		"2006-01-02 15:04:05",                     // 精确到秒
	} {
		if t, err := time.Parse(layout, s); err == nil {
			return t
		}
	}
	return time.Time{}
}

// Summary 时间段内汇总统计（支持渠道/模型筛选）
func (s *UsageStore) Summary(start, end time.Time, channelID int64, model string) (*models.SummaryStat, error) {
	query := `
		SELECT COUNT(*),
			COALESCE(SUM(prompt_tokens), 0),
			COALESCE(SUM(completion_tokens), 0),
			COALESCE(SUM(total_tokens), 0),
			COALESCE(SUM(CASE WHEN is_success = 1 THEN 1 ELSE 0 END), 0),
			COALESCE(SUM(CASE WHEN is_success = 0 THEN 1 ELSE 0 END), 0)
		FROM usage_records
		WHERE created_at >= ? AND created_at < ?
	`
	args := []any{start, end}
	if channelID > 0 {
		query += ` AND channel_id = ?`
		args = append(args, channelID)
	}
	if model != "" {
		query += ` AND model = ?`
		args = append(args, model)
	}

	var st models.SummaryStat
	if err := s.db.QueryRow(query, args...).Scan(&st.Count, &st.PromptTokens, &st.CompletionTokens, &st.TotalTokens, &st.SuccessCount, &st.FailCount); err != nil {
		return nil, err
	}
	return &st, nil
}

// ModelStats 按模型分组聚合统计
func (s *UsageStore) ModelStats(start, end time.Time, channelID int64) ([]*models.ModelStat, error) {
	query := `
		SELECT model,
			COUNT(*) AS count,
			COALESCE(SUM(prompt_tokens), 0) AS prompt_tokens,
			COALESCE(SUM(completion_tokens), 0) AS completion_tokens,
			COALESCE(SUM(total_tokens), 0) AS total_tokens,
			COALESCE(SUM(CASE WHEN is_success = 1 THEN 1 ELSE 0 END), 0) AS success_count,
			COALESCE(SUM(CASE WHEN is_success = 0 THEN 1 ELSE 0 END), 0) AS fail_count
		FROM usage_records
		WHERE created_at >= ? AND created_at < ?
	`
	args := []any{start, end}
	if channelID > 0 {
		query += ` AND channel_id = ?`
		args = append(args, channelID)
	}
	query += ` GROUP BY model ORDER BY count DESC, model ASC`

	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	stats := []*models.ModelStat{}
	for rows.Next() {
		var m models.ModelStat
		if err := rows.Scan(&m.Model, &m.Count, &m.PromptTokens, &m.CompletionTokens, &m.TotalTokens, &m.SuccessCount, &m.FailCount); err != nil {
			return nil, err
		}
		stats = append(stats, &m)
	}
	return stats, rows.Err()
}

// ChannelStats 按渠道分组聚合统计
// 渠道名称优先取渠道表当前录入的名称（channels.name），
// 渠道被删除时回退到用量记录中保存的名称，最后兜底"渠道#ID"。
func (s *UsageStore) ChannelStats(start, end time.Time, model string) ([]*models.ChannelStat, error) {
	query := `
		SELECT u.channel_id,
			COALESCE(NULLIF(c.name, ''), NULLIF(u.channel_name, ''), '渠道#' || u.channel_id) AS channel_name,
			COUNT(*) AS count,
			COALESCE(SUM(u.prompt_tokens), 0) AS prompt_tokens,
			COALESCE(SUM(u.completion_tokens), 0) AS completion_tokens,
			COALESCE(SUM(u.total_tokens), 0) AS total_tokens,
			COALESCE(SUM(CASE WHEN u.is_success = 1 THEN 1 ELSE 0 END), 0) AS success_count,
			COALESCE(SUM(CASE WHEN u.is_success = 0 THEN 1 ELSE 0 END), 0) AS fail_count
		FROM usage_records u
		LEFT JOIN channels c ON c.id = u.channel_id
		WHERE u.created_at >= ? AND u.created_at < ?
	`
	args := []any{start, end}
	if model != "" {
		query += ` AND u.model = ?`
		args = append(args, model)
	}
	query += ` GROUP BY u.channel_id ORDER BY count DESC, u.channel_id ASC`

	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	stats := []*models.ChannelStat{}
	for rows.Next() {
		var c models.ChannelStat
		if err := rows.Scan(&c.ChannelID, &c.ChannelName, &c.Count, &c.PromptTokens, &c.CompletionTokens, &c.TotalTokens, &c.SuccessCount, &c.FailCount); err != nil {
			return nil, err
		}
		stats = append(stats, &c)
	}
	return stats, rows.Err()
}

// DailyStats 按日聚合统计
func (s *UsageStore) DailyStats(start, end time.Time, channelID int64, model string) ([]*models.DailyStat, error) {
	query := `
		SELECT substr(created_at, 1, 10) AS day,
			COUNT(*) AS count,
			COALESCE(SUM(prompt_tokens), 0) AS prompt_tokens,
			COALESCE(SUM(completion_tokens), 0) AS completion_tokens,
			COALESCE(SUM(total_tokens), 0) AS total_tokens,
			COALESCE(SUM(CASE WHEN is_success = 1 THEN 1 ELSE 0 END), 0) AS success_count,
			COALESCE(SUM(CASE WHEN is_success = 0 THEN 1 ELSE 0 END), 0) AS fail_count
		FROM usage_records
		WHERE created_at >= ? AND created_at < ?
	`
	args := []any{start, end}

	if channelID > 0 {
		query += ` AND channel_id = ?`
		args = append(args, channelID)
	}
	if model != "" {
		query += ` AND model = ?`
		args = append(args, model)
	}
	query += ` GROUP BY day ORDER BY day DESC`

	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	stats := []*models.DailyStat{}
	for rows.Next() {
		var d models.DailyStat
		if err := rows.Scan(&d.Date, &d.Count, &d.PromptTokens, &d.CompletionTokens, &d.TotalTokens, &d.SuccessCount, &d.FailCount); err != nil {
			return nil, err
		}
		stats = append(stats, &d)
	}
	return stats, rows.Err()
}

// ModelTokenUsage 按模型聚合全量历史 token 用量
func (s *UsageStore) ModelTokenUsage() ([]*models.TokenUsage, error) {
	rows, err := s.db.Query(`
		SELECT model,
			COALESCE(SUM(prompt_tokens), 0) AS prompt_tokens,
			COALESCE(SUM(completion_tokens), 0) AS completion_tokens,
			COALESCE(SUM(total_tokens), 0) AS total_tokens
		FROM usage_records
		GROUP BY model
		ORDER BY total_tokens DESC, model ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	stats := []*models.TokenUsage{}
	for rows.Next() {
		var t models.TokenUsage
		if err := rows.Scan(&t.Model, &t.PromptTokens, &t.CompletionTokens, &t.TotalTokens); err != nil {
			return nil, err
		}
		stats = append(stats, &t)
	}
	return stats, rows.Err()
}

// TodaySummary 今日汇总（调用次数、输入/输出 token）
func (s *UsageStore) TodaySummary() (count int64, promptTokens, completionTokens int64, err error) {
	now := time.Now()
	start := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	err = s.db.QueryRow(`
		SELECT COUNT(*), COALESCE(SUM(prompt_tokens), 0), COALESCE(SUM(completion_tokens), 0)
		FROM usage_records WHERE created_at >= ?`, start).Scan(&count, &promptTokens, &completionTokens)
	if err != nil {
		return 0, 0, 0, fmt.Errorf("查询今日汇总失败: %w", err)
	}
	return count, promptTokens, completionTokens, nil
}

// DeleteBefore 删除指定时间之前的所有用量记录（利用 idx_usage_created_at 索引），返回删除条数
func (s *UsageStore) DeleteBefore(cutoff time.Time) (int64, error) {
	res, err := s.db.Exec(`DELETE FROM usage_records WHERE created_at < ?`, cutoff)
	if err != nil {
		return 0, fmt.Errorf("清理过期日志失败: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return 0, err
	}
	return n, nil
}

// Clear 清空全部用量记录，返回删除条数
func (s *UsageStore) Clear() (int64, error) {
	res, err := s.db.Exec(`DELETE FROM usage_records`)
	if err != nil {
		return 0, fmt.Errorf("清空请求日志失败: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return 0, err
	}
	return n, nil
}
