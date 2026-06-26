package main

import (
	"syscall"
	"unsafe"
)

// Windows API：剪贴板与按键状态（用于右键粘贴）
var (
	user32           = syscall.NewLazyDLL("user32.dll")
	getAsyncKeyState = user32.NewProc("GetAsyncKeyState")
	openClipboard    = user32.NewProc("OpenClipboard")
	closeClipboard   = user32.NewProc("CloseClipboard")
	getClipboardData = user32.NewProc("GetClipboardData")
	globalLock       = user32.NewProc("GlobalLock")
	globalUnlock     = user32.NewProc("GlobalUnlock")
	globalSize       = user32.NewProc("GlobalSize")
	rtlMoveMemory    = user32.NewProc("RtlMoveMemory")
)

// Windows 消息/虚拟键常量
const (
	WM_RBUTTONDOWN = 0x0204
	WM_RBUTTONUP   = 0x0205
	VK_CONTROL     = 0x11
	CF_TEXT        = 1
)

// 获取剪贴板内容
func getClipboardText() string {
	// 打开剪贴板
	ret, _, _ := openClipboard.Call(0)
	if ret == 0 {
		return ""
	}
	defer closeClipboard.Call()

	// 获取剪贴板数据
	h, _, _ := getClipboardData.Call(uintptr(CF_TEXT))
	if h == 0 {
		return ""
	}

	// 获取数据大小
	size, _, _ := globalSize.Call(h)
	if size == 0 {
		return ""
	}

	// 锁定内存
	l, _, _ := globalLock.Call(h)
	if l == 0 {
		return ""
	}
	defer globalUnlock.Call(h)

	// 使用 RtlMoveMemory 来复制内存
	data := make([]byte, size)
	rtlMoveMemory.Call(
		uintptr(unsafe.Pointer(&data[0])),
		l,
		size,
	)

	// 找到第一个 null 字节
	for i, b := range data {
		if b == 0 {
			return string(data[:i])
		}
	}

	return string(data)
}
