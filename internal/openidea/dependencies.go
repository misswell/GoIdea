package openidea

import (
	"context"
	"encoding/json"
)

// ManifestDependency is a dependency version range in a manifest. From and To
// are UTF-16 offsets into the document and exclude the surrounding quotes.
type ManifestDependency struct {
	Package     string `json:"package"`
	Requirement string `json:"requirement"`
	From        int    `json:"from"`
	To          int    `json:"to"`
}

// DependencyUpdate is the newest compatible replacement for a Cargo
// dependency requirement.
type DependencyUpdate struct {
	Latest      string `json:"latest"`
	Replacement string `json:"replacement"`
}

// DependencyVersions is one dependency and the versions available for it.
// Update is populated for Cargo dependencies, which also support inspections
// and a quick fix.
type DependencyVersions struct {
	Dependency ManifestDependency `json:"dependency"`
	Versions   []string           `json:"versions"`
	Update     *DependencyUpdate  `json:"update"`
}

// DependencyVersions asks OpenIDEA for the versions of the manifest dependency
// at offset. A nil offset asks for every Cargo dependency, which the inspection
// uses, while completion always passes the caret's UTF-16 offset.
func (m *Manager) DependencyVersions(
	ctx context.Context,
	window string,
	request uint32,
	doc uint32,
	offset *int,
) ([]DependencyVersions, error) {
	var offsetValue any
	if offset != nil {
		offsetValue = *offset
	}
	raw, err := m.Invoke(ctx, window, "dependency_versions", map[string]any{
		"request": request,
		"doc":     doc,
		"offset":  offsetValue,
	})
	if err != nil {
		return nil, err
	}
	var items []DependencyVersions
	if err := json.Unmarshal(raw, &items); err != nil {
		return nil, err
	}
	return items, nil
}
