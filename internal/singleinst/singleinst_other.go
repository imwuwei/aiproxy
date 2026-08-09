//go:build !windows

package singleinst

import (
	"fmt"
	"os"
	"path/filepath"
	"syscall"
	"time"
)

// lockFileExt 锁文件扩展名；activateExt 为激活标记文件扩展名。
const (
	mutexFileExt = ".lock"
	activateExt  = ".activate"
	pollInterval = 200 * time.Millisecond
)

// lockFilePath 返回锁文件路径（系统临时目录，进程退出后文件残留不影响 flock 语义）
func lockFilePath(name string) string {
	return filepath.Join(os.TempDir(), "aiproxy-"+name+mutexFileExt)
}

// activateMarkPath 返回激活标记文件路径
func activateMarkPath(name string) string {
	return filepath.Join(os.TempDir(), "aiproxy-"+name+activateExt)
}

// TryLock 尝试获取单实例锁。
// 若已有实例在运行，创建激活标记文件通知已有实例显示主窗口，并返回 ErrAlreadyRunning。
// 成功时返回持有锁的 Instance，调用方在进程退出前必须 Release。
func TryLock(name string) (*Instance, error) {
	path := lockFilePath(name)

	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o666)
	if err != nil {
		return nil, fmt.Errorf("创建单实例锁文件失败: %w", err)
	}

	// LOCK_EX | LOCK_NB：非阻塞独占锁；已有实例持有锁时立即失败
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		_ = f.Close()
		if err == syscall.EWOULDBLOCK || err == syscall.EAGAIN {
			// 已有实例在运行：创建激活标记文件通知其显示主窗口
			_ = os.WriteFile(activateMarkPath(name), []byte(time.Now().Format(time.RFC3339)), 0o644)
			return nil, ErrAlreadyRunning
		}
		return nil, fmt.Errorf("获取单实例锁失败: %w", err)
	}

	inst := &Instance{
		stop: make(chan struct{}),
		release: func() {
			_ = syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
			_ = f.Close()
		},
	}
	go inst.watchActivateMark(name, inst.stop)
	return inst, nil
}

// watchActivateMark 轮询激活标记文件，发现时删除并触发 notifyActivate，通知主界面显示窗口。
// stop 关闭（Release）后立即返回。
func (i *Instance) watchActivateMark(name string, stop <-chan struct{}) {
	mark := activateMarkPath(name)
	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-stop:
			return
		case <-ticker.C:
			if _, err := os.Stat(mark); err == nil {
				_ = os.Remove(mark)
				i.notifyActivate()
			}
		}
	}
}
