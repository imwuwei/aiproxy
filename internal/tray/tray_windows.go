//go:build windows

package tray

import (
	"crypto/md5"
	"encoding/hex"
	"io/ioutil"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"unsafe"

	"golang.org/x/sys/windows"
)

// Windows 原生托盘实现：
// 基于 Shell_NotifyIconW + 隐藏窗口消息循环。
// 左键单击（WM_LBUTTONUP）调用 opts.LeftClick 打开主界面；
// 右键单击（WM_RBUTTONUP）通过 TrackPopupMenu 弹出功能菜单。

const (
	WM_USER       = 0x0400
	WM_LBUTTONUP  = 0x0202
	WM_RBUTTONUP  = 0x0205
	WM_COMMAND    = 0x0111
	WM_ENDSESSION = 0x0016
	WM_CLOSE      = 0x0010
	WM_DESTROY    = 0x0002
	WM_QUIT       = 0x0012
	WM_TRAYICON   = WM_USER + 1

	// Shell_NotifyIcon
	NIM_ADD    = 0x00000000
	NIM_MODIFY = 0x00000001
	NIM_DELETE = 0x00000002

	NIF_MESSAGE = 0x00000001
	NIF_ICON    = 0x00000002
	NIF_TIP     = 0x00000004

	// TrackPopupMenu
	TPM_BOTTOMALIGN = 0x0020
	TPM_LEFTALIGN   = 0x0000

	// CreatePopupMenu / InsertMenuItem
	MFT_STRING  = 0x00000000
	MIIM_STRING = 0x00000040
	MIIM_ID     = 0x00000002
	MIIM_FTYPE  = 0x00000100

	// LoadImage
	IMAGE_ICON      = 1
	LR_LOADFROMFILE = 0x00000010
	LR_DEFAULTSIZE  = 0x00000040

	// ShowWindow
	SW_HIDE = 0

	// CreateWindowEx
	WS_OVERLAPPEDWINDOW = 0x00CF0000
	CW_USEDEFAULT       = 0x80000000

	// RegisterClassEx
	CS_HREDRAW = 0x0002
	CS_VREDRAW = 0x0001

	// 系统资源
	IDI_APPLICATION = 32512
	IDC_ARROW       = 32512
)

// 菜单项 ID（WM_COMMAND 的 wParam 低 16 位）
const (
	menuIDShow = 1
	menuIDQuit = 2
)

var (
	k32              = windows.NewLazySystemDLL("Kernel32.dll")
	pGetModuleHandle = k32.NewProc("GetModuleHandleW")

	s32              = windows.NewLazySystemDLL("Shell32.dll")
	pShellNotifyIcon = s32.NewProc("Shell_NotifyIconW")

	u32                  = windows.NewLazySystemDLL("User32.dll")
	pCreatePopupMenu     = u32.NewProc("CreatePopupMenu")
	pCreateWindowEx      = u32.NewProc("CreateWindowExW")
	pDefWindowProc       = u32.NewProc("DefWindowProcW")
	pDestroyWindow       = u32.NewProc("DestroyWindow")
	pDispatchMessage     = u32.NewProc("DispatchMessageW")
	pGetCursorPos        = u32.NewProc("GetCursorPos")
	pGetMessage          = u32.NewProc("GetMessageW")
	pInsertMenuItem      = u32.NewProc("InsertMenuItemW")
	pLoadCursor          = u32.NewProc("LoadCursorW")
	pLoadIcon            = u32.NewProc("LoadIconW")
	pLoadImage           = u32.NewProc("LoadImageW")
	pPostMessage         = u32.NewProc("PostMessageW")
	pPostQuitMessage     = u32.NewProc("PostQuitMessage")
	pRegisterClass       = u32.NewProc("RegisterClassExW")
	pRegisterWindowMsg   = u32.NewProc("RegisterWindowMessageW")
	pSetForegroundWindow = u32.NewProc("SetForegroundWindow")
	pShowWindow          = u32.NewProc("ShowWindow")
	pTrackPopupMenu      = u32.NewProc("TrackPopupMenu")
	pTranslateMessage    = u32.NewProc("TranslateMessage")
	pUnregisterClass     = u32.NewProc("UnregisterClassW")
	pUpdateWindow        = u32.NewProc("UpdateWindow")
)

// 回调函数（在 wndProc 线程中调用）
var (
	onLeftClick func()
	onShow      func()
	onQuit      func()
)

// wndClassEx 窗口类信息
type wndClassEx struct {
	Size, Style                        uint32
	WndProc                            uintptr
	ClsExtra, WndExtra                 int32
	Instance, Icon, Cursor, Background windows.Handle
	MenuName, ClassName                *uint16
	IconSm                             windows.Handle
}

// notifyIconData Shell_NotifyIcon 数据
type notifyIconData struct {
	Size                       uint32
	Wnd                        windows.Handle
	ID, Flags, CallbackMessage uint32
	Icon                       windows.Handle
	Tip                        [128]uint16
	State, StateMask           uint32
	Info                       [256]uint16
	Timeout, Version           uint32
	InfoTitle                  [64]uint16
	InfoFlags                  uint32
	GuidItem                   windows.GUID
	BalloonIcon                windows.Handle
}

// point 坐标
type point struct {
	X, Y int32
}

// menuItemInfo 菜单项信息
type menuItemInfo struct {
	Size, Mask, Type, State     uint32
	ID                          uint32
	SubMenu, Checked, Unchecked windows.Handle
	ItemData                    uintptr
	TypeData                    *uint16
	Cch                         uint32
	BMPItem                     windows.Handle
}

// msg GetMessageW 消息结构
type msg struct {
	hwnd    windows.Handle
	message uint32
	wparam  uintptr
	lparam  uintptr
	time    uint32
	pt      point
}

// winTray 托盘运行时状态
type winTray struct {
	instance,
	icon,
	cursor,
	window windows.Handle

	nid  *notifyIconData
	wcex *wndClassEx

	wmTrayIcon       uint32
	wmTaskbarCreated uint32

	menu windows.Handle

	muNID sync.RWMutex
}

var wt winTray

func (nid *notifyIconData) add() error {
	res, _, err := pShellNotifyIcon.Call(uintptr(NIM_ADD), uintptr(unsafe.Pointer(nid)))
	if res == 0 {
		return err
	}
	return nil
}

func (nid *notifyIconData) modify() error {
	res, _, err := pShellNotifyIcon.Call(uintptr(NIM_MODIFY), uintptr(unsafe.Pointer(nid)))
	if res == 0 {
		return err
	}
	return nil
}

func (nid *notifyIconData) delete() error {
	res, _, err := pShellNotifyIcon.Call(uintptr(NIM_DELETE), uintptr(unsafe.Pointer(nid)))
	if res == 0 && err != nil {
		return err
	}
	return nil
}

// register 注册窗口类
func (w *wndClassEx) register() error {
	w.Size = uint32(unsafe.Sizeof(*w))
	res, _, err := pRegisterClass.Call(uintptr(unsafe.Pointer(w)))
	if res == 0 {
		return err
	}
	return nil
}

func (w *wndClassEx) unregister() error {
	res, _, err := pUnregisterClass.Call(
		uintptr(unsafe.Pointer(w.ClassName)),
		uintptr(w.Instance),
	)
	if res == 0 && err != nil {
		return err
	}
	return nil
}

// wndProc 托盘隐藏窗口消息处理
func (t *winTray) wndProc(hWnd windows.Handle, message uint32, wParam, lParam uintptr) uintptr {
	switch message {
	case WM_COMMAND:
		id := int32(wParam)
		if id == -1 {
			break
		}
		switch uint32(id) {
		case menuIDShow:
			if onShow != nil {
				onShow()
			}
		case menuIDQuit:
			// 退出动作由 onQuit（service.go 中调用 tray.Quit）统一处理，
			// 此处不再重复 quit()，避免二次投递 WM_CLOSE。
			if onQuit != nil {
				onQuit()
			}
		}
	case WM_CLOSE:
		pDestroyWindow.Call(uintptr(t.window))
	case WM_DESTROY:
		// 移除托盘图标并退出消息循环
		t.muNID.Lock()
		if t.nid != nil {
			_ = t.nid.delete()
		}
		t.muNID.Unlock()
		pPostQuitMessage.Call(0)
	case t.wmTrayIcon:
		switch lParam {
		case WM_LBUTTONUP:
			// 左键单击：打开主界面
			if onLeftClick != nil {
				onLeftClick()
			}
		case WM_RBUTTONUP:
			// 右键单击：弹出功能菜单
			t.showMenu()
		}
	case t.wmTaskbarCreated:
		// explorer.exe 重启后重新添加托盘图标
		t.muNID.Lock()
		if t.nid != nil {
			_ = t.nid.add()
		}
		t.muNID.Unlock()
	default:
		lResult, _, _ := pDefWindowProc.Call(
			uintptr(hWnd),
			uintptr(message),
			uintptr(wParam),
			uintptr(lParam),
		)
		return lResult
	}
	return 0
}

// initInstance 初始化窗口类与托盘图标
func (t *winTray) initInstance() error {
	const className = "AIProxyTrayClass"
	const NIF_MESSAGE_ = NIF_MESSAGE

	t.wmTrayIcon = WM_USER + 1

	taskbarName, _ := windows.UTF16PtrFromString("TaskbarCreated")
	res, _, _ := pRegisterWindowMsg.Call(uintptr(unsafe.Pointer(taskbarName)))
	t.wmTaskbarCreated = uint32(res)

	instanceHandle, _, err := pGetModuleHandle.Call(0)
	if instanceHandle == 0 {
		return err
	}
	t.instance = windows.Handle(instanceHandle)

	iconHandle, _, err := pLoadIcon.Call(0, uintptr(IDI_APPLICATION))
	if iconHandle == 0 {
		return err
	}
	t.icon = windows.Handle(iconHandle)

	cursorHandle, _, err := pLoadCursor.Call(0, uintptr(IDC_ARROW))
	if cursorHandle == 0 {
		return err
	}
	t.cursor = windows.Handle(cursorHandle)

	classNamePtr, err := windows.UTF16PtrFromString(className)
	if err != nil {
		return err
	}

	windowNamePtr, err := windows.UTF16PtrFromString("")
	if err != nil {
		return err
	}

	t.wcex = &wndClassEx{
		Style:      CS_HREDRAW | CS_VREDRAW,
		WndProc:    windows.NewCallback(t.wndProc),
		Instance:   t.instance,
		Icon:       t.icon,
		Cursor:     t.cursor,
		Background: windows.Handle(6), // COLOR_WINDOW + 1
		ClassName:  classNamePtr,
		IconSm:     t.icon,
	}
	if err := t.wcex.register(); err != nil {
		return err
	}

	windowHandle, _, err := pCreateWindowEx.Call(
		0,
		uintptr(unsafe.Pointer(classNamePtr)),
		uintptr(unsafe.Pointer(windowNamePtr)),
		uintptr(WS_OVERLAPPEDWINDOW),
		uintptr(CW_USEDEFAULT), uintptr(CW_USEDEFAULT),
		uintptr(CW_USEDEFAULT), uintptr(CW_USEDEFAULT),
		0, 0, uintptr(t.instance), 0,
	)
	if windowHandle == 0 {
		return err
	}
	t.window = windows.Handle(windowHandle)

	pShowWindow.Call(uintptr(t.window), uintptr(SW_HIDE))
	pUpdateWindow.Call(uintptr(t.window))

	t.nid = &notifyIconData{
		Wnd:             t.window,
		ID:              100,
		Flags:           NIF_MESSAGE_,
		CallbackMessage: t.wmTrayIcon,
	}
	t.nid.Size = uint32(unsafe.Sizeof(*t.nid))
	return t.nid.add()
}

// setIcon 从文件加载图标并更新托盘
func (t *winTray) setIcon(filePath string) error {
	h, err := t.loadIcon(filePath)
	if err != nil {
		return err
	}
	t.muNID.Lock()
	defer t.muNID.Unlock()
	t.nid.Icon = h
	t.nid.Flags |= NIF_ICON
	t.nid.Size = uint32(unsafe.Sizeof(*t.nid))
	return t.nid.modify()
}

// setTooltip 更新悬停提示
func (t *winTray) setTooltip(tip string) error {
	b, err := windows.UTF16FromString(tip)
	if err != nil {
		return err
	}
	t.muNID.Lock()
	defer t.muNID.Unlock()
	copy(t.nid.Tip[:], b[:])
	t.nid.Flags |= NIF_TIP
	t.nid.Size = uint32(unsafe.Sizeof(*t.nid))
	return t.nid.modify()
}

// loadIcon 通过 LoadImageW 加载 .ico 文件
func (t *winTray) loadIcon(filePath string) (windows.Handle, error) {
	pathPtr, err := windows.UTF16PtrFromString(filePath)
	if err != nil {
		return 0, err
	}
	res, _, err := pLoadImage.Call(
		0,
		uintptr(unsafe.Pointer(pathPtr)),
		uintptr(IMAGE_ICON),
		0, 0,
		uintptr(LR_LOADFROMFILE|LR_DEFAULTSIZE),
	)
	if res == 0 {
		return 0, err
	}
	return windows.Handle(res), nil
}

// createMenu 创建弹出菜单
func (t *winTray) createMenu() error {
	menu, _, err := pCreatePopupMenu.Call()
	if menu == 0 {
		return err
	}
	t.menu = windows.Handle(menu)
	return nil
}

// addMenuItem 向主菜单追加菜单项（按 ID 插入）
func (t *winTray) addMenuItem(id uint32, title string) error {
	titlePtr, err := windows.UTF16PtrFromString(title)
	if err != nil {
		return err
	}
	mi := menuItemInfo{
		Mask:     MIIM_FTYPE | MIIM_STRING | MIIM_ID,
		Type:     MFT_STRING,
		ID:       id,
		TypeData: titlePtr,
		Cch:      uint32(len(title)),
	}
	mi.Size = uint32(unsafe.Sizeof(mi))
	res, _, err := pInsertMenuItem.Call(
		uintptr(t.menu),
		uintptr(id),
		0, // fByPosition = FALSE，按 ID 插入
		uintptr(unsafe.Pointer(&mi)),
	)
	if res == 0 {
		return err
	}
	return nil
}

// showMenu 在当前鼠标位置弹出功能菜单
func (t *winTray) showMenu() error {
	p := point{}
	res, _, err := pGetCursorPos.Call(uintptr(unsafe.Pointer(&p)))
	if res == 0 {
		return err
	}
	pSetForegroundWindow.Call(uintptr(t.window))
	res, _, err = pTrackPopupMenu.Call(
		uintptr(t.menu),
		uintptr(TPM_BOTTOMALIGN|TPM_LEFTALIGN),
		uintptr(p.X),
		uintptr(p.Y),
		0,
		uintptr(t.window),
		0,
	)
	if res == 0 {
		return err
	}
	return nil
}

// iconBytesToFilePath 将 ICO 字节缓存为临时文件（LoadImageW 需要文件路径）
func iconBytesToFilePath(iconBytes []byte) (string, error) {
	bh := md5.Sum(iconBytes)
	dataHash := hex.EncodeToString(bh[:])
	iconFilePath := filepath.Join(os.TempDir(), "aiproxy_tray_icon_"+dataHash)

	if _, err := os.Stat(iconFilePath); os.IsNotExist(err) {
		if err := ioutil.WriteFile(iconFilePath, iconBytes, 0644); err != nil {
			return "", err
		}
	}
	return iconFilePath, nil
}

// run 启动托盘并进入消息循环（阻塞直到 quit 被调用）
func run(opts Options) {
	// Windows 消息循环要求创建窗口与处理消息在同一 OS 线程，
	// 锁定当前 goroutine 的线程，避免调度器迁移导致消息收不到。
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	onLeftClick = opts.LeftClick
	onShow = opts.Show
	onQuit = opts.Quit

	if err := wt.initInstance(); err != nil {
		return
	}
	defer func() {
		if wt.wcex != nil {
			_ = wt.wcex.unregister()
		}
	}()

	// opts.Icon 为 PNG 字节（来自 assets/aiproxy.png）；LoadImageW 仅支持 .ico 文件，
	// 因此先在内存中转换为多尺寸 ICO，再缓存为临时文件加载。
	// 转换/加载失败均不致命，托盘回退系统默认图标。
	if len(opts.Icon) > 0 {
		if icoBytes, err := pngToICO(opts.Icon); err == nil {
			if p, err := iconBytesToFilePath(icoBytes); err == nil {
				_ = wt.setIcon(p)
			}
		}
	}
	if opts.Tooltip != "" {
		_ = wt.setTooltip(opts.Tooltip)
	}

	if err := wt.createMenu(); err == nil {
		_ = wt.addMenuItem(menuIDShow, "显示主窗口")
		_ = wt.addMenuItem(menuIDQuit, "退出")
	}

	// 主消息循环
	m := &msg{}
	for {
		ret, _, _ := pGetMessage.Call(uintptr(unsafe.Pointer(m)), 0, 0, 0)
		switch int32(ret) {
		case -1:
			return
		case 0:
			// WM_QUIT
			return
		default:
			pTranslateMessage.Call(uintptr(unsafe.Pointer(m)))
			pDispatchMessage.Call(uintptr(unsafe.Pointer(m)))
		}
	}
}

// quit 通过投递 WM_CLOSE 请求退出托盘消息循环
func quit() {
	pPostMessage.Call(uintptr(wt.window), uintptr(WM_CLOSE), 0, 0)
}
