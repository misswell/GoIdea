//go:build windows

package app

import "fmt"

func startUpdateInstaller(script, logPath string) error {
	return fmt.Errorf("online install is unsupported on Windows")
}
