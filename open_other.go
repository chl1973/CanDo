//go:build !windows

package main

import (
	"fmt"
	"os"
	"os/exec"
	"runtime"
)

func openWindow(url string) {
	if runtime.GOOS == "darwin" {
		exec.Command("open", url).Start()
		return
	}
	exec.Command("xdg-open", url).Start()
}

func showError(msg string) { fmt.Fprintln(os.Stderr, msg) }
func showInfo(msg string)  { fmt.Println(msg) }
