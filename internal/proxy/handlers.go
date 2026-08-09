package proxy

import (
	"encoding/json"
	"io"
	"log"
	"net/http"
	"strings"
	"time"

	"aiproxy/internal/models"
	"aiproxy/internal/proxy/relay"
)

// chatRequest 解析请求结构
type chatRequest struct {
	Model  string `json:"model"`
	Stream bool   `json:"stream"`
}

// handleChatCompletions 处理聊天补全请求
func (s *Server) handleChatCompletions(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeOpenAIError(w, http.StatusMethodNotAllowed, "method_not_allowed", "仅支持 POST 请求")
		return
	}
	body, err := readRequestBody(r)
	if err != nil {
		writeOpenAIError(w, http.StatusBadRequest, "invalid_request", "读取请求体失败: "+err.Error())
		return
	}
	var cr chatRequest
	if err := json.Unmarshal(body, &cr); err != nil || cr.Model == "" {
		writeOpenAIError(w, http.StatusBadRequest, "invalid_request", "请求体缺少 model 字段")
		return
	}

	start := time.Now()
	s.debugf("POST /v1/chat/completions model=%s stream=%v", cr.Model, cr.Stream)
	usage, statusCode, errMsg, reqErr, channelID, channelName := s.relayWithFailover(w, r, body, cr.Model, cr.Stream)
	s.debugf("POST /v1/chat/completions model=%s 完成 status=%d 耗时=%dms", cr.Model, statusCode, time.Since(start).Milliseconds())

	s.recordUsage(r, cr.Model, start, usage, statusCode, errMsg, reqErr, channelID, channelName)
}

// handleEmbeddings 处理嵌入请求
func (s *Server) handleEmbeddings(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeOpenAIError(w, http.StatusMethodNotAllowed, "method_not_allowed", "仅支持 POST 请求")
		return
	}
	body, err := readRequestBody(r)
	if err != nil {
		writeOpenAIError(w, http.StatusBadRequest, "invalid_request", "读取请求体失败: "+err.Error())
		return
	}
	var cr chatRequest
	if err := json.Unmarshal(body, &cr); err != nil || cr.Model == "" {
		writeOpenAIError(w, http.StatusBadRequest, "invalid_request", "请求体缺少 model 字段")
		return
	}

	start := time.Now()
	s.debugf("POST /v1/embeddings model=%s", cr.Model)
	usage, statusCode, errMsg, reqErr, channelID, channelName := s.relayWithFailover(w, r, body, cr.Model, false)
	s.debugf("POST /v1/embeddings model=%s 完成 status=%d 耗时=%dms", cr.Model, statusCode, time.Since(start).Milliseconds())

	s.recordUsage(r, cr.Model, start, usage, statusCode, errMsg, reqErr, channelID, channelName)
}

// readRequestBody 读取请求体（最多 128MB）。
// 对 chunked 流式请求体同样整体读取后用于转发，保证后续故障转移
// 可复用同一字节流；读取期间保持传输以支持"流式输入"。
func readRequestBody(r *http.Request) ([]byte, error) {
	return io.ReadAll(io.LimitReader(r.Body, 128*1024*1024))
}

// relayWithFailover 带故障转移的中继核心。
// 若 model 是已启用的模型别名，则在别名目标模型之间做加权轮询 + 模型级故障转移；
// 否则按原逻辑对单模型的所有支持渠道故障转移。
// 返回值：usage（nil 表示失败）、最终状态码、错误消息、内部错误、实际使用(或最后尝试)的渠道 ID、渠道名称
func (s *Server) relayWithFailover(w http.ResponseWriter, r *http.Request, body []byte, model string, stream bool) (*relay.UsageAccumulator, int, string, error, int64, string) {
	alias, targets := s.resolveAlias(model)
	if alias != nil && len(targets) > 0 {
		return s.relayAliasWithFailover(w, r, body, model, targets, stream)
	}

	// 普通模型：单模型多渠道故障转移
	usage, statusCode, errMsg, reqErr, channelID, channelName, next, failReasons := s.relayModelWithChannels(w, r, body, model, stream, "")
	if !next {
		return usage, statusCode, errMsg, reqErr, channelID, channelName
	}
	// 所有渠道失败
	msg := "所有渠道均失败: " + strings.Join(failReasons, " | ")
	log.Printf("[aiproxy] 模型 %s 请求失败: %s", model, msg)
	writeOpenAIError(w, http.StatusBadGateway, "upstream_error", msg)
	return nil, http.StatusBadGateway, msg, nil, channelID, channelName
}

// relayAliasWithFailover 别名路由核心：
// 按权重展开目标模型为轮询序列，从轮询起点依次尝试每个目标模型；
// 单个目标模型内部仍按渠道优先级故障转移；全部目标模型均失败才返回错误。
// 响应中的 model 字段通过写入包装器还原为别名。
func (s *Server) relayAliasWithFailover(w http.ResponseWriter, r *http.Request, body []byte, aliasName string, targets models.ModelAliasTargetList, stream bool) (*relay.UsageAccumulator, int, string, error, int64, string) {
	seq := weightedSeq(targets)
	start := s.nextAliasIndex(aliasName) % len(seq)
	var failReasons []string
	var lastChannelID int64
	var lastChannelName string

	for i := 0; i < len(seq); i++ {
		targetModel := seq[(start+i)%len(seq)]
		s.debugf("别名 %s → 尝试目标模型 %s", aliasName, targetModel)

		// 改写请求体中的 model 字段为真实模型名，再构造上游请求
		aliasedBody, err := rewriteModel(body, targetModel)
		if err != nil {
			failReasons = append(failReasons, targetModel+"(请求体改写失败)")
			continue
		}

		usage, status, errMsg, reqErr, chID, chName, next, reasons := s.relayModelWithChannels(w, r, aliasedBody, targetModel, stream, aliasName)
		failReasons = append(failReasons, reasons...)
		lastChannelID = chID
		lastChannelName = chName
		if !next {
			return usage, status, errMsg, reqErr, chID, chName
		}
	}

	msg := "所有渠道均失败: " + strings.Join(failReasons, " | ")
	log.Printf("[aiproxy] 别名 %s 请求失败: %s", aliasName, msg)
	writeOpenAIError(w, http.StatusBadGateway, "upstream_error", msg)
	return nil, http.StatusBadGateway, msg, nil, lastChannelID, lastChannelName
}

// relayModelWithChannels 对单个模型的所有支持渠道执行故障转移（按渠道优先级升序）。
// restoreAlias 非空时，将上游响应中的 model 字段还原为该别名（流式与非流式均生效）。
// 返回值 next 为 true 表示调用方可继续尝试下一个模型（本次未写最终响应）；
// 为 false 表示响应已写出或发生不可继续的错误，调用方应立即返回。
// failReasons 收集失败原因，供上层聚合错误消息。
func (s *Server) relayModelWithChannels(w http.ResponseWriter, r *http.Request, body []byte, model string, stream bool, restoreAlias string) (usage *relay.UsageAccumulator, statusCode int, errMsg string, reqErr error, channelID int64, channelName string, next bool, failReasons []string) {
	channels, err := s.models.ListChannelsByModel(model)
	if err != nil {
		writeOpenAIError(w, http.StatusInternalServerError, "internal_error", "查询渠道失败: "+err.Error())
		return nil, http.StatusInternalServerError, "", err, 0, "", false, failReasons
	}
	if len(channels) == 0 {
		failReasons = append(failReasons, model+"(无可用渠道)")
		return nil, 0, "", nil, 0, "", true, failReasons
	}

	// 别名请求：包装响应写入器，将 model 字段还原为别名
	out := w
	if restoreAlias != "" {
		out = &modelRestoreWriter{ResponseWriter: w, realModel: model, alias: restoreAlias}
	}

	for _, ch := range channels {
		// channelID/channelName 始终记录为最后处理（或最后尝试）的渠道，供上层日志/统计
		channelID = ch.ID
		channelName = ch.Name

		if s.breaker.IsCooling(ch.ID) {
			failReasons = append(failReasons, ch.Name+"(熔断冷却中)")
			continue
		}
		rel, ok := relay.Get(ch.Type)
		if !ok {
			failReasons = append(failReasons, ch.Name+"(不支持的厂商类型)")
			continue
		}

		keyIdx := s.nextKeyIndex(ch.ID)
		upstreamReq, err := rel.BuildChatRequest(r.Context(), ch, body, keyIdx)
		if err != nil {
			failReasons = append(failReasons, ch.Name+"(构造请求失败: "+err.Error()+")")
			continue
		}

		s.debugf("尝试渠道[%s] priority=%d url=%s stream=%v", ch.Name, ch.Priority, upstreamReq.URL.String(), stream)
		reqStart := time.Now()

		// 流式请求使用专用客户端：无整体超时，避免长流式被截断。
		// 加锁取当前配置对应客户端快照，避免与 ReloadConfig 并发替换产生数据竞争，
		// 同时确保超时等配置热更新后下一次请求即生效。
		s.mu.RLock()
		httpClient := s.client
		if stream {
			httpClient = s.streamClient
		}
		s.mu.RUnlock()
		resp, err := httpClient.Do(upstreamReq)
		if err != nil {
			s.debugf("渠道[%s] 网络错误: %v", ch.Name, err)
			failReasons = append(failReasons, ch.Name+"(网络错误: "+err.Error()+")")
			s.breaker.RecordFailure(ch.ID)
			continue
		}
		s.debugf("渠道[%s] 响应 %s 耗时=%dms", ch.Name, resp.Status, time.Since(reqStart).Milliseconds())

		// 上游返回错误（401/429/5xx）→ 故障转移
		if resp.StatusCode >= 400 {
			errBody, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
			resp.Body.Close()
			msg := rel.ParseError(errBody)
			failReasons = append(failReasons, ch.Name+"("+resp.Status+": "+msg+")")
			s.breaker.RecordFailure(ch.ID)
			_ = s.channels.UpdateStatus(ch.ID, models.ChannelStatusOffline, resp.Status+": "+msg, nil)
			continue
		}

		// 成功路径
		s.breaker.RecordSuccess(ch.ID)
		now := time.Now()
		_ = s.channels.UpdateStatus(ch.ID, models.ChannelStatusOnline, "", &now)

		usage = &relay.UsageAccumulator{}
		if stream {
			// 若上游实际未返回流式内容（如 Content-Type 非 SSE），退化为缓冲
			ct := resp.Header.Get("Content-Type")
			if strings.Contains(strings.ToLower(ct), "text/event-stream") {
				if err := s.handleStreamingResponse(out, r, resp, rel, usage); err != nil {
					// 流式传输未正常完成（未收到结束标记或客户端断开）：
					// 使用 499 状态码标记为不完整请求，不计入成功 token 统计。
					return usage, 499, err.Error(), err, ch.ID, ch.Name, false, failReasons
				}
			} else {
				respBody, err := io.ReadAll(io.LimitReader(resp.Body, 64*1024*1024))
				resp.Body.Close()
				if err != nil {
					failReasons = append(failReasons, ch.Name+"(读取响应失败: "+err.Error()+")")
					continue
				}
				transformed, err := rel.TransformResponse(respBody, usage)
				if err != nil {
					writeOpenAIError(out, http.StatusBadGateway, "upstream_error", "上游响应转换失败: "+err.Error())
					return usage, http.StatusBadGateway, "", err, ch.ID, ch.Name, false, failReasons
				}
				out.Header().Set("Content-Type", "application/json")
				out.WriteHeader(resp.StatusCode)
				_, _ = out.Write(transformed)
			}
		} else {
			respBody, err := io.ReadAll(io.LimitReader(resp.Body, 64*1024*1024))
			resp.Body.Close()
			if err != nil {
				failReasons = append(failReasons, ch.Name+"(读取响应失败: "+err.Error()+")")
				continue
			}
			transformed, err := rel.TransformResponse(respBody, usage)
			if err != nil {
				writeOpenAIError(out, http.StatusBadGateway, "upstream_error", "上游响应转换失败: "+err.Error())
				return usage, http.StatusBadGateway, "", err, ch.ID, ch.Name, false, failReasons
			}
			out.Header().Set("Content-Type", "application/json")
			out.WriteHeader(resp.StatusCode)
			_, _ = out.Write(transformed)
		}
		return usage, http.StatusOK, "", nil, ch.ID, ch.Name, false, failReasons
	}

	// 所有渠道失败：返回可继续尝试下一个模型的标志
	return nil, 0, "", nil, channelID, channelName, true, failReasons
}

// handleStreamingResponse 逐块转发上游 SSE 流，实现真正的流式输出。
// 每收到一个上游数据块即写入下游并 Flush，客户端断开时主动取消上游请求。
// 返回错误表示流式传输未正常完成（未收到结束标记或客户端断开），
// 上层应据此将本次请求标记为不完整而非成功，避免记录部分 token。
func (s *Server) handleStreamingResponse(w http.ResponseWriter, r *http.Request, resp *http.Response, rel relay.Relay, usage *relay.UsageAccumulator) error {
	defer resp.Body.Close()

	flusher, ok := w.(http.Flusher)
	if !ok {
		// 不支持流式，退化为缓冲
		body, _ := io.ReadAll(resp.Body)
		transformed, _ := rel.TransformResponse(body, usage)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(200)
		_, _ = w.Write(transformed)
		return nil
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.WriteHeader(200)
	flusher.Flush()

	// 客户端断开时取消上游请求，避免资源泄漏
	done := make(chan struct{})
	go func() {
		select {
		case <-r.Context().Done():
			// 通过取消底层传输连接中断上游拉取
			if c, ok := resp.Body.(io.Closer); ok {
				_ = c.Close()
			}
		case <-done:
		}
	}()

	err := rel.TransformStream(resp.Body, w, usage)
	close(done)
	if err != nil {
		// 客户端断开属正常情况，仅调试日志
		if r.Context().Err() != nil {
			s.debugf("流式传输中断（客户端断开）: %v", err)
		} else {
			log.Printf("[aiproxy] 流式转换失败: %v", err)
		}
	}
	flusher.Flush()
	return err
}
