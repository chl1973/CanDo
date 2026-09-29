//go:build !windows

package main

import (
	"os/exec"
	"sync"
	"syscall"
)

func hideConsole(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
}

func killTree(cmd *exec.Cmd) {
	if cmd.Process == nil {
		return
	}
	syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
	cmd.Process.Kill()
}

func shellCommand(command string) *exec.Cmd {
	return exec.Command("sh", "-c", command)
}

const shellName = "sh"

// openWithSystem 在测试环境中只记录，不真正打开。
var (
	openedMu      sync.Mutex
	openedTargets []string
)

func openWithSystem(target string) error {
	openedMu.Lock()
	openedTargets = append(openedTargets, target)
	openedMu.Unlock()
	return nil
}
