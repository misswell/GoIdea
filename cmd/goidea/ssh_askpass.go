package main

import (
	"fmt"
	"os"
	"strings"
)

// handleSSHAskpass is the tiny child mode OpenSSH starts for a password or
// encrypted-key prompt. The secret stays in this process environment and is
// written only to OpenSSH's askpass pipe, never to the app log.
func handleSSHAskpass() bool {
	if os.Getenv("GOIDEA_SSH_ASKPASS") != "1" {
		return false
	}
	secret := strings.TrimSuffix(os.Getenv("GOIDEA_SSH_ASKPASS_SECRET"), "\r\n")
	_, _ = fmt.Fprint(os.Stdout, secret)
	return true
}
