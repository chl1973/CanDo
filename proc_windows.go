//go:build windows

package main

import (
	"os/exec"
	"strconv"
	"syscall"
)

// hideConsole 让子进程不弹出黑色命令行窗口（工作台本身是窗口程序）。
func hideConsole(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: 0x08000000} // CREATE_NO_WINDOW
}

// killTree 结束进程及其子进程。
func killTree(cmd *exec.Cmd) {
	if cmd.Process == nil {
		return
	}
	k := exec.Command("taskkill", "/T", "/F", "/PID", strconv.Itoa(cmd.Process.Pid))
	hideConsole(k)
	k.Run()
	cmd.Process.Kill()
}

// shellCommand 用 PowerShell 执行一条命令，输出统一为 UTF-8。
func shellCommand(command string) *exec.Cmd {
	pre := "[Console]::OutputEncoding=[System.Text.Encoding]::UTF8;$OutputEncoding=[System.Text.Encoding]::UTF8;$ProgressPreference='SilentlyContinue';"
	return exec.Command("powershell", "-NoProfile", "-NonInteractive", "-ExecutionPolicy", "Bypass", "-Command", pre+command)
}

const shellName = "PowerShell"

// openWithSystem 用系统默认程序打开文件或网址。
func openWithSystem(target string) error {
	cmd := exec.Command("rundll32", "url.dll,FileProtocolHandler", target)
	hideConsole(cmd)
	return cmd.Start()
}
