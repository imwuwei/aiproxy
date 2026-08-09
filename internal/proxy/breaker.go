package proxy

import (
	"sync"
	"time"
)

// breakerState 单个渠道的熔断状态
type breakerState struct {
	consecutiveFailures int
	cooldownUntil       time.Time
}

// Breaker 熔断管理器
type Breaker struct {
	mu        sync.Mutex
	states    map[int64]*breakerState
	threshold int
	cooldown  time.Duration
}

// NewBreaker 创建熔断管理器
func NewBreaker(threshold int, cooldown time.Duration) *Breaker {
	return &Breaker{
		states:    make(map[int64]*breakerState),
		threshold: threshold,
		cooldown:  cooldown,
	}
}

// UpdateParams 更新熔断参数
func (b *Breaker) UpdateParams(threshold int, cooldown time.Duration) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.threshold = threshold
	b.cooldown = cooldown
}

// RecordFailure 记录一次失败，返回是否进入冷却
func (b *Breaker) RecordFailure(channelID int64) bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	st := b.getState(channelID)
	st.consecutiveFailures++
	if st.consecutiveFailures >= b.threshold {
		st.cooldownUntil = time.Now().Add(b.cooldown)
		return true
	}
	return false
}

// RecordSuccess 记录一次成功，清零失败计数
func (b *Breaker) RecordSuccess(channelID int64) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if st, ok := b.states[channelID]; ok {
		st.consecutiveFailures = 0
		st.cooldownUntil = time.Time{}
	}
}

// IsCooling 判断渠道是否处于冷却期
func (b *Breaker) IsCooling(channelID int64) bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	st, ok := b.states[channelID]
	if !ok {
		return false
	}
	if time.Now().After(st.cooldownUntil) {
		// 冷却结束自动恢复
		st.consecutiveFailures = 0
		st.cooldownUntil = time.Time{}
		return false
	}
	return st.consecutiveFailures >= b.threshold
}

func (b *Breaker) getState(channelID int64) *breakerState {
	st, ok := b.states[channelID]
	if !ok {
		st = &breakerState{}
		b.states[channelID] = st
	}
	return st
}
