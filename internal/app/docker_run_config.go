package app

import (
	"context"
	"encoding/json"
	"errors"
	"strings"

	"github.com/egoist/mygo/ui"
	"mayoi/internal/adapter"
	"mayoi/internal/openidea"
)

type dockerFormField struct {
	key, label string
	check      bool
}

func dockerFormFields(kind string) []dockerFormField {
	fields := []dockerFormField{{"connection", "Docker connection", false}}
	switch kind {
	case "docker-dockerfile":
		fields = append(fields, dockerFormField{"dockerfile", "Dockerfile", false}, dockerFormField{"context", "Context folder", false}, dockerFormField{"tag", "Image tag", false}, dockerFormField{"target", "Build stage", false}, dockerFormField{"buildArgs", "Build args", false}, dockerFormField{"buildOptions", "Build options", false}, dockerFormField{"runImage", "Run built image", true})
	case "docker-image":
		fields = append(fields, dockerFormField{"image", "Image ID or name", false}, dockerFormField{"pull", "Pull before running", true})
	case "docker-compose":
		return append(fields, dockerFormField{"files", "Compose files", false}, dockerFormField{"services", "Services", false}, dockerFormField{"project", "Project name", false}, dockerFormField{"detach", "Run in background", true}, dockerFormField{"build", "Build images", true}, dockerFormField{"upOptions", "Up options", false})
	}
	return append(fields, dockerFormField{"containerName", "Container name", false}, dockerFormField{"ports", "Bind ports", false}, dockerFormField{"volumes", "Bind mounts", false}, dockerFormField{"containerEnv", "Container environment", false}, dockerFormField{"runOptions", "Run options", false}, dockerFormField{"command", "Container command", false})
}

func isDockerRunKind(kind string) bool {
	return kind == "docker-dockerfile" || kind == "docker-image" || kind == "docker-compose"
}

func (d *runConfigDlg) loadDockerFields(cfg *openidea.RunConfig) {
	d.dockerFields = map[string]*string{}
	for _, f := range dockerFormFields(cfg.Kind) {
		value, _ := cfg.Params[f.key].(string)
		if f.key == "connection" && strings.TrimSpace(value) == "" {
			value = "default"
		}
		d.dockerFields[f.key] = &value
	}
}

func (d *runConfigDlg) renderDockerFields(c *ui.Context, kind string, p *ui.Theme) {
	for _, f := range dockerFormFields(kind) {
		value := d.dockerFields[f.key]
		if value == nil {
			continue
		}
		if f.check {
			checked := *value == "true"
			if ui.Checkbox(c, &checked, f.label).Changed() {
				if checked {
					*value = "true"
				} else {
					*value = "false"
				}
			}
		} else {
			ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
				ui.Text(c, f.label).FontSize(12).TextColor(p.TextMuted).Width(140).SingleLine()
				input := ui.TextInput(c, value).Width(520)
				if f.key == "connection" {
					input.Placeholder("default, or a connection ID from Services")
				}
			})
		}
		if e := d.errs[f.key]; e != "" {
			ui.Text(c, e).FontSize(12).TextColor(p.Danger).SingleLine()
		}
	}
}

// ConfigObject preserves unknown fields, but reads params from Raw. Rebuild Raw
// after editing so normal save and template paths retain the updated values.
func (d *runConfigDlg) applyDockerFields(cfg *openidea.RunConfig) (openidea.RunConfig, error) {
	obj, err := adapter.ConfigObject(cfg)
	if err != nil {
		return *cfg, err
	}
	params := make(map[string]any, len(cfg.Params)+len(d.dockerFields))
	for k, v := range cfg.Params {
		params[k] = v
	}
	for k, v := range d.dockerFields {
		if v != nil {
			value := *v
			if k == "connection" {
				value = strings.TrimSpace(value)
				if value == "" {
					value = "default"
				}
			}
			params[k] = value
		}
	}
	obj["params"] = params
	raw, err := json.Marshal(obj)
	if err != nil {
		return *cfg, err
	}
	updated, err := openidea.DecodeRunConfig(raw)
	if err != nil {
		return *cfg, err
	}
	prepared, err := dockerPrepareRunConfig(updated)
	if err != nil {
		return *updated, err
	}
	return *prepared, nil
}

func validateDockerFields(cfg *openidea.RunConfig, errs adapter.ConfigErrors) {
	key := ""
	switch cfg.Kind {
	case "docker-dockerfile":
		key = "dockerfile"
	case "docker-image":
		key = "image"
	case "docker-compose":
		key = "files"
	}
	if value, _ := cfg.Params[key].(string); strings.TrimSpace(value) == "" {
		errs[key] = "This field is required"
	}
	if _, err := dockerPrepareRunConfig(cfg); err != nil {
		errs["runOptions"] = "Check Docker command options"
	}
}

func (m *Model) prepareDockerLaunch(ctx context.Context, raw json.RawMessage, root string) (json.RawMessage, error) {
	cfg, err := openidea.DecodeRunConfig(raw)
	if err != nil || !isDockerRunKind(cfg.Kind) {
		return raw, err
	}
	errs := adapter.ConfigErrors{}
	validateDockerFields(cfg, errs)
	if errs.HasErrors() {
		return nil, errors.New("Check the Docker configuration fields")
	}
	cfg, err = dockerPrepareRunConfig(cfg)
	if err != nil {
		return nil, errors.New("Check Docker command options")
	}
	id, _ := cfg.Params["connection"].(string)
	id = strings.TrimSpace(id)
	endpoint := openidea.DockerEndpoint{Kind: "default"}
	if id != "" && id != "default" {
		data, e := m.invoke(ctx, "docker_settings", nil)
		if e != nil {
			return nil, errors.New("Could not load Docker connections")
		}
		var settings openidea.DockerSettings
		if json.Unmarshal(data, &settings) != nil {
			return nil, errors.New("Could not read Docker connections")
		}
		found := false
		for _, connection := range settings.Connections {
			if connection.ID == id {
				endpoint = connection.Endpoint
				found = true
				break
			}
		}
		if !found {
			return nil, errors.New("Docker connection was not found")
		}
	}
	data, err := m.invoke(ctx, "docker_run_env", map[string]any{"endpoint": endpoint})
	if err != nil {
		return nil, errors.New("Could not prepare Docker connection")
	}
	var env map[string]string
	if json.Unmarshal(data, &env) != nil {
		return nil, errors.New("Could not read Docker connection environment")
	}
	// Match OpenIDEA: explicit config variables override the connection environment.
	for k, v := range cfg.Env {
		env[k] = v
	}
	cfg.Env = env
	obj, err := adapter.ConfigObject(cfg)
	if err != nil {
		return nil, err
	}
	return json.Marshal(obj)
}
