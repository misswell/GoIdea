package openidea

import (
	"context"
	"encoding/json"
)

// LibraryGroup is one SDK or dependency group under External Libraries.
// Root paths can be folders or read-only archive roots ending in "!".
type LibraryGroup struct {
	Name  string        `json:"name"`
	Kind  string        `json:"kind"`
	Roots []LibraryRoot `json:"roots"`
}

// LibraryRoot is one source root of a library group.
type LibraryRoot struct {
	Name string `json:"name"`
	Path string `json:"path"`
}

// ProjectLibraries streams the project's SDK and dependency roots on channel,
// then returns the fully merged set. request can be cancelled with
// cancel_request.
func (m *Manager) ProjectLibraries(
	ctx context.Context,
	window string,
	request, channel uint32,
) ([]LibraryGroup, error) {
	raw, err := m.Invoke(ctx, window, "sdk_project_libraries", map[string]any{
		"request":  request,
		"onGroups": map[string]any{"$channel": channel},
	})
	if err != nil {
		return nil, err
	}
	var groups []LibraryGroup
	if err := json.Unmarshal(raw, &groups); err != nil {
		return nil, err
	}
	return groups, nil
}
