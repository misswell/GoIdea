package app

// SmokeTrust proves the authorization path on the real backend: an untrusted
// project is opened, the Trust Project prompt is up, granting it reopens the
// documents and loads the run configurations.

import (
	"context"
	"fmt"
	"time"
)

// SmokeTrust waits for the trust prompt, grants trust and reports what changed.
func (m *Model) SmokeTrust(ctx context.Context) (map[string]any, error) {
	out := map[string]any{}
	deadline := time.Now().Add(30 * time.Second)
	var asked bool
	for {
		var phase string
		m.post(func() { asked, phase = m.trustAsk, m.Phase() })
		if asked || time.Now().After(deadline) {
			break
		}
		if phase == "failed" {
			return out, fmt.Errorf("启动失败，无法检查授权")
		}
		select {
		case <-ctx.Done():
			return out, ctx.Err()
		case <-time.After(100 * time.Millisecond):
		}
	}
	out["promptShown"] = asked
	if !asked {
		return out, fmt.Errorf("未信任项目没有弹出 Trust Project")
	}
	m.post(func() { m.TrustProject() })
	deadline = time.Now().Add(60 * time.Second)
	for {
		var trusted, loaded bool
		var errText string
		var configs int
		m.post(func() {
			trusted, loaded = m.run.trusted, m.run.loaded && len(m.run.configs) > 0
			errText, configs = m.run.trustErr, len(m.run.configs)
		})
		if errText != "" {
			return out, fmt.Errorf("%s", errText)
		}
		if trusted && loaded {
			out["trusted"], out["configs"] = true, configs
			break
		}
		if time.Now().After(deadline) {
			out["trusted"], out["configs"] = trusted, configs
			return out, fmt.Errorf("授权后未加载运行配置")
		}
		select {
		case <-ctx.Done():
			return out, ctx.Err()
		case <-time.After(150 * time.Millisecond):
		}
	}
	var tabs int
	m.post(func() { tabs = len(m.tabs) })
	out["tabs"] = tabs
	return out, nil
}
