package app

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"mayoi/internal/openidea"
)

func (m *Model) testRemoteSSHConnection(
	ctx context.Context,
	config openidea.SSHConfiguration,
	secret string,
) (string, error) {
	if strings.TrimSpace(config.ID) == "" {
		return "", fmt.Errorf("The SSH configuration has no id.")
	}
	var testSecret any
	if secret != "" {
		testSecret = secret
	}
	raw, err := m.invoke(ctx, "ssh_test", map[string]any{"config": config, "secret": testSecret})
	if err != nil {
		return "", err
	}
	if len(raw) == 0 || string(raw) == "null" {
		return "", nil
	}
	var message string
	if err := json.Unmarshal(raw, &message); err != nil {
		return "", fmt.Errorf("read the SSH test response: %w", err)
	}
	return message, nil
}

func (m *Model) setRemoteSSHSecret(
	ctx context.Context,
	configID string,
	secret string,
	save bool,
) error {
	configID = strings.TrimSpace(configID)
	if configID == "" {
		return fmt.Errorf("The SSH configuration has no id.")
	}
	_, err := m.invoke(ctx, "ssh_set_secret", map[string]any{
		"id": configID, "secret": secret, "save": save,
	})
	return err
}

func (m *Model) trustRemoteSSHHost(
	ctx context.Context,
	configID string,
	fingerprint string,
) error {
	configID = strings.TrimSpace(configID)
	fingerprint = strings.TrimSpace(fingerprint)
	if configID == "" {
		return fmt.Errorf("The SSH configuration has no id.")
	}
	if fingerprint == "" {
		return fmt.Errorf("The SSH host key fingerprint is unavailable. Test the connection again.")
	}
	_, err := m.invoke(ctx, "ssh_trust_host", map[string]any{
		"id": configID, "fingerprint": fingerprint,
	})
	return err
}

func remoteSSHConfigsMatch(left, right openidea.SSHConfiguration) bool {
	if left.ID != right.ID || left.Host != right.Host || left.User != right.User ||
		left.Auth != right.Auth || left.KeyFile != right.KeyFile {
		return false
	}
	if left.Port == nil || right.Port == nil {
		return left.Port == nil && right.Port == nil
	}
	return *left.Port == *right.Port
}

func newRemoteSSHConfigurationID(configs []openidea.SSHConfiguration) string {
	for suffix := int64(0); ; suffix++ {
		id := "ssh-" + strconv.FormatInt(time.Now().UnixMilli()+suffix, 10)
		found := false
		for _, config := range configs {
			if config.ID == id {
				found = true
				break
			}
		}
		if !found {
			return id
		}
	}
}

func (m *Model) resetRemoteSSHTest(clearSecret bool) {
	state := &m.remote
	state.sshSettingsRequest++
	state.sshSettingsTesting = false
	state.sshTestResult = ""
	state.sshTestErr = ""
	state.sshTestRemedy = ""
	state.sshTestFingerprint = ""
	state.sshTestConfig = openidea.SSHConfiguration{}
	if clearSecret {
		state.sshSecret = ""
	}
}

func (m *Model) remoteSSHTestMatchesForm() bool {
	state := &m.remote
	current := remoteConfigFromState(state)
	if strings.TrimSpace(state.sshPort) != "" && current.Port == nil {
		return false
	}
	current.ID = state.sshEditID
	return remoteSSHConfigsMatch(current, state.sshTestConfig)
}

func (m *Model) testRemoteSSHConfiguration() {
	state := &m.remote
	if state.sshSettingsTesting || state.sshSettingsBusy {
		return
	}
	config := remoteConfigFromState(state)
	if strings.TrimSpace(config.ID) == "" {
		config.ID = newRemoteSSHConfigurationID(state.sshConfigs)
		state.sshEditID = config.ID
	}
	if strings.TrimSpace(config.Host) == "" {
		state.sshTestResult = ""
		state.sshTestErr = "Host is required."
		state.sshTestRemedy = ""
		state.sshTestFingerprint = ""
		state.sshTestConfig = config
		m.invalidate()
		return
	}
	if portText := strings.TrimSpace(state.sshPort); portText != "" && config.Port == nil {
		state.sshTestResult = ""
		state.sshTestErr = "Port must be between 1 and 65535."
		state.sshTestRemedy = ""
		state.sshTestFingerprint = ""
		state.sshTestConfig = config
		m.invalidate()
		return
	}

	state.sshSettingsRequest++
	request := state.sshSettingsRequest
	generation := state.generation
	secret := state.sshSecret
	state.sshSettingsTesting = true
	state.sshTestResult = ""
	state.sshTestErr = ""
	state.sshTestRemedy = ""
	state.sshTestFingerprint = ""
	state.sshTestConfig = config
	m.invalidate()
	go func() {
		ctx, cancel := context.WithTimeout(m.remoteBaseContext(), 30*time.Second)
		defer cancel()
		message, err := m.testRemoteSSHConnection(ctx, config, secret)
		m.post(func() {
			if state.generation != generation || state.sshSettingsRequest != request {
				return
			}
			state.sshSettingsTesting = false
			if err != nil {
				state.sshTestErr = err.Error()
				state.sshTestRemedy = remoteSSHRemedyForError(err, config.Auth)
				state.sshTestFingerprint = sshHostKeyFingerprint(err.Error())
				if state.sshTestRemedy == remoteSSHRemedyTrustHost &&
					state.sshTestFingerprint == "" {
					state.sshTestErr += " The host key fingerprint is unavailable; test again."
				}
				m.invalidate()
				return
			}
			if message == "" {
				message = "Connected to " + config.Label()
			}
			state.sshTestResult = message
			state.sshTestErr = ""
			state.sshTestRemedy = ""
			state.sshTestFingerprint = ""
			m.invalidate()
		})
	}()
}

func (m *Model) trustRemoteSSHConfigurationHost() {
	state := &m.remote
	if state.sshSettingsTesting || state.sshSettingsBusy ||
		state.sshTestRemedy != remoteSSHRemedyTrustHost ||
		!m.remoteSSHTestMatchesForm() {
		return
	}
	config := remoteConfigFromState(state)
	config.ID = state.sshEditID
	fingerprint := state.sshTestFingerprint
	if strings.TrimSpace(fingerprint) == "" {
		state.sshTestErr = "The SSH host key fingerprint is unavailable. Test the connection again."
		m.invalidate()
		return
	}
	state.sshSettingsRequest++
	request := state.sshSettingsRequest
	generation := state.generation
	state.sshSettingsTesting = true
	state.sshTestErr = ""
	m.invalidate()
	go func() {
		ctx, cancel := context.WithTimeout(m.remoteBaseContext(), 30*time.Second)
		defer cancel()
		err := m.trustRemoteSSHHost(ctx, config.ID, fingerprint)
		m.post(func() {
			if state.generation != generation || state.sshSettingsRequest != request {
				return
			}
			state.sshSettingsTesting = false
			if err != nil {
				state.sshTestErr = "Could not trust the SSH host: " + err.Error()
				state.sshTestRemedy = ""
				m.invalidate()
				return
			}
			state.sshTestRemedy = ""
			state.sshTestFingerprint = ""
			m.testRemoteSSHConfiguration()
		})
	}()
}

func (m *Model) retryRemoteSSHConfigurationSecret() {
	state := &m.remote
	if state.sshSettingsTesting || state.sshSettingsBusy ||
		state.sshTestRemedy != remoteSSHRemedySecret ||
		!m.remoteSSHTestMatchesForm() {
		return
	}
	if state.sshSecret == "" {
		state.sshTestErr = "Enter the password or key passphrase."
		m.invalidate()
		return
	}
	m.testRemoteSSHConfiguration()
}
