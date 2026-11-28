package app

import (
	"context"
	"encoding/json"
	"testing"

	"mayoi/internal/openidea"
)

func TestRemoteSSHTestUsesCurrentConfigurationAndDoesNotSaveSecret(t *testing.T) {
	state := remoteDevelopmentState{
		sshEditID: "saved-box", sshHost: "box.example", sshAuth: "password",
	}
	config := remoteConfigFromState(&state)
	if config.ID != "saved-box" {
		t.Fatalf("test configuration ID = %q, want saved-box", config.ID)
	}
	calls := 0
	m := &Model{invokeHook: func(_ context.Context, command string, args any) (json.RawMessage, error) {
		calls++
		if command != "ssh_test" {
			t.Fatalf("unexpected command %q", command)
		}
		values := args.(map[string]any)
		if values["config"].(openidea.SSHConfiguration).ID != "saved-box" || values["secret"] != "password" {
			t.Fatalf("unexpected test arguments: %+v", values)
		}
		return json.RawMessage(`"connected"`), nil
	}}
	message, err := m.testRemoteSSHConnection(context.Background(), config, "password")
	if err != nil || message != "connected" || calls != 1 {
		t.Fatalf("test result = %q, %v; calls = %d", message, err, calls)
	}
}

func TestRemoteSSHTestResultDoesNotMatchInvalidPort(t *testing.T) {
	m := &Model{}
	m.remote.sshEditID = "box"
	m.remote.sshHost = "box.example"
	m.remote.sshAuth = "openSsh"
	m.remote.sshTestConfig = remoteConfigFromState(&m.remote)
	if !m.remoteSSHTestMatchesForm() {
		t.Fatal("unchanged form should match")
	}
	m.remote.sshPort = "invalid"
	if m.remoteSSHTestMatchesForm() {
		t.Fatal("invalid port must not match a test with the default port")
	}
}

func TestRemoteSSHTestPassesOpenSSHKeyPassphrase(t *testing.T) {
	m := &Model{invokeHook: func(_ context.Context, command string, args any) (json.RawMessage, error) {
		if command != "ssh_test" || args.(map[string]any)["secret"] != "  " {
			t.Fatalf("unexpected SSH test: %s %+v", command, args)
		}
		return json.RawMessage(`"connected"`), nil
	}}
	_, err := m.testRemoteSSHConnection(context.Background(), openidea.SSHConfiguration{
		ID: "box", Host: "box.example", Auth: "openSsh",
	}, "  ")
	if err != nil {
		t.Fatal(err)
	}
}
