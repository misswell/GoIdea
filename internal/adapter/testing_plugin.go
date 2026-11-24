package adapter

import (
	"errors"
	"fmt"
	"strings"

	"mayoi/internal/openidea"
)

// PluginTestMapper converts a plugin's incremental test updates into the same
// tree events as OpenIDEA's built-in parsers. It ports
// src/features/run/testing/plugin.ts PluginTestEvents and keeps its state across
// chunks of one run.
type PluginTestMapper struct {
	paths map[string][]string
	names map[string]string
}

// NewPluginTestMapper creates an empty mapper for one plugin-owned run.
func NewPluginTestMapper() *PluginTestMapper {
	return &PluginTestMapper{
		paths: map[string][]string{},
		names: map[string]string{},
	}
}

func pluginTestPathKey(path []string) string {
	return strings.Join(path, "\x00")
}

func copyTestPath(path []string) []string {
	return append([]string(nil), path...)
}

// Map validates one batch of updates and returns the parsers' common events.
// An invalid or unbounded tree is an error so the caller can switch the whole
// run to its built-in fallback parser.
func (p *PluginTestMapper) Map(events []openidea.PluginTestEvent) ([]TestEvent, error) {
	if p == nil {
		return nil, errors.New("nil plugin test mapper")
	}
	if p.paths == nil {
		p.paths = map[string][]string{}
	}
	if p.names == nil {
		p.names = map[string]string{}
	}
	var out []TestEvent
	for _, event := range events {
		switch event.Type {
		case "other":
			continue
		case "suiteStarted", "testStarted":
			if event.ID == "" || event.Name == "" {
				return nil, errors.New("invalid plugin test start")
			}
			var path []string
			if event.Parent != nil {
				parent, ok := p.paths[*event.Parent]
				if !ok {
					return nil, errors.New("invalid test parent")
				}
				path = copyTestPath(parent)
			}
			path = append(path, event.Name)
			if len(path) > 128 || (len(p.paths) >= 100000 && p.paths[event.ID] == nil) {
				return nil, errors.New("plugin test tree too large")
			}
			key := pluginTestPathKey(path)
			if existingID, ok := p.names[key]; ok && existingID != event.ID {
				path[len(path)-1] += " [" + event.ID + "]"
				key = pluginTestPathKey(path)
			}
			p.names[key] = event.ID
			p.paths[event.ID] = copyTestPath(path)
			out = append(out, TestEvent{Kind: "start", Path: copyTestPath(path)})
			if event.Location != nil {
				out = append(out, TestEvent{
					Kind:     "location",
					Path:     copyTestPath(path),
					Location: pluginTestLocation(event.Location),
				})
			}
		default:
			path, ok := p.paths[event.ID]
			if !ok {
				return nil, fmt.Errorf("unknown plugin test id %q", event.ID)
			}
			switch event.Type {
			case "output":
				out = append(out, TestEvent{
					Kind: "output",
					Path: copyTestPath(path),
					Line: event.Text,
				})
			case "skipped":
				out = append(out, TestEvent{
					Kind:   "result",
					Path:   copyTestPath(path),
					Status: TestSkipped,
				})
				if event.Reason != nil {
					out = append(out, TestEvent{
						Kind: "output",
						Path: copyTestPath(path),
						Line: *event.Reason,
					})
				}
			case "passed", "failed":
				status := TestPassed
				if event.Type == "failed" {
					status = TestFailed
				}
				out = append(out, TestEvent{
					Kind:       "result",
					Path:       copyTestPath(path),
					Status:     status,
					DurationMs: event.DurationMs,
				})
				if event.Type == "failed" {
					out = append(out, TestEvent{
						Kind: "output",
						Path: copyTestPath(path),
						Line: event.Message,
					})
					if event.Expected != nil {
						out = append(out, TestEvent{
							Kind: "output",
							Path: copyTestPath(path),
							Line: "Expected: " + *event.Expected,
						})
					}
					if event.Actual != nil {
						out = append(out, TestEvent{
							Kind: "output",
							Path: copyTestPath(path),
							Line: "Actual: " + *event.Actual,
						})
					}
					if event.Location != nil {
						out = append(out, TestEvent{
							Kind:     "location",
							Path:     copyTestPath(path),
							Location: pluginTestLocation(event.Location),
						})
					}
				}
			default:
				return nil, fmt.Errorf("unknown plugin test event %q", event.Type)
			}
		}
	}
	return out, nil
}

func pluginTestLocation(location *openidea.Location) *TestLocation {
	if location == nil {
		return nil
	}
	return &TestLocation{
		Path:    location.Path,
		Line:    int(location.Range.Start.Line) + 1,
		Col:     int(location.Range.Start.Col) + 1,
		Resolve: "path",
	}
}
