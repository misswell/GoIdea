package app

// Resolving a Java Application/Spring Boot run configuration before run_start.
// The whole flow lives in internal/adapter (the port of OpenIDEA's
// platform/javaLaunch.ts); this file only wires it to the GoIDEA Model: the
// backend RPC surface, the run console report and the raw-config round trip.
// A failed preparation is an error: run_start is never called with an
// unresolved configuration.

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"mayoi/internal/adapter"
	"mayoi/internal/openidea"
)

// javaBackend is adapter.Backend over the live OpenIDEA server. Every call
// goes through Model.invoke, the seam tests use to inject a fake RPC client
// and prove that a failed preparation never reaches run_start.
type javaBackend struct{ m *Model }

func (b javaBackend) FindFiles(ctx context.Context, query string, limit int) ([]openidea.FileMatch, error) {
	raw, err := b.m.invoke(ctx, "find_files", map[string]any{"query": query, "limit": limit})
	if err != nil {
		return nil, err
	}
	var files []openidea.FileMatch
	if err := json.Unmarshal(raw, &files); err != nil {
		return nil, err
	}
	return files, nil
}

func (b javaBackend) ReadText(ctx context.Context, path string) (string, error) {
	raw, err := b.m.invoke(ctx, "fs_read_text", map[string]any{"path": path})
	if err != nil {
		return "", err
	}
	var text string
	if err := json.Unmarshal(raw, &text); err != nil {
		return "", err
	}
	return text, nil
}

func (b javaBackend) RunConfigs(ctx context.Context) ([]openidea.RunConfig, error) {
	raw, err := b.m.invoke(ctx, "run_configs", nil)
	if err != nil {
		return nil, err
	}
	return openidea.DecodeRunConfigs(raw)
}

func (b javaBackend) ResolveLaunch(ctx context.Context, module string, test bool, policy string, request uint32) (json.RawMessage, error) {
	return b.m.invoke(ctx, "java_resolve_launch", map[string]any{
		"module": module, "test": test, "policy": policy, "request": request,
	})
}

// invoke is the one place the app talks to the OpenIDEA backend's invoke
// surface; tests set invokeHook instead of a live server.
func (m *Model) invoke(ctx context.Context, cmd string, args any) (json.RawMessage, error) {
	if m.invokeHook != nil {
		return m.invokeHook(ctx, cmd, args)
	}
	return m.mgr.Invoke(ctx, Window, cmd, args)
}

// resolveJVMConfig returns the configuration with its classpath placeholders
// and java launcher resolved through java_resolve_launch. A configuration of
// another kind comes back unchanged. Every preparation failure is an error:
// the caller shows it and does not start the process.
func (m *Model) resolveJVMConfig(ctx context.Context, raw json.RawMessage, root string, report func(text, kind string)) (json.RawMessage, error) {
	cfg, err := openidea.DecodeRunConfig(raw)
	if err != nil {
		return nil, fmt.Errorf("运行配置无法解析：%w", err)
	}
	if !adapter.IsJVMApplication(cfg.Kind) {
		return raw, nil
	}
	prepared, err := adapter.PrepareJavaLaunch(ctx, javaBackend{m}, cfg, &adapter.Preparation{
		Request: m.nextRequest(),
		Report:  report,
	})
	if err != nil {
		return nil, err
	}
	return resolvedRunConfig(raw, prepared.Command, prepared.Resolved)
}

// resolvedRunConfig writes the resolved command and arguments back into the
// configuration exactly as the backend sent it, so run_start keeps every
// field GoIDEA does not model.
func resolvedRunConfig(raw json.RawMessage, command string, args []string) (json.RawMessage, error) {
	var obj map[string]any
	if err := json.Unmarshal(raw, &obj); err != nil {
		return nil, fmt.Errorf("运行配置无法解析：%w", err)
	}
	obj["command"], obj["args"] = command, args
	return json.Marshal(obj)
}

// reportRunConsole appends preparation messages to the run console of the
// project they belong to: a project switch during the preparation must not
// leak text into the new project's empty console.
func (m *Model) reportRunConsole(root string, ch uint32) func(text, kind string) {
	return func(text, kind string) {
		m.post(func() {
			if m.Root() != root || m.run.channel != ch {
				return
			}
			if kind == "" {
				kind = "system"
			}
			for _, line := range strings.Split(strings.TrimSuffix(text, "\n"), "\n") {
				if line == "" {
					continue
				}
				m.run.output = append(m.run.output, outputLine{kind: kind, text: line})
			}
			if over := len(m.run.output) - maxRunLines; over > 0 {
				m.run.output = append(m.run.output[:0], m.run.output[over:]...)
				m.run.truncated = true
			}
			m.invalidate()
		})
	}
}
