package singleinst

import (
	"fmt"
	"testing"
	"time"
)

// uniqueName 生成唯一的锁名称，避免并行/重复运行间冲突
func uniqueName(t *testing.T) string {
	return fmt.Sprintf("test-%d-%s", time.Now().UnixNano(), t.Name()[:8])
}

// TestTryLockRelease 验证：首次获取成功，重复获取返回 ErrAlreadyRunning，释放后可重新获取。
func TestTryLockRelease(t *testing.T) {
	name := uniqueName(t)

	// 首次获取成功
	inst, err := TryLock(name)
	if err != nil {
		t.Fatalf("首次 TryLock 应成功, got: %v", err)
	}
	defer inst.Release()

	// 重复获取应返回 ErrAlreadyRunning
	if _, err := TryLock(name); err != ErrAlreadyRunning {
		t.Fatalf("重复 TryLock 应返回 ErrAlreadyRunning, got: %v", err)
	}

	// 释放后可重新获取
	inst.Release()
	inst2, err := TryLock(name)
	if err != nil {
		t.Fatalf("释放后重新 TryLock 应成功, got: %v", err)
	}
	inst2.Release()

	// Release 幂等：重复释放不应 panic
	inst.Release()
	inst2.Release()
}

// TestActivateCallback 验证：新实例尝试启动时，主实例的激活回调被触发。
func TestActivateCallback(t *testing.T) {
	name := uniqueName(t)

	inst, err := TryLock(name)
	if err != nil {
		t.Fatalf("首次 TryLock 应成功, got: %v", err)
	}
	defer inst.Release()

	// 注册激活回调
	activated := make(chan struct{}, 1)
	inst.OnActivate(func() {
		activated <- struct{}{}
	})

	// 再次启动（模拟新实例）应触发主实例的激活回调
	if _, err := TryLock(name); err != ErrAlreadyRunning {
		t.Fatalf("重复 TryLock 应返回 ErrAlreadyRunning, got: %v", err)
	}

	select {
	case <-activated:
		// 成功
	case <-time.After(2 * time.Second):
		t.Fatal("激活回调未被触发")
	}
}
