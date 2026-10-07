//go:build !windows

package main

// 其他系统只看环境变量（HTTPS_PROXY 等）。
func readSystemProxySetting() string { return "" }
