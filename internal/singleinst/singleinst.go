// Package singleinst 提供跨平台单实例锁，确保同一时间只有一个 AIProxy 进程在运行。
// 当新实例尝试启动时，会自动通知已有实例显示主窗口。
package singleinst

import (
	"errors"
	"sync"
)

// warnAlreadyRunning 弹出"已有实例在运行"的提示（平台相关实现，见 *_warn.go）。
// 供新实例在 TryLock 检测到重复运行时调用。
var warnAlreadyRunning = func() {}

// ErrAlreadyRunning 表示已有同名实例在运行。
// 返回该错误时，新实例应直接退出，无需再做任何清理。
var ErrAlreadyRunning = errors.New("已有 AIProxy 实例在运行")

// Instance 表示当前进程持有的单实例锁。
type Instance struct {
	// release 平台相关的锁释放函数（Windows 为互斥体释放，其他平台为 flock 释放）
	release func()

	// onActivate 主实例注册的"被新实例请求激活"回调
	onActivate func()

	// stop 停止后台激活等待 goroutine 的信号；Release 时关闭
	stop chan struct{}
	// stopOnce 保证 stop 只被关闭一次
	stopOnce sync.Once

	// warn 弹出"已有实例在运行"提示；仅由新实例（TryLock 返回错误后）调用
	warn func()

	// warnOnce 保证提示只弹一次
	warnOnce sync.Once
}

// Release 释放单实例锁并停止后台等待 goroutine。
// 程序正常退出或重启前必须调用，否则其他实例无法启动。
func (i *Instance) Release() {
	if i == nil {
		return
	}
	closeStop := func() {
		if i.stop != nil {
			i.stopOnce.Do(func() { close(i.stop) })
		}
	}
	closeStop()
	if i.release != nil {
		i.release()
		i.release = nil
	}
}

// OnActivate 注册主实例被新实例请求激活时的回调（例如显示主窗口）。
// 仅持有锁的主实例有效；回调在独立 goroutine 中触发，调用方需自行切换到 UI 线程。
func (i *Instance) OnActivate(fn func()) {
	if i != nil {
		i.onActivate = fn
	}
}

// notifyActivate 触发激活回调（平台实现调用）。
func (i *Instance) notifyActivate() {
	if i != nil && i.onActivate != nil {
		i.onActivate()
	}
}

// ShowAlreadyRunningMessage 弹出"请勿重复运行"提示框。
// 新实例在 TryLock 检测到已有实例时调用；重复调用只弹一次。
func ShowAlreadyRunningMessage() {
	warnAlreadyRunning()
}

// ShowAlreadyRunningMessage 弹出"请勿重复运行"提示框。
// 新实例检测到已有实例在运行时调用（持有锁的主实例通常不使用）。
func (i *Instance) ShowAlreadyRunningMessage() {
	if i == nil {
		ShowAlreadyRunningMessage()
		return
	}
	i.warnOnce.Do(func() {
		if i.warn != nil {
			i.warn()
		} else {
			warnAlreadyRunning()
		}
	})
}
