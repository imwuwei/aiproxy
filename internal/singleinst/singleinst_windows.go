//go:build windows

package singleinst

import (
	"sync"

	"golang.org/x/sys/windows"
)

// 命名对象前缀，避免与系统中其他应用冲突。
// 命名互斥体与命名事件均使用 Local\（当前会话域），无需管理员权限。
const (
	mutexPrefix = `Local\AIProxy_`
	eventPrefix = `Local\AIProxy_Activate_`
)

// TryLock 尝试获取单实例锁。
// 若已有实例在运行，仅设置激活事件通知已有实例显示主窗口，并返回 ErrAlreadyRunning。
// 成功时返回持有锁的 Instance，调用方在进程退出前必须 Release。
func TryLock(name string) (*Instance, error) {
	mutexName := mutexPrefix + name

	// 先创建互斥体再检查错误码：
	// CreateMutex 在互斥体已存在时返回有效句柄 + ERROR_ALREADY_EXISTS，
	// 必须先判断该错误码，再判断 err != nil（避免把已存在误判为其他错误）。
	h, err := windows.CreateMutex(nil, false, windows.StringToUTF16Ptr(mutexName))
	if err == windows.ERROR_ALREADY_EXISTS {
		// 已有实例在运行：设置激活事件通知其显示主窗口，然后释放句柄返回错误
		windowName := eventPrefix + name
		ev, createErr := windows.CreateEvent(nil, 0, 0, windows.StringToUTF16Ptr(windowName))
		if createErr == nil {
			_ = windows.SetEvent(ev)
			windows.CloseHandle(ev)
		}
		windows.CloseHandle(h)
		return nil, ErrAlreadyRunning
	}
	if err != nil {
		windows.CloseHandle(h)
		return nil, err
	}

	// 当前进程成为主实例：创建命名事件，并在后台等待激活请求
	windowName := eventPrefix + name
	ev, err := windows.CreateEvent(nil, 0, 0, windows.StringToUTF16Ptr(windowName))
	if err != nil {
		// 事件创建失败不影响锁本身，仅丧失被激活能力，继续持有锁
		return &Instance{
			stop:    make(chan struct{}),
			release: mutexRelease(h),
		}, nil
	}

	inst := &Instance{
		stop: make(chan struct{}),
		release: func() {
			// 关闭事件句柄可唤醒正在等待的临时 goroutine（返回 WAIT_FAILED 后退出），
			// 再释放互斥体
			mutexRelease(h)()
			windows.CloseHandle(ev)
		},
	}
	go inst.waitActivate(ev, inst.stop)
	return inst, nil
}

// mutexRelease 释放命名互斥体并关闭句柄
func mutexRelease(h windows.Handle) func() {
	var once sync.Once
	return func() {
		once.Do(func() {
			_ = windows.ReleaseMutex(h)
			windows.CloseHandle(h)
		})
	}
}

// waitActivate 后台监听新实例的激活请求。
// stop 关闭后立即返回（Release 时触发）；事件句柄在 Release 中被关闭，
// 可能仍有临时 goroutine 阻塞在 WaitForSingleObject 上，句柄关闭后其返回 WAIT_FAILED，
// 向带缓冲的通道发送结果后自然退出。
func (i *Instance) waitActivate(ev windows.Handle, stop <-chan struct{}) {
	type sig struct {
		h   uint32
		err error
	}

	for {
		ch := make(chan sig, 1)
		go func() {
			h, err := windows.WaitForSingleObject(ev, windows.INFINITE)
			ch <- sig{h, err}
		}()

		select {
		case <-stop:
			return
		case r := <-ch:
			// 句柄已关闭（Release）或等待出错，终止监听
			if r.err != nil || r.h == windows.WAIT_FAILED {
				return
			}
			i.notifyActivate()
			// 自动重置事件(Win32 默认) 已被本次等待消费，继续等待下次请求
		}
	}
}
