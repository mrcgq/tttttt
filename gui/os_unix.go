//go:build !windows

package main

import "os/exec"

// hideWindow 在 Unix 系统下无需处理（默认不弹窗）
func hideWindow(cmd *exec.Cmd) {
}
