package app

import (
	"regexp"
	"strings"
)

type remoteSSHRemedy string

const (
	remoteSSHRemedyTrustHost remoteSSHRemedy = "trustHostKey"
	remoteSSHRemedySecret    remoteSSHRemedy = "secret"
)

var sshFingerprintPattern = regexp.MustCompile(
	`\(\s*((?:ssh-)?[A-Za-z0-9._-]+ SHA256:[A-Za-z0-9+/=_-]+)\s*\)`,
)

// sshHostKeyFingerprint extracts the exact key shown by the backend's
// "Unknown SSH host key ... (algorithm SHA256:...)" error. Callers pass it
// back when trusting the host so a concurrent reconnect cannot swap keys.
func sshHostKeyFingerprint(message string) string {
	match := sshFingerprintPattern.FindStringSubmatch(message)
	if len(match) < 2 {
		return ""
	}
	return strings.TrimSpace(match[1])
}

func sshAuthenticationNeedsSecret(message string) bool {
	lower := strings.ToLower(message)
	return strings.Contains(lower, "password required") ||
		strings.Contains(lower, "key passphrase required") ||
		strings.Contains(lower, "cannot decrypt ssh key") ||
		strings.Contains(lower, "authentication failed")
}

func sshHostKeyNeedsTrust(message string) bool {
	lower := strings.ToLower(message)
	return strings.Contains(lower, "unknown ssh host key") ||
		strings.Contains(lower, "unknown host key")
}

// remoteSSHRemedyForError matches OpenIDEA's SSH Test Connection and Start SSH
// Session recovery decisions. A changed host key is never offered for trust,
// and an OpenSSH configuration cannot be repaired by typing a secret.
func remoteSSHRemedyForError(err error, auth string) remoteSSHRemedy {
	if err == nil {
		return ""
	}
	kind, message := dbErrorDetails(err)
	if strings.HasPrefix(message, "Unknown SSH host key") {
		return remoteSSHRemedyTrustHost
	}
	if kind != "" && kind != "permissionDenied" {
		return ""
	}
	if strings.HasPrefix(message, "SSH host key ") && strings.Contains(message, " has changed") {
		return ""
	}
	if strings.HasPrefix(message, "SSH password required") ||
		strings.HasPrefix(message, "SSH key passphrase required") ||
		strings.HasPrefix(message, "cannot decrypt SSH key") {
		return remoteSSHRemedySecret
	}
	if strings.HasPrefix(message, "SSH authentication failed") &&
		!strings.EqualFold(strings.TrimSpace(auth), "openSsh") {
		return remoteSSHRemedySecret
	}
	return ""
}
