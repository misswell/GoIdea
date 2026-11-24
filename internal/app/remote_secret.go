package app

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"sync"
	"time"
)

const remoteSSHKeychainService = "com.goidea.remote.ssh"

type remoteSecretVault struct {
	mu     sync.Mutex
	values map[string]string
	runner remoteProcessRunner
}

func newRemoteSecretVault() *remoteSecretVault {
	return &remoteSecretVault{values: map[string]string{}, runner: defaultRemoteRunner}
}

func (v *remoteSecretVault) get(id string, saved bool) string {
	v.mu.Lock()
	secret := v.values[id]
	v.mu.Unlock()
	if secret != "" || !saved || runtime.GOOS != "darwin" {
		return secret
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	out, err := v.runner.command(ctx, "security",
		"find-generic-password", "-s", remoteSSHKeychainService, "-a", id, "-w").Output()
	if err != nil {
		return ""
	}
	secret = strings.TrimRight(string(out), "\r\n")
	if secret != "" {
		v.mu.Lock()
		v.values[id] = secret
		v.mu.Unlock()
	}
	return secret
}

func (v *remoteSecretVault) set(id, secret string, save bool) error {
	v.mu.Lock()
	if secret == "" {
		delete(v.values, id)
	} else {
		v.values[id] = secret
	}
	v.mu.Unlock()
	if !save || runtime.GOOS != "darwin" {
		return nil
	}
	if secret == "" {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		err := v.runner.command(ctx, "security",
			"delete-generic-password", "-s", remoteSSHKeychainService, "-a", id).Run()
		if err != nil {
			var exit *exec.ExitError
			if !errors.As(err, &exit) || exit.ExitCode() != 44 {
				return fmt.Errorf("remove the saved SSH secret: %w", err)
			}
		}
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	cmd := v.runner.command(ctx, "security",
		"add-generic-password", "-U", "-s", remoteSSHKeychainService, "-a", id, "-w", secret)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("save the SSH secret in Keychain: %w", err)
	}
	return nil
}

func adjustRemoteBackendSecret(backend *remoteBackend, vault *remoteSecretVault) {
	if backend == nil || backend.target.Kind != "ssh" || backend.ssh.Auth == "openSsh" {
		return
	}
	backend.secret = vault.get(backend.ssh.ID, backend.ssh.SaveSecret)
}

func sshAskpassProgram() string {
	exe, err := os.Executable()
	if err != nil {
		return ""
	}
	return exe
}
