//go:build windows

package main

import (
	"os/exec"
	"syscall"
	"unsafe"
)

// openWindow 用系统默认浏览器打开工作台。
// （1.0.0/1.0.1 曾优先使用 Edge 的应用窗口模式，部分电脑上会出现空白窗口，1.0.2 起改为默认浏览器。）
func openWindow(url string) {
	cmd := exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	if cmd.Start() == nil {
		return
	}
	exec.Command("cmd", "/c", "start", "", url).Start()
}

func messageBox(msg string, flags uintptr) {
	user32 := syscall.NewLazyDLL("user32.dll")
	box := user32.NewProc("MessageBoxW")
	t, _ := syscall.UTF16PtrFromString(msg)
	c, _ := syscall.UTF16PtrFromString("CanDo 可为")
	box.Call(0, uintptr(unsafe.Pointer(t)), uintptr(unsafe.Pointer(c)), flags)
}

func showError(msg string) { messageBox(msg, 0x10) }
func showInfo(msg string)  { messageBox(msg, 0x40) }
