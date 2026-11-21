package app

import (
	"context"
	"encoding/json"
	"fmt"
	"time"
)

// SmokeAgents exercises the real Agents host RPC surface without creating a
// task. It proves the initial subscription, provider discovery and external
// session scan all return the arrays the native window decodes.
func (m *Model) SmokeAgents(ctx context.Context) (map[string]any, error) {
	if m.Root() == "" {
		return nil, fmt.Errorf("Agents smoke needs an open project")
	}
	if err := m.waitForSmokeAgentTrust(ctx); err != nil {
		return nil, err
	}
	channel := m.nextChannel()
	rawTasks, err := m.invoke(ctx, "agent_subscribe", map[string]any{
		"onEvent": map[string]any{"$channel": channel},
	})
	if err != nil {
		return nil, fmt.Errorf("agent_subscribe: %w", err)
	}
	var tasks []agentTask
	if err := json.Unmarshal(rawTasks, &tasks); err != nil {
		return nil, fmt.Errorf("decode agent_subscribe: %w", err)
	}
	if tasks == nil {
		return nil, fmt.Errorf("agent_subscribe returned null instead of a task array")
	}

	rawProviders, err := m.invoke(ctx, "agent_providers", nil)
	if err != nil {
		return nil, fmt.Errorf("agent_providers: %w", err)
	}
	var providers []agentProvider
	if err := json.Unmarshal(rawProviders, &providers); err != nil {
		return nil, fmt.Errorf("decode agent_providers: %w", err)
	}
	if providers == nil {
		return nil, fmt.Errorf("agent_providers returned null instead of a provider array")
	}
	providerIDs := make([]string, 0, len(providers))
	for _, provider := range providers {
		providerIDs = append(providerIDs, provider.ID)
	}

	rawExternal, err := m.invoke(ctx, "agent_external_sessions", nil)
	if err != nil {
		return nil, fmt.Errorf("agent_external_sessions: %w", err)
	}
	var external []agentExternalSession
	if err := json.Unmarshal(rawExternal, &external); err != nil {
		return nil, fmt.Errorf("decode agent_external_sessions: %w", err)
	}
	if external == nil {
		return nil, fmt.Errorf("agent_external_sessions returned null instead of a session array")
	}

	return map[string]any{
		"trusted":        true,
		"subscribed":     true,
		"tasks":          len(tasks),
		"providers":      len(providers),
		"providerIDs":    providerIDs,
		"external":       len(external),
		"externalParsed": true,
	}, nil
}

// waitForSmokeAgentTrust uses the same explicit permission action as the trust
// prompt. Reused E2E data may already be trusted, so both states are accepted.
func (m *Model) waitForSmokeAgentTrust(ctx context.Context) error {
	deadline := time.NewTimer(45 * time.Second)
	defer deadline.Stop()
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	for {
		var trusted, asked, busy bool
		var failure string
		m.post(func() {
			trusted = m.run.trusted
			asked = m.trustAsk
			busy = m.run.busy
			failure = m.run.trustErr
		})
		if trusted {
			return nil
		}
		if failure != "" {
			return fmt.Errorf("project trust: %s", failure)
		}
		if asked && !busy {
			m.post(m.TrustProject)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-deadline.C:
			return fmt.Errorf("Agents smoke timed out waiting for project trust")
		case <-ticker.C:
		}
	}
}
