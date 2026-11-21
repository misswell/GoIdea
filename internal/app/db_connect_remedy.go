package app

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"

	"mayoi/internal/openidea"
)

type dbConnectRemedy struct {
	open    bool
	kind    string
	prompt  string
	target  string
	source  string
	name    string
	user    string
	driver  string
	sshHost string
	sshUser string
	sshPort string
	sshAuth string
	secret  string
	save    bool
	busy    bool
	err     string
}

func dbErrorDetails(err error) (kind, message string) {
	var remote *openidea.Error
	if errors.As(err, &remote) {
		return remote.Kind, remote.Message
	}
	return "", err.Error()
}

func dbConnectRemedyKind(err error, source map[string]any) string {
	kind, message := dbErrorDetails(err)
	if strings.HasPrefix(message, "Unknown SSH host key") {
		return "trustHostKey"
	}
	if kind != "permissionDenied" {
		return ""
	}
	if strings.HasPrefix(message, "SSH host key ") && strings.Contains(message, " has changed") {
		return ""
	}
	if strings.HasPrefix(message, "SSH password required") ||
		strings.HasPrefix(message, "SSH key passphrase required") ||
		strings.HasPrefix(message, "cannot decrypt SSH key") {
		return "sshSecret"
	}
	if strings.HasPrefix(message, "SSH authentication failed") {
		ssh := dbMapObject(source, "ssh")
		if fmt.Sprint(ssh["auth"]) == "openSsh" {
			return ""
		}
		return "sshSecret"
	}
	return "password"
}

func (m *Model) dbSourceRawForID(ctx context.Context, id string) map[string]any {
	base := dbBaseTarget(id)
	sources, err := m.mgr.DBSourcesRaw(ctx, Window)
	if err != nil {
		return nil
	}
	for _, source := range sources {
		if fmt.Sprint(source["id"]) == base {
			return source
		}
	}
	return nil
}

func (m *Model) openDBConnectRemedy(target string, err error, raw map[string]any) {
	if m.db == nil || err == nil {
		return
	}
	remedy := &dbConnectRemedy{
		open: true, target: target, source: dbBaseTarget(target),
	}
	remedy.kind = dbConnectRemedyKind(err, raw)
	if remedy.kind == "" {
		return
	}
	_, remedy.prompt = dbErrorDetails(err)
	remedy.name = dbMapString(raw, "name")
	remedy.driver = dbMapString(raw, "driver")
	remedy.user = dbMapString(raw, "user")
	ssh := dbMapObject(raw, "ssh")
	remedy.sshHost = dbMapString(ssh, "host")
	remedy.sshUser = dbMapString(ssh, "user")
	remedy.sshPort = dbMapNumbers(ssh, "port")
	remedy.sshAuth = dbMapString(ssh, "auth")
	remedy.save = remedy.kind == "password" && dbMapBool(raw, "savePassword") ||
		remedy.kind == "sshSecret" && dbMapBool(ssh, "saveSecret")
	if remedy.name == "" {
		for _, source := range m.db.sources {
			if source.ID == remedy.source {
				remedy.name = source.Name
				break
			}
		}
	}
	m.db.connectRemedy = remedy
	m.invalidate()
}

func (m *Model) ApplyDBConnectRemedy() {
	if m.db == nil || m.db.connectRemedy == nil {
		return
	}
	remedy := m.db.connectRemedy
	if remedy.busy {
		return
	}
	remedy.busy = true
	remedy.err = ""
	m.invalidate()
	go func() {
		ctx, cancel := context.WithTimeout(m.ctx, 30*time.Second)
		defer cancel()
		var err error
		switch remedy.kind {
		case "trustHostKey":
			err = m.mgr.TrustDBSSHHost(ctx, Window, remedy.source)
		case "password":
			err = m.mgr.DBSetPassword(ctx, Window, remedy.source, &remedy.secret, remedy.save)
		case "sshSecret":
			err = m.mgr.DBSetSSHSecret(ctx, Window, remedy.source, &remedy.secret, remedy.save)
		default:
			err = fmt.Errorf("unknown connection remedy %q", remedy.kind)
		}
		m.post(func() {
			if m.db == nil || m.db.connectRemedy != remedy {
				return
			}
			remedy.busy = false
			if err != nil {
				remedy.err = err.Error()
				m.invalidate()
				return
			}
			target := remedy.target
			m.db.connectRemedy = nil
			m.ConnectDBSource(target)
		})
	}()
}

func (m *Model) dbConnectRemedyDialog(c *ui.Context, p *ui.Theme) {
	if m.db == nil || m.db.connectRemedy == nil {
		return
	}
	remedy := m.db.connectRemedy
	open := remedy.open
	ui.Modal(c, &open, func() {
		width := 420
		if remedy.kind == "trustHostKey" {
			width = 500
		}
		ui.Column(c).Width(float32(width)).Gap(10).Children(func() {
			title := "Connect to " + remedy.name
			if remedy.kind == "sshSecret" {
				label := remedy.sshHost
				if remedy.sshUser != "" {
					label = remedy.sshUser + "@" + label
				}
				if remedy.sshPort != "" {
					label += ":" + remedy.sshPort
				}
				if label == "" {
					label = remedy.name
				}
				title = "SSH: " + label
			} else if remedy.kind == "trustHostKey" {
				title = "Unknown SSH Host"
			}
			ui.Text(c, title).FontSize(13).Bold().TextColor(p.Text).SingleLine()
			switch remedy.kind {
			case "trustHostKey":
				ui.Text(c, remedy.prompt+". The authenticity of the host cannot be established. "+
					"Trust it and add the key to ~/.ssh/known_hosts?").
					FontSize(12).TextColor(p.TextMuted)
			case "password":
				ui.Row(c).Gap(8).Children(func() {
					ui.Text(c, "User:").FontSize(12).TextColor(p.TextMuted).Width(70)
					user := remedy.user
					if user == "" {
						user = "(default)"
					}
					ui.Text(c, user).FontSize(12).TextColor(p.Text)
				})
				label := "Password:"
				if remedy.driver == "snowflake" {
					label = "Snowflake secret:"
				}
				ui.Text(c, label).FontSize(12).TextColor(p.TextMuted)
				ui.TextInput(c, &remedy.secret).Height(28).Password().AutoFocus()
				ui.Checkbox(c, &remedy.save, "Save password")
			case "sshSecret":
				ui.Text(c, remedy.prompt).FontSize(12).TextColor(p.TextMuted)
				label := "Passphrase:"
				if remedy.sshAuth == "password" || strings.Contains(remedy.prompt, "password required") {
					label = "Password:"
				}
				ui.Text(c, label).FontSize(12).TextColor(p.TextMuted)
				ui.TextInput(c, &remedy.secret).Height(28).Password().AutoFocus()
				ui.Checkbox(c, &remedy.save, "Save password")
			}
			if remedy.err != "" {
				ui.Text(c, remedy.err).FontSize(12).TextColor(p.Danger).SingleLine().Ellipsis("…")
			}
			ui.Row(c).Gap(8).Children(func() {
				ui.Spacer(c)
				ui.Button(c, "Cancel").OnClick(func() {
					m.db.connectRemedy = nil
					m.invalidate()
				})
				label := "Connect"
				if remedy.kind == "trustHostKey" {
					label = "Trust and Connect"
				}
				ui.Button(c, label).Disabled(remedy.busy ||
					remedy.kind != "trustHostKey" && strings.TrimSpace(remedy.secret) == "").
					OnClick(func() { m.ApplyDBConnectRemedy() })
			})
		})
	})
	if !open && m.db != nil && m.db.connectRemedy == remedy {
		m.db.connectRemedy = nil
		m.invalidate()
	}
}
