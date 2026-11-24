package openidea

import (
	"encoding/json"
	"testing"
)

func TestDecodeDeploymentSettingsPreservesServersAndNormalizesIdentity(t *testing.T) {
	raw := json.RawMessage(`{
		"deployment": {
			"servers": [
				{"kind":"ftps","id":"","name":"Web","sshConfig":"ssh","ftp":{"host":"files.example","user":"web","passive":true},"rootPath":"/srv/app","webUrl":"https://example","mappings":[{"local":"","deployment":"","web":""}]},
				{"kind":"unknown","id":"dup","name":"Other"},
				{"id":"dup","name":"Duplicate"}
			],
			"defaultServer":"missing",
			"autoUpload":"always",
			"exclude":[".git;*.tmp"]
		}
	}`)
	settings, err := DecodeDeploymentSettings(raw)
	if err != nil {
		t.Fatal(err)
	}
	if len(settings.Servers) != 3 {
		t.Fatalf("servers = %d, want 3", len(settings.Servers))
	}
	if settings.Servers[0].ID != "server1" || settings.Servers[0].Kind != DeploymentFTPS {
		t.Fatalf("first server = %+v", settings.Servers[0])
	}
	if settings.Servers[1].ID != "dup" || settings.Servers[1].Kind != DeploymentSFTP {
		t.Fatalf("second server = %+v", settings.Servers[1])
	}
	if settings.Servers[2].ID == "dup" {
		t.Fatalf("duplicate id was not normalized: %+v", settings.Servers[2])
	}
	if settings.DefaultServer != "server1" {
		t.Fatalf("default server = %q, want server1", settings.DefaultServer)
	}
	if settings.AutoUpload != AutoUploadAlways {
		t.Fatalf("auto upload = %q", settings.AutoUpload)
	}
}

func TestDeploymentConnectionsMatchPinnedRpcTaggedUnion(t *testing.T) {
	ssh := DeploymentServer{Kind: DeploymentSFTP, SSHConfig: "build"}.Connection()
	if ssh.Kind != FsConnectionSFTP || ssh.SSH != "build" {
		t.Fatalf("sftp connection = %+v", ssh)
	}
	local := DeploymentServer{Kind: DeploymentLocal}.Connection()
	if local.Kind != FsConnectionLocal {
		t.Fatalf("local connection = %+v", local)
	}
	ftp := FTPConnection{Host: "files", User: "web", Passive: true}
	ftps := DeploymentServer{Kind: DeploymentFTPS, ID: "site", FTP: ftp}.Connection()
	if ftps.Kind != FsConnectionFTP || !ftps.Secure || ftps.Server != "site" || ftps.FTP == nil ||
		ftps.FTP.Host != "files" {
		t.Fatalf("ftps connection = %+v", ftps)
	}
}

func TestDecodeRunTargetSettingsKeepsLegacyAndContainerFields(t *testing.T) {
	raw := json.RawMessage(`{
		"runTargets":[
			{"id":"ssh","name":"Build box","sshConfig":"build","projectRoot":"/srv/project","syncBeforeRun":true},
			{"kind":"compose","id":"web","name":"Web","dockerConnection":"local","composeFiles":["compose.dev.yaml"],"service":"web","ports":["8000:8000"]}
		],
		"run":{"defaultTarget":"web"}
	}`)
	settings, err := DecodeRunTargetSettings(raw)
	if err != nil {
		t.Fatal(err)
	}
	if len(settings.Targets) != 2 || settings.Targets[0].Kind != RunTargetSSH {
		t.Fatalf("targets = %+v", settings.Targets)
	}
	if settings.Targets[1].Kind != RunTargetCompose || settings.Targets[1].Service != "web" ||
		len(settings.Targets[1].ComposeFiles) != 1 {
		t.Fatalf("compose target = %+v", settings.Targets[1])
	}
	if settings.DefaultTarget != "web" {
		t.Fatalf("default target = %q", settings.DefaultTarget)
	}
}
