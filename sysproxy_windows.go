//go:build windows

package main

import (
	"syscall"
	"unsafe"
)

// readSystemProxySetting 读当前用户的系统代理（“设置 → 网络和 Internet → 代理”里手动设置的那一项）。
// 没开代理时返回空。自动配置脚本（PAC）不支持。
func readSystemProxySetting() string {
	path, _ := syscall.UTF16PtrFromString(`Software\Microsoft\Windows\CurrentVersion\Internet Settings`)
	var h syscall.Handle
	if syscall.RegOpenKeyEx(syscall.HKEY_CURRENT_USER, path, 0, syscall.KEY_READ, &h) != nil {
		return ""
	}
	defer syscall.RegCloseKey(h)
	if regDWORD(h, "ProxyEnable") != 1 {
		return ""
	}
	return regString(h, "ProxyServer")
}

func regDWORD(h syscall.Handle, name string) uint32 {
	n, _ := syscall.UTF16PtrFromString(name)
	var typ, val uint32
	size := uint32(4)
	if syscall.RegQueryValueEx(h, n, nil, &typ, (*byte)(unsafe.Pointer(&val)), &size) != nil || typ != syscall.REG_DWORD {
		return 0
	}
	return val
}

func regString(h syscall.Handle, name string) string {
	n, _ := syscall.UTF16PtrFromString(name)
	var typ, size uint32
	if syscall.RegQueryValueEx(h, n, nil, &typ, nil, &size) != nil || size == 0 || (typ != syscall.REG_SZ && typ != syscall.REG_EXPAND_SZ) {
		return ""
	}
	buf := make([]uint16, size/2+1)
	if syscall.RegQueryValueEx(h, n, nil, &typ, (*byte)(unsafe.Pointer(&buf[0])), &size) != nil {
		return ""
	}
	return syscall.UTF16ToString(buf)
}
