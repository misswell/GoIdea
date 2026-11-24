package openidea

import (
	"encoding/json"
	"fmt"
	"strings"
)

// DeploymentType is the filesystem of a deployment server.
type DeploymentType string

const (
	DeploymentSFTP  DeploymentType = "sftp"
	DeploymentFTP   DeploymentType = "ftp"
	DeploymentFTPS  DeploymentType = "ftps"
	DeploymentLocal DeploymentType = "local"
)

// AutoUpload controls when a saved file is uploaded to the default server.
type AutoUpload string

const (
	AutoUploadNever        AutoUpload = "never"
	AutoUploadExplicitSave AutoUpload = "onExplicitSave"
	AutoUploadAlways       AutoUpload = "always"
)

// FTPConnection is an FTP or explicit-FTPS endpoint.
type FTPConnection struct {
	Host    string `json:"host"`
	Port    *int   `json:"port"`
	User    string `json:"user"`
	Passive bool   `json:"passive"`
}

// DeploymentMapping maps one project folder to a folder below the server root.
type DeploymentMapping struct {
	Local      string `json:"local"`
	Deployment string `json:"deployment"`
	Web        string `json:"web"`
}

// DeploymentServer is one Settings › Deployment › Servers entry.
type DeploymentServer struct {
	Kind          DeploymentType      `json:"kind"`
	ID            string              `json:"id"`
	Name          string              `json:"name"`
	SSHConfig     string              `json:"sshConfig"`
	FTP           FTPConnection       `json:"ftp"`
	RootPath      string              `json:"rootPath"`
	WebURL        string              `json:"webUrl"`
	Mappings      []DeploymentMapping `json:"mappings"`
	ExcludedPaths []string            `json:"excludedPaths"`
}

// Label is the name shown in menus and the Remote Host window.
func (s DeploymentServer) Label() string {
	if name := strings.TrimSpace(s.Name); name != "" {
		return name
	}
	return strings.TrimSpace(s.ID)
}

// DeploymentSettings is the project's deployment configuration.
type DeploymentSettings struct {
	Servers            []DeploymentServer `json:"servers"`
	DefaultServer      string             `json:"defaultServer"`
	AutoUpload         AutoUpload         `json:"autoUpload"`
	DeleteRemote       bool               `json:"deleteRemote"`
	PreserveTimestamps bool               `json:"preserveTimestamps"`
	Exclude            []string           `json:"exclude"`
}

func defaultDeploymentSettings() DeploymentSettings {
	return DeploymentSettings{
		AutoUpload:         AutoUploadNever,
		PreserveTimestamps: true,
		Exclude: []string{
			".git", ".svn", ".hg", ".idea", ".openidea", ".DS_Store", "*.pyc", "__pycache__",
		},
	}
}

type deploymentSettingsEnvelope struct {
	Deployment json.RawMessage `json:"deployment"`
}

// DecodeDeploymentSettings reads the deployment object from settings_get.
func DecodeDeploymentSettings(raw json.RawMessage) (DeploymentSettings, error) {
	var envelope deploymentSettingsEnvelope
	if err := json.Unmarshal(raw, &envelope); err != nil {
		return DeploymentSettings{}, err
	}
	settings := defaultDeploymentSettings()
	if len(envelope.Deployment) == 0 || string(envelope.Deployment) == "null" {
		return settings, nil
	}
	if err := json.Unmarshal(envelope.Deployment, &settings); err != nil {
		return DeploymentSettings{}, err
	}
	normalizeDeploymentSettings(&settings)
	return settings, nil
}

func normalizeDeploymentSettings(settings *DeploymentSettings) {
	if settings.AutoUpload != AutoUploadAlways && settings.AutoUpload != AutoUploadExplicitSave {
		settings.AutoUpload = AutoUploadNever
	}
	ids := map[string]bool{}
	for i := range settings.Servers {
		server := &settings.Servers[i]
		server.ID = strings.TrimSpace(server.ID)
		if server.ID == "" {
			server.ID = fmt.Sprintf("server%d", i+1)
		}
		for ids[server.ID] {
			server.ID += "_"
		}
		ids[server.ID] = true
		switch server.Kind {
		case DeploymentSFTP, DeploymentFTP, DeploymentFTPS, DeploymentLocal:
		default:
			server.Kind = DeploymentSFTP
		}
		if server.FTP.Port != nil && (*server.FTP.Port <= 0 || *server.FTP.Port > 65535) {
			server.FTP.Port = nil
		}
	}
	if settings.DefaultServer != "" && !ids[settings.DefaultServer] {
		settings.DefaultServer = ""
	}
	if settings.DefaultServer == "" && len(settings.Servers) > 0 {
		settings.DefaultServer = settings.Servers[0].ID
	}
}

// DeploymentOptions is the subset written by the native Deployment Options UI.
type DeploymentOptions struct {
	DefaultServer      string     `json:"defaultServer"`
	AutoUpload         AutoUpload `json:"autoUpload"`
	DeleteRemote       bool       `json:"deleteRemote"`
	PreserveTimestamps bool       `json:"preserveTimestamps"`
	Exclude            []string   `json:"exclude"`
}

// Options returns the options without the server list.
func (s DeploymentSettings) Options() DeploymentOptions {
	return DeploymentOptions{
		DefaultServer:      s.DefaultServer,
		AutoUpload:         s.AutoUpload,
		DeleteRemote:       s.DeleteRemote,
		PreserveTimestamps: s.PreserveTimestamps,
		Exclude:            append([]string(nil), s.Exclude...),
	}
}

// RunTargetKind is where a run configuration executes.
type RunTargetKind string

const (
	RunTargetSSH     RunTargetKind = "ssh"
	RunTargetDocker  RunTargetKind = "docker"
	RunTargetCompose RunTargetKind = "compose"
)

// RunTarget is one Settings › Run Targets entry.
type RunTarget struct {
	Kind             RunTargetKind `json:"kind"`
	ID               string        `json:"id"`
	Name             string        `json:"name"`
	SSHConfig        string        `json:"sshConfig"`
	ProjectRoot      string        `json:"projectRoot"`
	SyncBeforeRun    bool          `json:"syncBeforeRun"`
	Python           string        `json:"python"`
	Node             string        `json:"node"`
	Java             string        `json:"java"`
	DockerConnection string        `json:"dockerConnection"`
	Image            string        `json:"image"`
	ComposeFiles     []string      `json:"composeFiles"`
	Service          string        `json:"service"`
	ContainerRoot    string        `json:"containerRoot"`
	Ports            []string      `json:"ports"`
	RunOptions       string        `json:"runOptions"`
	ImageUser        bool          `json:"imageUser"`
}

// Label is the target's user-facing name.
func (t RunTarget) Label() string {
	if name := strings.TrimSpace(t.Name); name != "" {
		return name
	}
	return strings.TrimSpace(t.ID)
}

type runTargetSettingsEnvelope struct {
	RunTargets []RunTarget `json:"runTargets"`
	Run        struct {
		DefaultTarget string `json:"defaultTarget"`
	} `json:"run"`
}

// RunTargetSettings is the target list and the default target of new runs.
type RunTargetSettings struct {
	Targets       []RunTarget `json:"targets"`
	DefaultTarget string      `json:"defaultTarget"`
}

// DecodeRunTargetSettings reads runTargets and run.defaultTarget from settings_get.
func DecodeRunTargetSettings(raw json.RawMessage) (RunTargetSettings, error) {
	var envelope runTargetSettingsEnvelope
	if err := json.Unmarshal(raw, &envelope); err != nil {
		return RunTargetSettings{}, err
	}
	seen := map[string]bool{}
	out := RunTargetSettings{
		Targets:       append([]RunTarget(nil), envelope.RunTargets...),
		DefaultTarget: strings.TrimSpace(envelope.Run.DefaultTarget),
	}
	for i := range out.Targets {
		target := &out.Targets[i]
		target.ID = strings.TrimSpace(target.ID)
		if target.ID == "" {
			target.ID = fmt.Sprintf("target%d", i+1)
		}
		for seen[target.ID] {
			target.ID += "_"
		}
		seen[target.ID] = true
		switch target.Kind {
		case RunTargetSSH, RunTargetDocker, RunTargetCompose:
		default:
			target.Kind = RunTargetSSH
		}
	}
	return out, nil
}

// PluginRunTargetSpec is the argv wrapper a project plugin contributes to the
// Run on selector. The plugin host owns it and its id is opaque.
type PluginRunTargetSpec struct {
	ID            string   `json:"id"`
	Name          string   `json:"name"`
	Program       string   `json:"program"`
	Args          []string `json:"args"`
	WorkdirArgs   []string `json:"workdirArgs"`
	ContainerRoot *string  `json:"containerRoot"`
}

// RegisteredRunTarget is one project plugin's launchable Run Target.
type RegisteredRunTarget struct {
	ID         string              `json:"id"`
	PluginID   string              `json:"pluginId"`
	PluginName string              `json:"pluginName"`
	Target     PluginRunTargetSpec `json:"target"`
	Prompt     bool                `json:"prompt"`
}

// DecodeRegisteredRunTargets reads plugin_run_targets.
func DecodeRegisteredRunTargets(raw json.RawMessage) ([]RegisteredRunTarget, error) {
	if len(raw) == 0 || string(raw) == "null" {
		return nil, nil
	}
	var targets []RegisteredRunTarget
	if err := json.Unmarshal(raw, &targets); err != nil {
		return nil, err
	}
	seen := map[string]bool{}
	out := targets[:0]
	for _, target := range targets {
		target.ID = strings.TrimSpace(target.ID)
		target.PluginID = strings.TrimSpace(target.PluginID)
		target.PluginName = strings.TrimSpace(target.PluginName)
		target.Target.ID = strings.TrimSpace(target.Target.ID)
		target.Target.Name = strings.TrimSpace(target.Target.Name)
		if target.ID == "" || target.PluginID == "" || target.Target.ID == "" || seen[target.ID] {
			continue
		}
		seen[target.ID] = true
		out = append(out, target)
	}
	return out, nil
}

// FsConnectionKind identifies a deployment filesystem.
type FsConnectionKind string

const (
	FsConnectionSFTP  FsConnectionKind = "sftp"
	FsConnectionFTP   FsConnectionKind = "ftp"
	FsConnectionLocal FsConnectionKind = "local"
)

// FsConnection is the tagged connection accepted by deploy_* and remote_* RPCs.
type FsConnection struct {
	Kind   FsConnectionKind `json:"kind"`
	SSH    string           `json:"ssh,omitempty"`
	Server string           `json:"server,omitempty"`
	Secure bool             `json:"secure,omitempty"`
	FTP    *FTPConnection   `json:"ftp,omitempty"`
}

// Connection returns the deployment server's filesystem connection.
func (s DeploymentServer) Connection() FsConnection {
	switch s.Kind {
	case DeploymentFTP, DeploymentFTPS:
		ftp := s.FTP
		return FsConnection{
			Kind: FsConnectionFTP, Server: s.ID,
			Secure: s.Kind == DeploymentFTPS, FTP: &ftp,
		}
	case DeploymentLocal:
		return FsConnection{Kind: FsConnectionLocal}
	default:
		return FsConnection{Kind: FsConnectionSFTP, SSH: s.SSHConfig}
	}
}

// RemoteEntry is one file or folder on a deployment filesystem.
type RemoteEntry struct {
	Name        string  `json:"name"`
	Path        string  `json:"path"`
	IsDir       bool    `json:"isDir"`
	IsSymlink   bool    `json:"isSymlink"`
	Size        uint64  `json:"size"`
	MTime       *uint32 `json:"mtime"`
	Permissions string  `json:"permissions"`
}

// TransferError is one path a deployment operation could not handle.
type TransferError struct {
	Path    string `json:"path"`
	Message string `json:"message"`
}

// TransferReport is the result of an upload, download, delete or sync.
type TransferReport struct {
	Files   uint32          `json:"files"`
	Skipped uint32          `json:"skipped"`
	Errors  []TransferError `json:"errors"`
}

// DeploymentCoverage says which selected paths a server can deploy.
type DeploymentCoverage struct {
	Server  string          `json:"server"`
	Paths   []string        `json:"paths"`
	Skipped []TransferError `json:"skipped"`
}

// FileStamp compares one side of a deployment sync.
type FileStamp struct {
	Size  uint64  `json:"size"`
	MTime *uint32 `json:"mtime"`
}

// SyncStatus classifies one file in Sync with Deployed.
type SyncStatus string

const (
	SyncLocalOnly   SyncStatus = "localOnly"
	SyncRemoteOnly  SyncStatus = "remoteOnly"
	SyncLocalNewer  SyncStatus = "localNewer"
	SyncRemoteNewer SyncStatus = "remoteNewer"
	SyncDifferent   SyncStatus = "different"
	SyncSame        SyncStatus = "same"
)

// SyncEntry is one local/remote file pair.
type SyncEntry struct {
	Path       string     `json:"path"`
	RemotePath string     `json:"remotePath"`
	Local      *FileStamp `json:"local"`
	Remote     *FileStamp `json:"remote"`
	Status     SyncStatus `json:"status"`
}

// SyncAction is one choice in Sync with Deployed.
type SyncAction string

const (
	SyncUpload       SyncAction = "upload"
	SyncDownload     SyncAction = "download"
	SyncDeleteRemote SyncAction = "deleteRemote"
	SyncDeleteLocal  SyncAction = "deleteLocal"
)

// SyncStep applies an action to one project-relative path.
type SyncStep struct {
	Path   string     `json:"path"`
	Action SyncAction `json:"action"`
}

// DeploymentRoot is the resolved Remote Host root and its project mappings.
type DeploymentRoot struct {
	Root     string            `json:"root"`
	Mappings []ResolvedMapping `json:"mappings"`
}

// ResolvedMapping maps an absolute local folder to an absolute remote folder.
type ResolvedMapping struct {
	Local      string `json:"local"`
	Deployment string `json:"deployment"`
}

// DeployedPath is where a local project file lives on a deployment server.
type DeployedPath struct {
	Server     string  `json:"server"`
	RemotePath string  `json:"remotePath"`
	WebURL     *string `json:"webUrl"`
}
