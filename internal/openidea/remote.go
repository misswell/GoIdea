package openidea

import (
	"encoding/json"
	"fmt"
	"strings"
)

// RemoteTargetKind is where a remote project's openidea-server runs.
type RemoteTargetKind string

const (
	RemoteTargetSSH          RemoteTargetKind = "ssh"
	RemoteTargetWSL          RemoteTargetKind = "wsl"
	RemoteTargetDevContainer RemoteTargetKind = "devContainer"
)

// RemoteTarget matches OpenIDEA's tagged RemoteTarget shape.
type RemoteTarget struct {
	Kind   RemoteTargetKind `json:"kind"`
	Config string           `json:"config,omitempty"`
	Distro string           `json:"distro,omitempty"`
	Folder string           `json:"folder,omitempty"`
}

func (t RemoteTarget) Validate() error {
	switch t.Kind {
	case RemoteTargetSSH:
		if strings.TrimSpace(t.Config) == "" {
			return fmt.Errorf("choose an SSH configuration")
		}
	case RemoteTargetWSL:
		if !validWSLDistro(t.Distro) {
			return fmt.Errorf("'%s' is not a valid WSL distribution name", t.Distro)
		}
	case RemoteTargetDevContainer:
		if strings.TrimSpace(t.Folder) == "" {
			return fmt.Errorf("the dev-container folder is empty")
		}
	default:
		return fmt.Errorf("unknown remote target %q", t.Kind)
	}
	return nil
}

func (t RemoteTarget) Key() string {
	switch t.Kind {
	case RemoteTargetSSH:
		return "ssh:" + t.Config
	case RemoteTargetWSL:
		return "wsl:" + t.Distro
	case RemoteTargetDevContainer:
		return "devcontainer:" + t.Folder + ":" + t.Config
	default:
		return string(t.Kind)
	}
}

func (t RemoteTarget) Label() string {
	switch t.Kind {
	case RemoteTargetSSH:
		return t.Config
	case RemoteTargetWSL:
		return t.Distro
	case RemoteTargetDevContainer:
		parts := strings.FieldsFunc(t.Folder, func(r rune) bool { return r == '/' || r == '\\' })
		if len(parts) == 0 {
			return t.Folder
		}
		return parts[len(parts)-1]
	default:
		return string(t.Kind)
	}
}

// SSHConfiguration is Settings › Tools › SSH Configurations.
type SSHConfiguration struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	Host       string `json:"host"`
	Port       *int   `json:"port"`
	User       string `json:"user"`
	Auth       string `json:"auth"`
	KeyFile    string `json:"keyFile"`
	SaveSecret bool   `json:"saveSecret"`
}

func (c SSHConfiguration) Label() string {
	host := strings.TrimSpace(c.Host)
	if host == "" {
		return strings.TrimSpace(c.Name)
	}
	out := host
	if user := strings.TrimSpace(c.User); user != "" && c.Auth != "openSsh" {
		out = user + "@" + out
	}
	if c.Port != nil && *c.Port > 0 {
		out = fmt.Sprintf("%s:%d", out, *c.Port)
	}
	return out
}

type remoteSettingsEnvelope struct {
	SSH struct {
		Configs []SSHConfiguration `json:"configs"`
	} `json:"ssh"`
}

// DecodeSSHConfigurations reads settings_get_global's ssh.configs. Duplicate
// or missing ids are normalized exactly as the pinned frontend does.
func DecodeSSHConfigurations(raw json.RawMessage) ([]SSHConfiguration, error) {
	var settings remoteSettingsEnvelope
	if err := json.Unmarshal(raw, &settings); err != nil {
		return nil, err
	}
	seen := map[string]bool{}
	out := make([]SSHConfiguration, 0, len(settings.SSH.Configs))
	for i, config := range settings.SSH.Configs {
		config.ID = strings.TrimSpace(config.ID)
		if config.ID == "" {
			config.ID = fmt.Sprintf("ssh%d", i+1)
		}
		for seen[config.ID] {
			config.ID += "_"
		}
		seen[config.ID] = true
		switch config.Auth {
		case "password", "keyPair", "openSsh":
		default:
			config.Auth = "openSsh"
		}
		if config.Port != nil && (*config.Port <= 0 || *config.Port > 65535) {
			config.Port = nil
		}
		out = append(out, config)
	}
	return out, nil
}

// RemoteStage is the progress state in File › Remote Development.
type RemoteStage string

const (
	RemoteStageConnecting  RemoteStage = "connecting"
	RemoteStageContainer   RemoteStage = "container"
	RemoteStageChecking    RemoteStage = "checking"
	RemoteStageDownloading RemoteStage = "downloading"
	RemoteStageUploading   RemoteStage = "uploading"
	RemoteStageStarting    RemoteStage = "starting"
	RemoteStageOpening     RemoteStage = "opening"
)

// RemoteProgress carries one stage and, for download/upload, its fraction.
type RemoteProgress struct {
	Stage    RemoteStage `json:"stage"`
	Fraction *float64    `json:"fraction,omitempty"`
}

// RemoteDirectoryEntry is one folder from remote_bootstrap_list.
type RemoteDirectoryEntry struct {
	Name  string `json:"name"`
	Path  string `json:"path"`
	IsDir bool   `json:"isDir"`
}

// ForwardedPort is a remote port reachable on 127.0.0.1 locally.
type ForwardedPort struct {
	RemotePort uint16  `json:"remotePort"`
	LocalPort  uint16  `json:"localPort"`
	Host       *string `json:"host"`
}

// RemoteSession is the current connection shown in status and the Ports view.
type RemoteSession struct {
	Target        RemoteTarget `json:"target"`
	Root          string       `json:"root"`
	Host          string       `json:"host"`
	OS            string       `json:"os"`
	ServerVersion string       `json:"serverVersion"`
	Commands      []string     `json:"commands"`
}

func validWSLDistro(name string) bool {
	name = strings.TrimSpace(name)
	if name == "" || strings.HasPrefix(name, "-") {
		return false
	}
	for _, r := range name {
		if r == '/' || r == '\\' || r == ':' || r == ';' || r == '"' || r == '\'' ||
			r == '\n' || r == '\r' || r == '\t' || r == 0 {
			return false
		}
	}
	return true
}
