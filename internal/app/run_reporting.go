package app

import (
	"encoding/json"
	"strings"

	"mayoi/internal/adapter"
	"mayoi/internal/openidea"
)

// prepareTestReporting ports OpenIDEA's withTestReporting: it adds the launch
// flags each parser needs, without discarding fields the native form does not
// model.
func prepareTestReporting(
	raw json.RawMessage,
	cfg *openidea.RunConfig,
) (json.RawMessage, *adapter.DotnetReports) {
	if cfg == nil || !cfg.IsTest {
		return raw, nil
	}
	var dotnetReports *adapter.DotnetReports
	switch adapter.FrameworkFor(cfg) {
	case "go":
		if len(cfg.Args) > 0 && cfg.Args[0] == "test" {
			hasJSON := false
			for _, arg := range cfg.Args {
				if arg == "-json" || strings.HasPrefix(arg, "-json=") {
					hasJSON = true
					break
				}
			}
			if !hasJSON {
				if patched, ok := patchRunConfigArgs(raw, append([]string{"test", "-json"}, cfg.Args[1:]...)); ok {
					raw = patched
				}
			}
		}
	case "pytest":
		chosen := false
		for _, arg := range cfg.Args {
			if arg == "-v" || arg == "-q" || arg == "--verbose" || arg == "--quiet" ||
				strings.HasPrefix(arg, "--verbosity") {
				chosen = true
				break
			}
		}
		if !chosen {
			if patched, ok := patchRunConfigArgs(raw, append(append([]string(nil), cfg.Args...), "-v")); ok {
				raw = patched
			}
		}
	case "unittest":
		if patched, ok := patchRunConfigArgs(raw, adapter.UnittestWithVerbose(cfg)); ok {
			raw = patched
		}
	case "phpunit":
		if patched, ok := patchRunConfigArgs(raw, adapter.PHPUnitWithTeamcity(cfg.Args)); ok {
			raw = patched
		}
	case "ruby":
		if cfg.Kind != "rspec" {
			if patched, ok := patchRunConfigArgs(raw, adapter.RubyVerboseArgs(cfg)); ok {
				raw = patched
			}
		}
	case "dotnet":
		dotnetReports = adapter.NewDotnetReports(cfg)
		if patched, ok := patchRunConfigArgs(raw, dotnetReports.Args(cfg)); ok {
			raw = patched
		}
	}
	return raw, dotnetReports
}
