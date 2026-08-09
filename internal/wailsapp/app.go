//go:build !cli

// Package wailsapp 提供基于 Wails v2 的桌面 GUI 后端绑定层。
// 聚合现有 store/proxy/modelsync/config 组件，向前端暴露全部业务 API。
package wailsapp

import (
	"context"
	"fmt"
	"log"
	"strconv"
	"strings"
	"time"

	"aiproxy/internal/config"
	"aiproxy/internal/models"
	"aiproxy/internal/modelsync"
	"aiproxy/internal/proxy"
	"aiproxy/internal/singleinst"
	"aiproxy/internal/store"

	"github.com/wailsapp/wails/v2/pkg/runtime"
)

// App 是 Wails 绑定的根对象，所有导出方法均被前端通过 window.go.* 调用。
type App struct {
	ctx          context.Context
	config       *config.Config
	settings     *store.SettingsStore
	channels     *store.ChannelStore
	models       *store.ChannelModelStore
	customModels *store.CustomModelStore
	usage        *store.UsageStore
	aliases      *store.ModelAliasStore
	proxySrv     *proxy.Server
	modelSync    *modelsync.Service
	instance     *singleinst.Instance
}

// NewApp 创建 Wails 绑定应用。
func NewApp(cfg *config.Config, settingsStore *store.SettingsStore, channelStore *store.ChannelStore,
	modelStore *store.ChannelModelStore, customModelStore *store.CustomModelStore, usageStore *store.UsageStore, aliasStore *store.ModelAliasStore,
	proxySrv *proxy.Server, sync *modelsync.Service) *App {
	return &App{
		config:       cfg,
		settings:     settingsStore,
		channels:     channelStore,
		models:       modelStore,
		customModels: customModelStore,
		usage:        usageStore,
		aliases:      aliasStore,
		proxySrv:     proxySrv,
		modelSync:    sync,
	}
}

// SetSingleInstance 注入单实例锁实例，用于销毁时释放与激活回调。
func (a *App) SetSingleInstance(inst *singleinst.Instance) {
	a.instance = inst
}

// init 供 wails 在应用就绪时调用（通过 Startup 绑定）。
func (a *App) init(ctx context.Context) {
	a.ctx = ctx

	// 单实例激活回调：被新实例请求激活时恢复显示主窗口
	if a.instance != nil {
		a.instance.OnActivate(func() {
			runtime.EventsEmit(a.ctx, "app:activate")
			runtime.WindowShow(a.ctx)
		})
	}

	// 自动启动代理服务
	if err := a.proxySrv.Start(); err != nil {
		log.Printf("[wailsapp] 启动代理失败: %v", err)
	} else {
		a.modelSync.Start()
	}
}

// shutdown 供 wails 在退出时调用（通过 Shutdown 绑定）。
func (a *App) shutdown(_ context.Context) {
	a.modelSync.Stop()
	_ = a.proxySrv.Stop()
	if a.instance != nil {
		a.instance.Release()
	}
}

// Shutdown 暴露给前端的手动退出入口（托盘"退出"等）。
func (a *App) Shutdown() {
	runtime.Quit(a.ctx)
}

// ---------- 仪表盘 ----------

// Dashboard 返回仪表盘数据。
type Dashboard struct {
	Running         bool   `json:"running"`
	BaseURL         string `json:"base_url"`
	ListenPort      int    `json:"listen_port"`
	TodayCount      int64  `json:"today_count"`
	TodayPrompt     int64  `json:"today_prompt"`
	TodayCompletion int64  `json:"today_completion"`
	TodayTotal      int64  `json:"today_total"`
	TotalModels     int    `json:"total_models"`
	EnabledCh       int    `json:"enabled_channels"`
	OnlineCh        int    `json:"online_channels"`
	CoolingCh       int    `json:"cooling_channels"`
	OfflineCh       int    `json:"offline_channels"`
	TotalCh         int    `json:"total_channels"`
}

// GetDashboard 获取仪表盘数据。
func (a *App) GetDashboard() Dashboard {
	d := Dashboard{
		Running:    a.proxySrv.Running(),
		BaseURL:    a.config.BaseURL(),
		ListenPort: a.config.ListenPort,
	}
	if count, prompt, completion, err := a.usage.TodaySummary(); err == nil {
		d.TodayCount = count
		d.TodayPrompt = prompt
		d.TodayCompletion = completion
		d.TodayTotal = prompt + completion
	}
	if channels, err := a.channels.List(); err == nil {
		d.TotalCh = len(channels)
		for _, c := range channels {
			if c.Enabled {
				d.EnabledCh++
			}
			switch c.Status {
			case models.ChannelStatusOnline:
				d.OnlineCh++
			case models.ChannelStatusCooling:
				d.CoolingCh++
			case models.ChannelStatusOffline:
				d.OfflineCh++
			}
		}
	}
	if modelsList, err := a.models.ListAllModels(); err == nil {
		d.TotalModels = len(modelsList)
	}
	return d
}

// ToggleProxy 启动/停止代理服务，返回最新运行状态。
func (a *App) ToggleProxy() (bool, error) {
	if a.proxySrv.Running() {
		if err := a.proxySrv.Stop(); err != nil {
			return true, err
		}
	} else {
		if err := a.proxySrv.Start(); err != nil {
			return false, err
		}
		a.modelSync.Start()
	}
	a.emitStateChanged()
	return a.proxySrv.Running(), nil
}

// CopyText 复制文本到系统剪贴板。
func (a *App) CopyText(text string) {
	if a.ctx != nil {
		if err := runtime.ClipboardSetText(a.ctx, text); err != nil {
			log.Printf("[wailsapp] 复制到剪贴板失败: %v", err)
		}
	}
}

// ---------- 渠道管理 ----------

// CreateChannel 创建渠道（保存后自动触发模型同步）。
func (a *App) CreateChannel(ch *models.Channel) (int64, error) {
	if ch.Type == "" {
		ch.Type = models.ChannelTypeOpenAICompatible
	}
	id, err := a.channels.Create(ch)
	if err != nil {
		return 0, err
	}
	ch.ID = id
	go func() {
		_ = a.modelSync.SyncChannel(context.Background(), ch)
		a.emitStateChanged()
	}()
	return id, nil
}

// UpdateChannel 更新渠道（保存后自动触发模型同步）。
func (a *App) UpdateChannel(ch *models.Channel) error {
	if err := a.channels.Update(ch); err != nil {
		return err
	}
	go func() {
		_ = a.modelSync.SyncChannel(context.Background(), ch)
		a.emitStateChanged()
	}()
	return nil
}

// DeleteChannel 删除渠道。
func (a *App) DeleteChannel(id int64) error {
	err := a.channels.Delete(id)
	if err == nil {
		a.emitStateChanged()
	}
	return err
}

// ToggleChannel 启用/停用渠道。
func (a *App) ToggleChannel(id int64, enabled bool) error {
	if err := a.channels.SetEnabled(id, enabled); err != nil {
		return err
	}
	if enabled {
		if ch, err := a.channels.Get(id); err == nil {
			go func() {
				_ = a.modelSync.SyncChannel(context.Background(), ch)
				a.emitStateChanged()
			}()
		}
	}
	return nil
}

// TestChannel 测试渠道连接，返回可用模型数。
func (a *App) TestChannel(id int64) (int, error) {
	ch, err := a.channels.Get(id)
	if err != nil {
		return 0, err
	}
	n, err := a.modelSync.TestChannel(ch)
	if err == nil {
		log.Printf("[wailsapp] 渠道 %s 测试成功，共 %d 个模型", ch.Name, n)
	}
	return n, err
}

// RefreshChannelModels 手动刷新渠道模型列表。
func (a *App) RefreshChannelModels(id int64) error {
	ch, err := a.channels.Get(id)
	if err != nil {
		return err
	}
	if err := a.modelSync.SyncChannel(context.Background(), ch); err != nil {
		return err
	}
	a.emitStateChanged()
	return nil
}

// ListChannels 列出所有渠道。
func (a *App) ListChannels() ([]*models.Channel, error) {
	return a.channels.List()
}

// ChannelTypeName 返回渠道类型中文名。
func (a *App) ChannelTypeName(t models.ChannelType) string {
	switch t {
	case models.ChannelTypeOpenAICompatible:
		return "OpenAI 兼容"
	case models.ChannelTypeAnthropic:
		return "Anthropic"
	case models.ChannelTypeGemini:
		return "Gemini"
	case models.ChannelTypeCustom:
		return "自定义"
	default:
		return string(t)
	}
}

// ChannelTypes 渠道类型选项（前端下拉用）。
func (a *App) ChannelTypes() []string {
	return []string{
		string(models.ChannelTypeOpenAICompatible),
		string(models.ChannelTypeAnthropic),
		string(models.ChannelTypeGemini),
		string(models.ChannelTypeCustom),
	}
}

// ---------- 模型管理 ----------

// ModelRow 模型列表行。
type ModelRow struct {
	Model            string `json:"model"`
	ChannelCount     int    `json:"channel_count"`
	PromptTokens     int64  `json:"prompt_tokens"`
	CompletionTokens int64  `json:"completion_tokens"`
	TotalTokens      int64  `json:"total_tokens"`
	IsCustom         bool   `json:"is_custom"` // 是否为用户自定义模型
	Description      string `json:"description"`
}

// ListModels 返回聚合模型列表及 token 用量。
// 包含同步模型和自定义模型，标注来源与描述。
func (a *App) ListModels() ([]ModelRow, error) {
	modelsList, err := a.models.ListAllModels()
	if err != nil {
		return nil, err
	}
	// 加载自定义模型名称集合（用于标注 is_custom）
	customNames := map[string]*models.CustomModel{}
	if customModels, err := a.customModels.List(); err == nil {
		for _, cm := range customModels {
			customNames[cm.Name] = cm
		}
	}
	tokenMap := map[string]*models.TokenUsage{}
	if stats, err := a.usage.ModelTokenUsage(); err == nil {
		for _, st := range stats {
			tokenMap[st.Model] = st
		}
	}
	rows := make([]ModelRow, 0, len(modelsList))
	for _, m := range modelsList {
		row := ModelRow{Model: m}
		if n, err := a.models.CountChannelsForModel(m); err == nil {
			row.ChannelCount = n
		}
		if tu, ok := tokenMap[m]; ok {
			row.PromptTokens = tu.PromptTokens
			row.CompletionTokens = tu.CompletionTokens
			row.TotalTokens = tu.TotalTokens
		}
		if cm, ok := customNames[m]; ok {
			row.IsCustom = true
			row.Description = cm.Description
		}
		rows = append(rows, row)
	}
	return rows, nil
}

// SyncAllModels 全量同步所有启用渠道的模型。
func (a *App) SyncAllModels() {
	go func() {
		a.modelSync.SyncAll()
		a.emitStateChanged()
	}()
}

// ---------- 自定义模型管理 ----------

// CreateCustomModel 创建自定义模型并绑定到指定渠道。
// name 为模型名称，description 为可选描述，channelIDs 为绑定的渠道 ID 列表。
func (a *App) CreateCustomModel(name string, description string, channelIDs []int64) error {
	if strings.TrimSpace(name) == "" {
		return fmt.Errorf("模型名称不能为空")
	}
	// 检查是否已存在（GetByName 在不存在时返回 (nil, nil)）
	if existing, _ := a.customModels.GetByName(name); existing != nil {
		return fmt.Errorf("自定义模型「%s」已存在", name)
	}
	if len(channelIDs) == 0 {
		return fmt.Errorf("请至少选择一个渠道")
	}
	// 验证渠道存在
	for _, id := range channelIDs {
		if _, err := a.channels.Get(id); err != nil {
			return fmt.Errorf("渠道 %d 不存在: %w", id, err)
		}
	}
	if _, err := a.customModels.Create(name, description); err != nil {
		return err
	}
	if err := a.models.SetBindings(name, channelIDs); err != nil {
		// 绑定失败时回滚元数据
		_ = a.customModels.Delete(name)
		return fmt.Errorf("绑定渠道失败: %w", err)
	}
	a.emitStateChanged()
	return nil
}

// UpdateCustomModel 更新自定义模型的描述。
func (a *App) UpdateCustomModel(name string, description string) error {
	if strings.TrimSpace(name) == "" {
		return fmt.Errorf("模型名称不能为空")
	}
	if err := a.customModels.Update(name, description); err != nil {
		return err
	}
	a.emitStateChanged()
	return nil
}

// DeleteCustomModel 删除自定义模型及其所有渠道绑定。
func (a *App) DeleteCustomModel(name string) error {
	if strings.TrimSpace(name) == "" {
		return fmt.Errorf("模型名称不能为空")
	}
	// 检查是否存在
	cm, err := a.customModels.GetByName(name)
	if err != nil {
		return fmt.Errorf("查询自定义模型失败: %w", err)
	}
	if cm == nil {
		return fmt.Errorf("自定义模型「%s」不存在", name)
	}
	// 先删除所有绑定，再删除元数据
	if err := a.models.DeleteAllBindings(name); err != nil {
		return fmt.Errorf("删除渠道绑定失败: %w", err)
	}
	if err := a.customModels.Delete(name); err != nil {
		return fmt.Errorf("删除自定义模型失败: %w", err)
	}
	a.emitStateChanged()
	return nil
}

// ListCustomModels 列出所有自定义模型。
func (a *App) ListCustomModels() ([]*models.CustomModel, error) {
	return a.customModels.List()
}

// ---------- 模型渠道绑定管理 ----------

// GetModelBindings 查询模型的所有渠道绑定（含来源标记与渠道状态）。
func (a *App) GetModelBindings(model string) ([]*models.ModelBinding, error) {
	return a.models.GetModelBindings(model)
}

// SetModelBindings 设置模型的渠道绑定关系（全量覆盖）。
// 适用于所有模型（同步模型和自定义模型）。
func (a *App) SetModelBindings(model string, channelIDs []int64) error {
	if strings.TrimSpace(model) == "" {
		return fmt.Errorf("模型名称不能为空")
	}
	// 验证渠道存在
	for _, id := range channelIDs {
		if _, err := a.channels.Get(id); err != nil {
			return fmt.Errorf("渠道 %d 不存在: %w", id, err)
		}
	}
	if err := a.models.SetBindings(model, channelIDs); err != nil {
		return fmt.Errorf("设置渠道绑定失败: %w", err)
	}
	a.emitStateChanged()
	return nil
}

// ---------- 模型别名管理 ----------

// CreateAlias 创建模型别名。
func (a *App) CreateAlias(alias *models.ModelAlias) (int64, error) {
	if alias.Name == "" {
		return 0, fmt.Errorf("别名名称不能为空")
	}
	if _, err := alias.ParseTargets(); err != nil {
		return 0, err
	}
	return a.aliases.Create(alias)
}

// UpdateAlias 更新模型别名。
func (a *App) UpdateAlias(alias *models.ModelAlias) error {
	if alias.Name == "" {
		return fmt.Errorf("别名名称不能为空")
	}
	if _, err := alias.ParseTargets(); err != nil {
		return err
	}
	return a.aliases.Update(alias)
}

// DeleteAlias 删除模型别名。
func (a *App) DeleteAlias(id int64) error {
	return a.aliases.Delete(id)
}

// ToggleAlias 启用/停用模型别名。
func (a *App) ToggleAlias(id int64, enabled bool) error {
	return a.aliases.SetEnabled(id, enabled)
}

// ListAliases 列出所有模型别名。
func (a *App) ListAliases() ([]*models.ModelAlias, error) {
	return a.aliases.List()
}

// RenderAliasTargets 将别名目标配置字符串转为展示文本。
// 例如 [{"model":"gpt-4o","weight":2},{"model":"claude-3-5-sonnet"}] → gpt-4o(2), claude-3-5-sonnet。
func (a *App) RenderAliasTargets(targets string) string {
	list, err := models.ParseAliasTargets(targets)
	if err != nil {
		return targets
	}
	return list.Render()
}

// ---------- 用量统计 ----------

// StatParams 统计查询参数。
type StatParams struct {
	Range     string `json:"range"`      // today | 7d | 30d | week | month | custom
	StartDate string `json:"start_date"` // custom 时生效：YYYY-MM-DD（含当天）
	EndDate   string `json:"end_date"`   // custom 时生效：YYYY-MM-DD（含当天）
	ChannelID int64  `json:"channel_id"`
	Model     string `json:"model"`
}

// StatsRange 统计时间范围（开始/结束）。
type StatsRange struct {
	Start string `json:"start"`
	End   string `json:"end"`
}

// StatsResult 用量统计结果。
type StatsResult struct {
	Today     *models.SummaryStat   `json:"today"`
	Range     StatsRange            `json:"range"`
	Daily     []*models.DailyStat   `json:"daily"`
	ByModel   []*models.ModelStat   `json:"by_model"`
	ByChannel []*models.ChannelStat `json:"by_channel"`
}

func (a *App) statsStartEnd(r string) (start, end time.Time) {
	now := time.Now()
	todayStart := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	start = todayStart
	end = todayStart.AddDate(0, 0, 1)
	switch r {
	case "today":
	case "week":
		// 本周一 00:00 起
		weekday := int(now.Weekday())
		if weekday == 0 {
			weekday = 7
		}
		start = todayStart.AddDate(0, 0, -(weekday - 1))
	case "month":
		start = time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location())
	case "30d":
		start = todayStart.AddDate(0, 0, -29)
	default: // 7d
		start = todayStart.AddDate(0, 0, -6)
	}
	return start, end
}

// customRange 解析自定义日期范围。
// start_date/end_date 为 YYYY-MM-DD，end_date 含当天（结束时间为次日零点）。
// 非法或为空时回退到今日；end_date 早于 start_date 时强制至少一天。
func (a *App) customRange(p StatParams) (start, end time.Time) {
	now := time.Now()
	start = time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	end = start.AddDate(0, 0, 1)
	if p.StartDate != "" {
		if t, err := time.ParseInLocation("2006-01-02", p.StartDate, now.Location()); err == nil {
			start = t
		}
	}
	if p.EndDate != "" {
		if t, err := time.ParseInLocation("2006-01-02", p.EndDate, now.Location()); err == nil {
			end = t.AddDate(0, 0, 1)
		}
	}
	if end.Before(start.AddDate(0, 0, 1)) {
		end = start.AddDate(0, 0, 1)
	}
	return start, end
}

// GetStats 获取用量统计。
func (a *App) GetStats(p StatParams) StatsResult {
	start, end := a.statsStartEnd("7d")
	if p.Range == "custom" {
		start, end = a.customRange(p)
	} else if p.Range != "" {
		start, end = a.statsStartEnd(p.Range)
	}
	res := StatsResult{}
	if st, err := a.usage.Summary(start, end, p.ChannelID, p.Model); err == nil {
		res.Today = st
	}
	res.Range = StatsRange{
		Start: start.Format("2006-01-02"),
		End:   end.AddDate(0, 0, -1).Format("2006-01-02"),
	}
	if stats, err := a.usage.DailyStats(start, end, p.ChannelID, p.Model); err == nil {
		res.Daily = stats
	}
	if stats, err := a.usage.ModelStats(start, end, p.ChannelID); err == nil {
		res.ByModel = stats
	}
	if stats, err := a.usage.ChannelStats(start, end, p.Model); err == nil {
		res.ByChannel = stats
	}
	return res
}

// StatsOptions 返回统计页筛选项。
type StatsOptions struct {
	Channels []*models.Channel `json:"channels"`
	Models   []string          `json:"models"`
}

// GetStatsOptions 获取统计页渠道/模型下拉选项。
func (a *App) GetStatsOptions() StatsOptions {
	opts := StatsOptions{}
	channels, err := a.channels.List()
	if err == nil {
		opts.Channels = channels
	}
	if models, err := a.usage.DistinctModels(); err == nil {
		opts.Models = models
	}
	return opts
}

// ---------- 请求日志 ----------

// GetLogs 获取最近请求日志（limit 条，支持渠道/模型筛选）。
func (a *App) GetLogs(limit int, channelID int64, model string) ([]*models.UsageRecord, error) {
	if limit <= 0 {
		limit = 200
	}
	return a.usage.ListRecent(limit, channelID, model)
}

// LogOptions 请求日志筛选项。
type LogOptions struct {
	Channels []*models.Channel `json:"channels"`
	Models   []string          `json:"models"`
}

// GetLogOptions 获取日志页渠道/模型下拉选项。
func (a *App) GetLogOptions() LogOptions {
	opts := LogOptions{}
	if channels, err := a.channels.List(); err == nil {
		opts.Channels = channels
	}
	if models, err := a.usage.DistinctModels(); err == nil {
		opts.Models = models
	}
	return opts
}

// ClearLogs 清空全部请求日志。
func (a *App) ClearLogs() (int64, error) {
	return a.usage.Clear()
}

// FormatLogTime 格式化日志时间显示。
func (a *App) FormatLogTime(createdAtRaw string, createdAt time.Time) string {
	if createdAtRaw != "" {
		return createdAtRaw
	}
	if !createdAt.IsZero() {
		return createdAt.Format("2006-01-02 15:04:05")
	}
	return ""
}

// ---------- 设置 ----------

// SettingsData 设置页数据。
type SettingsData struct {
	ListenAddr        string `json:"listen_addr"`
	ListenPort        int    `json:"listen_port"`
	AccessToken       string `json:"access_token"`
	AuthEnabled       bool   `json:"auth_enabled"`
	ModelSyncInterval int    `json:"model_sync_interval_minutes"`
	ProxyTimeout      int    `json:"proxy_timeout_seconds"`
	BreakerThreshold  int    `json:"breaker_threshold"`
	BreakerCooldown   int    `json:"breaker_cooldown_seconds"`
	LogRetentionDays  int    `json:"log_retention_days"`
	Debug             bool   `json:"debug"`
	AutoStart         bool   `json:"autostart"`
	AutoStartEnabled  bool   `json:"autostart_supported"`
	StartMinimized    bool   `json:"start_minimized"`
	TokenDisplay      string `json:"token_display"`
	ProxyRunning      bool   `json:"proxy_running"`
}

// GetSettings 获取当前设置。
func (a *App) GetSettings() SettingsData {
	return SettingsData{
		ListenAddr:        a.config.ListenAddr,
		ListenPort:        a.config.ListenPort,
		AccessToken:       a.config.AccessToken,
		AuthEnabled:       a.config.AuthEnabled,
		ModelSyncInterval: int(a.config.ModelSyncInterval.Minutes()),
		ProxyTimeout:      int(a.config.ProxyTimeout.Seconds()),
		BreakerThreshold:  a.config.BreakerThreshold,
		BreakerCooldown:   int(a.config.BreakerCooldown.Seconds()),
		LogRetentionDays:  a.config.LogRetentionDays,
		Debug:             a.config.Debug,
		AutoStart:         config.IsAutoStartEnabled(),
		AutoStartEnabled:  config.AutoStartSupported,
		StartMinimized:    a.config.StartMinimized,
		TokenDisplay:      a.config.TokenDisplay,
		ProxyRunning:      a.proxySrv.Running(),
	}
}

// SaveTokenDisplay 保存 Token 数值显示方式并立即生效。
func (a *App) SaveTokenDisplay(display string) error {
	if display != "auto" && display != "raw" {
		return fmt.Errorf("Token 显示方式取值无效: %s（可选 auto/raw）", display)
	}
	if err := a.settings.Set(models.SettingsTokenDisplay, display); err != nil {
		return fmt.Errorf("保存 Token 显示方式失败: %v", err)
	}
	a.config.TokenDisplay = display
	a.emitStateChanged()
	return nil
}

// SetStartMinimized 设置启动时默认最小化，立即生效（下次启动应用时生效）。
func (a *App) SetStartMinimized(enabled bool) error {
	if err := a.settings.Set(models.SettingsStartMinimized, strconv.FormatBool(enabled)); err != nil {
		return fmt.Errorf("保存启动最小化设置失败: %v", err)
	}
	cfg, err := a.settings.Load()
	if err != nil {
		return err
	}
	a.config = cfg
	a.emitStateChanged()
	return nil
}

// SaveServiceConfig 保存服务配置（监听地址/端口/访问令牌/鉴权开关）。
// 代理服务运行中禁止修改，须先停止服务；保存后于下次启动服务时生效。
func (a *App) SaveServiceConfig(listenAddr string, listenPort int, accessToken string, authEnabled bool) error {
	if listenPort <= 0 {
		return fmt.Errorf("监听端口必须是大于 0 的整数")
	}
	if authEnabled && strings.TrimSpace(accessToken) == "" {
		return fmt.Errorf("启用代理鉴权时，访问令牌不能为空")
	}
	// 代理服务运行中禁止修改服务配置，防止"保存成功但监听未变化"的误导。
	// 前端已锁定表单项，此处为后端兜底校验。
	if a.proxySrv.Running() {
		return fmt.Errorf("代理服务运行中，请先停止服务后再修改服务配置")
	}

	set := func(key, value string) bool {
		return a.settings.Set(key, value) == nil
	}
	ok := set(models.SettingsListenAddr, listenAddr) &&
		set(models.SettingsListenPort, strconv.Itoa(listenPort)) &&
		set(models.SettingsAccessToken, accessToken) &&
		set(models.SettingsAuthEnabled, strconv.FormatBool(authEnabled))
	if !ok {
		return fmt.Errorf("保存服务配置失败")
	}

	// 更新运行时配置（下次启动服务时由 Start 重新加载生效）
	if cfg, err := a.settings.Load(); err == nil {
		a.config = cfg
	}
	a.emitStateChanged()
	return nil
}

// SaveModelSyncConfig 保存模型刷新间隔并立即生效。
func (a *App) SaveModelSyncConfig(minutes int) error {
	if minutes <= 0 {
		return fmt.Errorf("模型刷新间隔必须是大于 0 的整数")
	}
	if err := a.settings.Set(models.SettingsModelSyncInterval, strconv.Itoa(minutes)); err != nil {
		return fmt.Errorf("保存模型同步设置失败: %v", err)
	}
	cfg, err := a.settings.Load()
	if err != nil {
		return err
	}
	a.config = cfg
	a.modelSync.SetInterval(cfg.ModelSyncInterval)
	a.emitStateChanged()
	return nil
}

// SaveBreakerConfig 保存代理容错参数（请求超时/熔断）并立即热更新。
func (a *App) SaveBreakerConfig(timeoutSeconds, breakerThreshold, breakerCooldown int) error {
	if timeoutSeconds <= 0 {
		return fmt.Errorf("代理请求超时必须是大于 0 的整数")
	}
	if breakerThreshold <= 0 {
		return fmt.Errorf("熔断阈值必须是大于 0 的整数")
	}
	if breakerCooldown <= 0 {
		return fmt.Errorf("熔断冷却必须是大于 0 的整数")
	}

	set := func(key, value string) bool {
		return a.settings.Set(key, value) == nil
	}
	ok := set(models.SettingsProxyTimeoutSeconds, strconv.Itoa(timeoutSeconds)) &&
		set(models.SettingsBreakerThreshold, strconv.Itoa(breakerThreshold)) &&
		set(models.SettingsBreakerCooldownSec, strconv.Itoa(breakerCooldown))
	if !ok {
		return fmt.Errorf("保存代理容错设置失败")
	}

	cfg, err := a.settings.Load()
	if err != nil {
		return err
	}
	a.config = cfg

	// 热更新 client/streamClient/breaker 参数，服务运行中亦安全
	a.proxySrv.ReloadConfig()
	a.emitStateChanged()
	return nil
}

// SaveLogConfig 保存日志与调试设置并立即热更新。
func (a *App) SaveLogConfig(logRetentionDays int, debug bool) error {
	if logRetentionDays <= 0 {
		return fmt.Errorf("日志保留天数必须是大于 0 的整数")
	}
	set := func(key, value string) bool {
		return a.settings.Set(key, value) == nil
	}
	ok := set(models.SettingsLogRetentionDays, strconv.Itoa(logRetentionDays)) &&
		set(models.SettingsDebug, strconv.FormatBool(debug))
	if !ok {
		return fmt.Errorf("保存日志设置失败")
	}

	cfg, err := a.settings.Load()
	if err != nil {
		return err
	}
	a.config = cfg

	// 热更新 debug 标志等运行时行为
	a.proxySrv.ReloadConfig()
	a.emitStateChanged()
	return nil
}

// SetAutoStart 设置开机自启动。
func (a *App) SetAutoStart(enabled bool) error {
	return config.SetAutoStartEnabled(enabled)
}

// GenerateAccessToken 生成随机访问令牌。
func (a *App) GenerateAccessToken() string {
	return config.GenerateAccessToken()
}

// ---------- 事件推送 ----------

// emitStateChanged 推送"状态变化"事件给前端，触发对应页面刷新。
func (a *App) emitStateChanged() {
	if a.ctx != nil {
		runtime.EventsEmit(a.ctx, "state:changed")
	}
}

// NotifyStateChanged 供外部（如 main）在业务状态变化后调用。
func (a *App) NotifyStateChanged() {
	a.emitStateChanged()
}
