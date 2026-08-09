package proxy

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"sync"
	"time"

	"aiproxy/internal/config"
	"aiproxy/internal/store"
)

// Server 代理服务
type Server struct {
	mu        sync.RWMutex
	srv       *http.Server
	running   bool
	cfg       *config.Config
	cfgLoader func() *config.Config

	channels *store.ChannelStore
	models   *store.ChannelModelStore
	usage    *store.UsageStore
	aliases  *store.ModelAliasStore
	breaker  *Breaker
	client   *http.Client
	// streamClient 流式请求专用客户端：不设整体超时，仅限制响应头部等待时间，
	// 避免长流式（如长时间推理输出）被总超时截断。
	streamClient *http.Client
	keyCount     map[int64]*int64
	// aliasIdx 别名加权轮询计数（名称 → 下一目标下标）
	aliasIdx map[string]int64
}

// NewServer 创建代理服务
// aliasStore 可为 nil，此时别名功能不可用（普通模型路由不受影响）。
func NewServer(cfgLoader func() *config.Config, channelStore *store.ChannelStore, modelStore *store.ChannelModelStore, usageStore *store.UsageStore, aliasStore *store.ModelAliasStore) *Server {
	s := &Server{
		cfgLoader: cfgLoader,
		channels:  channelStore,
		models:    modelStore,
		usage:     usageStore,
		aliases:   aliasStore,
		breaker:   NewBreaker(5, 30*time.Second),
		keyCount:  make(map[int64]*int64),
		aliasIdx:  make(map[string]int64),
	}
	s.applyConfig()
	return s
}

// applyConfig 应用最新配置
func (s *Server) applyConfig() {
	if s.cfgLoader != nil {
		s.cfg = s.cfgLoader()
	}
	s.client = &http.Client{
		Timeout: s.cfg.ProxyTimeout,
	}
	// 流式客户端：无整体 Timeout，仅限制首响应头等待时间。
	// 请求上下文（r.Context()）负责最终取消，客户端断开即可中断上游。
	s.streamClient = &http.Client{
		Transport: &http.Transport{
			MaxIdleConns:          100,
			MaxIdleConnsPerHost:   8,
			ResponseHeaderTimeout: s.cfg.ProxyTimeout,
		},
	}
	s.breaker.UpdateParams(s.cfg.BreakerThreshold, s.cfg.BreakerCooldown)
}

// ReloadConfig 重新加载并应用最新配置（供设置保存后热更新调用）。
// 代理服务运行时调用同样安全：client/streamClient/breaker 会在后续请求中生效。
func (s *Server) ReloadConfig() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.applyConfig()
}

// Config 获取当前配置
func (s *Server) Config() *config.Config {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.cfg
}

// Running 是否运行中
func (s *Server) Running() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.running
}

// Start 启动代理服务
func (s *Server) Start() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.running {
		return fmt.Errorf("代理服务已在运行")
	}
	s.applyConfig()

	mux := http.NewServeMux()
	mux.HandleFunc("/v1/chat/completions", s.handleChatCompletions)
	mux.HandleFunc("/v1/embeddings", s.handleEmbeddings)
	mux.HandleFunc("/v1/models", s.handleModels)
	mux.HandleFunc("/health", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})

	addr := s.cfg.ProxyAddr()
	s.srv = &http.Server{
		Addr:    addr,
		Handler: s.authMiddleware(mux),
	}
	s.running = true

	go func() {
		log.Printf("[aiproxy] 代理服务已启动: http://%s", addr)
		if err := s.srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Printf("[aiproxy] 代理服务错误: %v", err)
		}
	}()
	return nil
}

// Stop 停止代理服务
func (s *Server) Stop() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.running {
		return nil
	}
	if s.srv != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := s.srv.Shutdown(ctx); err != nil {
			return err
		}
	}
	s.running = false
	log.Printf("[aiproxy] 代理服务已停止")
	return nil
}

// authMiddleware 鉴权中间件
func (s *Server) authMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// 加锁读取配置快照，避免与 ReloadConfig 并发写 s.cfg 产生数据竞争
		s.mu.RLock()
		authEnabled := s.cfg.AuthEnabled
		accessToken := s.cfg.AccessToken
		s.mu.RUnlock()
		if authEnabled && accessToken != "" {
			auth := r.Header.Get("Authorization")
			expected := "Bearer " + accessToken
			if auth != expected {
				writeOpenAIError(w, http.StatusUnauthorized, "invalid_api_key", "未提供有效的访问令牌")
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}

// nextKeyIndex 轮询获取渠道 API Key 下标
func (s *Server) nextKeyIndex(channelID int64) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	cnt, ok := s.keyCount[channelID]
	if !ok {
		n := int64(0)
		cnt = &n
		s.keyCount[channelID] = cnt
	}
	idx := int(*cnt)
	*cnt++
	return idx
}

// debugf 输出调试日志（仅当启用 Debug 时）
func (s *Server) debugf(format string, args ...any) {
	s.mu.RLock()
	debug := s.cfg != nil && s.cfg.Debug
	s.mu.RUnlock()
	if debug {
		log.Printf("[aiproxy][debug] "+format, args...)
	}
}
