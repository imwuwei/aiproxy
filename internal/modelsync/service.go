package modelsync

import (
	"context"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"

	"aiproxy/internal/config"
	"aiproxy/internal/models"
	"aiproxy/internal/proxy/relay"
	"aiproxy/internal/store"
)

// Service 模型同步服务
type Service struct {
	db       *store.ChannelModelStore
	channels *store.ChannelStore
	settings *store.SettingsStore

	mu       sync.Mutex
	ticker   *time.Ticker
	stopCh   chan struct{}
	interval time.Duration
	client   *http.Client
	running  bool
}

// New 创建模型同步服务
func New(db *store.ChannelModelStore, channels *store.ChannelStore, settings *store.SettingsStore) *Service {
	return &Service{
		db:       db,
		channels: channels,
		settings: settings,
		client:   &http.Client{Timeout: 60 * time.Second},
		stopCh:   make(chan struct{}),
	}
}

// Start 启动定时刷新
func (s *Service) Start() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.running {
		return
	}
	s.loadInterval()
	s.running = true
	s.ticker = time.NewTicker(s.interval)

	go func() {
		// 启动后立即执行一次全量同步
		s.SyncAll()
		for {
			select {
			case <-s.ticker.C:
				s.SyncAll()
			case <-s.stopCh:
				s.ticker.Stop()
				return
			}
		}
	}()
}

// Stop 停止定时刷新
func (s *Service) Stop() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.running {
		return
	}
	s.running = false
	close(s.stopCh)
	s.stopCh = make(chan struct{})
}

// loadInterval 从设置加载刷新间隔
func (s *Service) loadInterval() {
	if v, err := s.settings.Get(models.SettingsModelSyncInterval); err == nil && v != "" {
		if n, err := parseInt(v); err == nil && n > 0 {
			s.interval = time.Duration(n) * time.Minute
			return
		}
	}
	s.interval = config.DefaultConfig().ModelSyncInterval
}

// RefreshInterval 返回当前刷新间隔（供 GUI 显示）
func (s *Service) RefreshInterval() time.Duration {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.interval
}

// SetInterval 更新刷新间隔（供 GUI 设置变更后调用）
func (s *Service) SetInterval(d time.Duration) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.interval = d
	if s.running {
		s.ticker.Reset(d)
	}
}

// debugEnabled 读取当前调试开关
func (s *Service) debugEnabled() bool {
	v, err := s.settings.Get(models.SettingsDebug)
	if err != nil || v == "" {
		return false
	}
	return v == "true" || v == "1"
}

// debugf 输出调试日志（仅当启用 Debug 时）
func (s *Service) debugf(format string, args ...any) {
	if s.debugEnabled() {
		log.Printf("[aiproxy][debug] "+format, args...)
	}
}

// SyncChannel 同步单个渠道模型
func (s *Service) SyncChannel(ctx context.Context, ch *models.Channel) error {
	rel, ok := relay.Get(ch.Type)
	if !ok {
		return fmt.Errorf("不支持的厂商类型: %s", ch.Type)
	}

	modelList, err := s.fetchModels(ctx, rel, ch)
	if err != nil {
		_ = s.channels.UpdateStatus(ch.ID, models.ChannelStatusOffline, err.Error(), nil)
		return err
	}
	if len(modelList) == 0 {
		// 空列表不出错，但更新状态
		log.Printf("[modelsync] 渠道 %s 未返回模型列表", ch.Name)
	}

	// 同一供应商返回的模型列表可能含有重名模型，去重后只保留一个
	modelList = dedupeModels(modelList)

	if err := s.db.Replace(ch.ID, modelList); err != nil {
		return err
	}
	now := time.Now()
	if err := s.channels.UpdateStatus(ch.ID, models.ChannelStatusOnline, "", &now); err != nil {
		return err
	}
	s.debugf("同步渠道[%s] 模型数=%d", ch.Name, len(modelList))
	return nil
}

// SyncAll 全量同步所有启用渠道
func (s *Service) SyncAll() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.loadInterval()

	channels, err := s.channels.ListEnabled()
	if err != nil {
		log.Printf("[modelsync] 加载渠道失败: %v", err)
		return
	}

	var wg sync.WaitGroup
	for _, ch := range channels {
		wg.Add(1)
		go func(c *models.Channel) {
			defer wg.Done()
			ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
			defer cancel()
			if err := s.SyncChannel(ctx, c); err != nil {
				log.Printf("[modelsync] 渠道 %s 同步失败: %v", c.Name, err)
			}
		}(ch)
	}
	wg.Wait()
	log.Printf("[modelsync] 全量同步完成，共 %d 个渠道", len(channels))
}

// dedupeModels 去除模型列表中的重名项，保留首次出现的顺序
func dedupeModels(models []string) []string {
	seen := make(map[string]struct{}, len(models))
	deduped := make([]string, 0, len(models))
	for _, m := range models {
		if _, ok := seen[m]; ok {
			continue
		}
		seen[m] = struct{}{}
		deduped = append(deduped, m)
	}
	return deduped
}

// TestChannel 测试渠道连接（返回模型数）
func (s *Service) TestChannel(ch *models.Channel) (int, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	rel, ok := relay.Get(ch.Type)
	if !ok {
		return 0, fmt.Errorf("不支持的厂商类型: %s", ch.Type)
	}
	modelList, err := s.fetchModels(ctx, rel, ch)
	if err != nil {
		return 0, err
	}
	return len(modelList), nil
}

// fetchModels 拉取渠道模型列表（不写入）。
// 默认携带认证头；若带认证请求返回 401/403/400（权限类错误），
// 自动重试一次无认证请求（兼容 Ollama、自建代理等无需认证的接口）。
func (s *Service) fetchModels(ctx context.Context, rel relay.Relay, ch *models.Channel) ([]string, error) {
	keyIdx := 0
	authReq, err := rel.BuildModelsRequest(ctx, ch, keyIdx)
	if err != nil {
		return nil, err
	}
	s.debugf("获取模型渠道[%s] url=%s 携带认证", ch.Name, authReq.URL.String())
	reqStart := time.Now()
	resp, err := s.client.Do(authReq)
	if err != nil {
		return nil, err
	}
	s.debugf("渠道[%s] 响应 %s 耗时=%dms", ch.Name, resp.Status, time.Since(reqStart).Milliseconds())

	// 权限类错误 → 回退无认证请求重试一次
	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden || resp.StatusCode == http.StatusBadRequest {
		errBody, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		resp.Body.Close()
		authErr := fmt.Sprintf("HTTP %d: %s", resp.StatusCode, rel.ParseError(errBody))
		s.debugf("渠道[%s] 带认证请求失败(%s)，尝试无认证请求", ch.Name, authErr)

		noAuthReq, err := rel.BuildModelsRequest(ctx, ch, keyIdx)
		if err != nil {
			return nil, err
		}
		noAuthReq.Header.Del("Authorization")
		noAuthReq.Header.Del("X-Api-Key")
		noAuthReq.Header.Del("X-Goog-Api-Key")
		s.debugf("获取模型渠道[%s] url=%s 无需认证", ch.Name, noAuthReq.URL.String())
		resp2, err := s.client.Do(noAuthReq)
		if err != nil {
			return nil, err
		}
		s.debugf("渠道[%s] 无认证请求响应 %s 耗时=%dms", ch.Name, resp2.Status, time.Since(reqStart).Milliseconds())
		return s.parseModelsResponse(rel, resp2)
	}

	return s.parseModelsResponse(rel, resp)
}

// parseModelsResponse 解析模型响应并统一处理 HTTP 错误
func (s *Service) parseModelsResponse(rel relay.Relay, resp *http.Response) ([]string, error) {
	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		resp.Body.Close()
		errMsg := fmt.Sprintf("HTTP %d: %s", resp.StatusCode, rel.ParseError(body))
		if strings.TrimSpace(errMsg) == "" {
			errMsg = resp.Status
		}
		return nil, fmt.Errorf("模型列表请求失败(%d): %s", resp.StatusCode, errMsg)
	}
	defer resp.Body.Close()
	return rel.ParseModelsResponse(resp)
}

func parseInt(s string) (int, error) {
	var n int
	_, err := fmt.Sscanf(s, "%d", &n)
	return n, err
}
