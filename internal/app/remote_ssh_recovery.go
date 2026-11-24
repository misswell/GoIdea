package app

import (
	"regexp"
	"strings"
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
