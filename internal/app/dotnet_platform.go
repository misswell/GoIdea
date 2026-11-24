package app

import (
	"context"
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"mayoi/internal/adapter"
)

type dotnetRunner string

const (
	dotnetRunnerVSTest    dotnetRunner = "vstest"
	dotnetRunnerMTP       dotnetRunner = "mtp"
	dotnetRunnerMTPLegacy dotnetRunner = "mtp-legacy"
)

type dotnetPackageRef struct {
	ID      string `json:"id"`
	Version string `json:"version"`
}

type dotnetPackage struct {
	ID         string `json:"id"`
	Version    string `json:"version"`
	Latest     string `json:"latest,omitempty"`
	Transitive bool   `json:"transitive,omitempty"`
}

type dotnetProject struct {
	Path            string             `json:"path"`
	Name            string             `json:"name"`
	Frameworks      []string           `json:"frameworks"`
	Configurations  []string           `json:"configurations"`
	References      []string           `json:"references"`
	Packages        []dotnetPackageRef `json:"packages"`
	Test            bool               `json:"test"`
	Runner          dotnetRunner       `json:"runner"`
	Adapter         string             `json:"adapter"`
	TRX             bool               `json:"trx"`
	PlatformVersion string             `json:"platformVersion,omitempty"`
}

type dotnetSolution struct {
	Path     string   `json:"path"`
	Projects []string `json:"projects"`
}

type dotnetWorkspace struct {
	Projects  []dotnetProject  `json:"projects"`
	Solutions []dotnetSolution `json:"solutions"`
	Errors    []string         `json:"errors,omitempty"`
}

var (
	dotnetProjectFileRE     = regexp.MustCompile(`(?i)\.(?:cs|fs|vb)proj$`)
	dotnetSolutionFileRE    = regexp.MustCompile(`(?i)\.slnx?$`)
	dotnetSolutionProjectRE = regexp.MustCompile(
		`(?m)^Project\([^\r\n]+?=\s*"[^"]*",\s*"([^"]+)"`,
	)
	dotnetWindowsAbsoluteRE = regexp.MustCompile(`^[A-Za-z]:[\\/]`)
	dotnetTestPackageRE     = regexp.MustCompile(
		`(?i)^(?:Microsoft\.NET\.Test\.Sdk|MSTest|NUnit|xunit(?:\.v3(?:\.mtp-v[12])?)?)$`,
	)
	dotnetTestOptionRE = regexp.MustCompile(
		`^(?:--no-build$|--no-restore$|-p:|-property:|--property:)`,
	)
	dotnetTestMethodRE = regexp.MustCompile(`\(.*$`)
)

func dotnetIsAbsolute(path string) bool {
	return filepath.IsAbs(path) || dotnetWindowsAbsoluteRE.MatchString(path)
}

func dotnetAbsolute(base, value string) string {
	value = strings.ReplaceAll(strings.TrimSpace(value), `\`, "/")
	if value == "" {
		return ""
	}
	if dotnetIsAbsolute(value) {
		return filepath.Clean(filepath.FromSlash(value))
	}
	return filepath.Clean(filepath.Join(base, filepath.FromSlash(value)))
}

func dotnetUnique(paths []string) []string {
	seen := make(map[string]bool, len(paths))
	out := make([]string, 0, len(paths))
	for _, path := range paths {
		path = filepath.Clean(path)
		key := filepath.ToSlash(path)
		if path == "." || path == "" || seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, path)
	}
	return out
}

// dotnetSolutionProjectPaths ports solutionPaths from platform/dotnet.ts.
// Legacy .sln files are read line-by-line; .slnx is XML.
func dotnetSolutionProjectPaths(text string, xmlDocument bool, dir string) []string {
	var names []string
	if xmlDocument {
		decoder := xml.NewDecoder(strings.NewReader(text))
		for {
			token, err := decoder.Token()
			if err != nil {
				break
			}
			start, ok := token.(xml.StartElement)
			if !ok || start.Name.Local != "Project" {
				continue
			}
			for _, attr := range start.Attr {
				if attr.Name.Local == "Path" {
					names = append(names, attr.Value)
					break
				}
			}
		}
	} else {
		for _, match := range dotnetSolutionProjectRE.FindAllStringSubmatch(text, -1) {
			if len(match) > 1 {
				names = append(names, match[1])
			}
		}
	}
	paths := make([]string, 0, len(names))
	for _, name := range names {
		if dotnetProjectFileRE.MatchString(name) {
			paths = append(paths, dotnetAbsolute(dir, name))
		}
	}
	return dotnetUnique(paths)
}

// dotnetGlobalRunner parses global.json after removing comments and trailing
// commas, matching the JSONC input accepted by the pinned frontend.
func dotnetGlobalRunner(text string) (dotnetRunner, error) {
	clean := dotnetJSONC(text)
	var value struct {
		Test struct {
			Runner string `json:"runner"`
		} `json:"test"`
	}
	if err := json.Unmarshal([]byte(clean), &value); err != nil {
		return "", err
	}
	if value.Test.Runner == "Microsoft.Testing.Platform" {
		return dotnetRunnerMTP, nil
	}
	return dotnetRunnerVSTest, nil
}

func dotnetJSONC(text string) string {
	var withoutComments strings.Builder
	withoutComments.Grow(len(text))
	inString, escaped, lineComment, blockComment := false, false, false, false
	for i := 0; i < len(text); i++ {
		ch := text[i]
		next := byte(0)
		if i+1 < len(text) {
			next = text[i+1]
		}
		if lineComment {
			if ch == '\n' {
				lineComment = false
				withoutComments.WriteByte(ch)
			}
			continue
		}
		if blockComment {
			if ch == '*' && next == '/' {
				blockComment = false
				i++
			}
			continue
		}
		if inString {
			withoutComments.WriteByte(ch)
			if escaped {
				escaped = false
			} else if ch == '\\' {
				escaped = true
			} else if ch == '"' {
				inString = false
			}
			continue
		}
		switch {
		case ch == '"':
			inString = true
			withoutComments.WriteByte(ch)
		case ch == '/' && next == '/':
			lineComment = true
			i++
		case ch == '/' && next == '*':
			blockComment = true
			i++
		default:
			withoutComments.WriteByte(ch)
		}
	}

	clean := withoutComments.String()
	var out strings.Builder
	out.Grow(len(clean))
	inString, escaped = false, false
	for i := 0; i < len(clean); i++ {
		ch := clean[i]
		if inString {
			out.WriteByte(ch)
			if escaped {
				escaped = false
			} else if ch == '\\' {
				escaped = true
			} else if ch == '"' {
				inString = false
			}
			continue
		}
		if ch == '"' {
			inString = true
			out.WriteByte(ch)
			continue
		}
		if ch == ',' {
			j := i + 1
			for j < len(clean) {
				switch clean[j] {
				case ' ', '\t', '\r', '\n':
					j++
				default:
					goto nextRune
				}
			}
		nextRune:
			if j < len(clean) && (clean[j] == '}' || clean[j] == ']') {
				continue
			}
		}
		out.WriteByte(ch)
	}
	return out.String()
}

func dotnetJSONOutput(text string) ([]byte, error) {
	start := strings.IndexByte(text, '{')
	end := strings.LastIndexByte(text, '}')
	if start < 0 || end < start {
		return nil, errors.New(".NET returned no JSON object")
	}
	data := []byte(text[start : end+1])
	if !json.Valid(data) {
		return nil, errors.New(".NET returned malformed JSON")
	}
	return data, nil
}

func dotnetParamString(params map[string]any, key string) string {
	value, ok := params[key]
	if !ok || value == nil {
		return ""
	}
	switch value := value.(type) {
	case string:
		return value
	case fmt.Stringer:
		return value.String()
	default:
		return fmt.Sprint(value)
	}
}

func dotnetRunnerForProject(ctx context.Context, m *Model, dir string) (dotnetRunner, error) {
	current := filepath.Clean(dir)
	for {
		path := filepath.Join(current, "global.json")
		text, found, err := dotnetReadText(ctx, m, path)
		if err == nil && found {
			return dotnetGlobalRunner(text)
		}
		parent := filepath.Dir(current)
		if parent == current {
			break
		}
		current = parent
	}
	return dotnetRunnerVSTest, nil
}

func dotnetReadText(ctx context.Context, m *Model, path string) (string, bool, error) {
	raw, err := m.invoke(ctx, "fs_read_text", map[string]any{"path": path})
	if err != nil {
		return "", false, err
	}
	var text string
	if err := json.Unmarshal(raw, &text); err != nil {
		return "", false, err
	}
	return text, true, nil
}

func dotnetMsbuildArgs(path, configuration, framework string) []string {
	args := []string{
		"msbuild",
		path,
		"-nologo",
		"-p:Configuration=" + configuration,
	}
	if framework != "" {
		args = append(args, "-p:TargetFramework="+framework)
	}
	return append(args,
		"-getProperty:MSBuildProjectName,TargetFramework,TargetFrameworks,Configurations,IsTestProject,IsTestingPlatformApplication,TestingPlatformDotnetTestSupport,EnableMSTestRunner,ProjectAssetsFile",
		"-getItem:ProjectReference,PackageReference",
	)
}

type dotnetEvaluation struct {
	Properties map[string]string              `json:"Properties"`
	Items      map[string][]map[string]string `json:"Items"`
}

func dotnetEvaluateProject(
	ctx context.Context,
	m *Model,
	path, configuration, framework string,
) (dotnetProject, error) {
	path = filepath.Clean(path)
	if configuration == "" {
		configuration = "Debug"
	}
	output, err := m.runDotnetCommand(
		ctx,
		dotnetMsbuildArgs(path, configuration, framework),
		filepath.Dir(path),
	)
	if err != nil {
		return dotnetProject{}, err
	}
	data, err := dotnetJSONOutput(output)
	if err != nil {
		return dotnetProject{}, err
	}
	var evaluation dotnetEvaluation
	if err := json.Unmarshal(data, &evaluation); err != nil {
		return dotnetProject{}, err
	}
	properties := evaluation.Properties
	if framework == "" && properties["TargetFrameworks"] != "" {
		first := strings.Split(properties["TargetFrameworks"], ";")[0]
		return dotnetEvaluateProject(ctx, m, path, configuration, strings.TrimSpace(first))
	}

	packages := make([]dotnetPackageRef, 0, len(evaluation.Items["PackageReference"]))
	for _, item := range evaluation.Items["PackageReference"] {
		id, _ := item["Identity"]
		if id == "" {
			continue
		}
		packages = append(packages, dotnetPackageRef{ID: id, Version: item["Version"]})
	}
	runner, err := dotnetRunnerForProject(ctx, m, filepath.Dir(path))
	if err != nil {
		return dotnetProject{}, err
	}

	mtp := dotnetPropertyTrue(properties, "IsTestingPlatformApplication") ||
		dotnetPropertyTrue(properties, "TestingPlatformDotnetTestSupport") ||
		dotnetPropertyTrue(properties, "EnableMSTestRunner")
	for _, pkg := range packages {
		if strings.Contains(strings.ToLower(pkg.ID), "xunit.v3.mtp") {
			mtp = true
		}
	}

	trx := false
	platformVersion := ""
	if assetsPath := properties["ProjectAssetsFile"]; assetsPath != "" {
		if !dotnetIsAbsolute(assetsPath) {
			assetsPath = filepath.Join(filepath.Dir(path), assetsPath)
		}
		if text, found, readErr := dotnetReadText(ctx, m, assetsPath); readErr == nil && found {
			var assets struct {
				Libraries map[string]json.RawMessage `json:"libraries"`
			}
			if json.Unmarshal([]byte(text), &assets) == nil {
				keys := make([]string, 0, len(assets.Libraries))
				for key := range assets.Libraries {
					keys = append(keys, key)
				}
				sort.Strings(keys)
				for _, key := range keys {
					lower := strings.ToLower(key)
					if strings.HasPrefix(lower, "microsoft.testing.extensions.trxreport/") {
						trx = true
					}
					if platformVersion == "" &&
						strings.HasPrefix(lower, "microsoft.testing.platform/") {
						platformVersion = strings.SplitN(key, "/", 2)[1]
					}
				}
			}
		}
	}
	for _, pkg := range packages {
		if strings.EqualFold(pkg.ID, "Microsoft.Testing.Extensions.TrxReport") {
			trx = true
		}
	}

	test := dotnetPropertyTrue(properties, "IsTestProject") || mtp
	if properties["IsTestProject"] != "false" {
		for _, pkg := range packages {
			if dotnetTestPackageRE.MatchString(pkg.ID) {
				test = true
			}
		}
	}
	projectRunner := runner
	if runner != dotnetRunnerMTP && mtp {
		projectRunner = dotnetRunnerMTPLegacy
	}
	projectAdapter := "mstest"
	for _, pkg := range packages {
		lower := strings.ToLower(pkg.ID)
		switch {
		case strings.HasPrefix(lower, "xunit"):
			projectAdapter = "xunit"
		case strings.HasPrefix(lower, "nunit"):
			projectAdapter = "nunit"
		}
	}
	references := make([]string, 0, len(evaluation.Items["ProjectReference"]))
	for _, item := range evaluation.Items["ProjectReference"] {
		if identity := item["Identity"]; identity != "" {
			references = append(references, dotnetAbsolute(filepath.Dir(path), identity))
		}
	}
	frameworks := strings.Split(properties["TargetFrameworks"], ";")
	if properties["TargetFramework"] != "" {
		frameworks = strings.Split(properties["TargetFramework"], ";")
	}
	frameworks = dotnetNonEmptyParts(frameworks)
	configurations := strings.Split(properties["Configurations"], ";")
	if properties["Configurations"] == "" {
		configurations = []string{"Debug", "Release"}
	}
	configurations = dotnetNonEmptyParts(configurations)
	name := properties["MSBuildProjectName"]
	if name == "" {
		name = filepath.Base(path)
	}
	return dotnetProject{
		Path:            path,
		Name:            name,
		Frameworks:      frameworks,
		Configurations:  configurations,
		References:      dotnetUnique(references),
		Packages:        packages,
		Test:            test,
		Runner:          projectRunner,
		Adapter:         projectAdapter,
		TRX:             trx,
		PlatformVersion: platformVersion,
	}, nil
}

func dotnetPropertyTrue(properties map[string]string, key string) bool {
	return strings.EqualFold(strings.TrimSpace(properties[key]), "true")
}

func dotnetNonEmptyParts(parts []string) []string {
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		if part = strings.TrimSpace(part); part != "" {
			out = append(out, part)
		}
	}
	return out
}

type dotnetDirectoryEntry struct {
	Name     string `json:"name"`
	Path     string `json:"path"`
	Kind     string `json:"kind"`
	Excluded bool   `json:"excluded"`
}

func dotnetSkipDirectory(name string, excluded bool) bool {
	if excluded {
		return true
	}
	lower := strings.ToLower(name)
	return strings.HasPrefix(name, ".") ||
		lower == "bin" ||
		lower == "obj" ||
		lower == "node_modules" ||
		lower == "target" ||
		lower == "packages"
}

func dotnetDiscoverWorkspace(ctx context.Context, m *Model, root string) (dotnetWorkspace, error) {
	root = filepath.Clean(root)
	directories := []string{root}
	projectPaths := make([]string, 0)
	projectSeen := map[string]bool{}
	solutions := make([]dotnetSolution, 0)
	addProject := func(path string) {
		path = filepath.Clean(path)
		key := filepath.ToSlash(path)
		if projectSeen[key] {
			return
		}
		projectSeen[key] = true
		projectPaths = append(projectPaths, path)
	}

	for dirIndex := 0; dirIndex < len(directories); dirIndex++ {
		if err := ctx.Err(); err != nil {
			return dotnetWorkspace{}, err
		}
		dir := directories[dirIndex]
		raw, err := m.invoke(ctx, "fs_read_dir", map[string]any{"path": dir})
		if err != nil {
			return dotnetWorkspace{}, err
		}
		var entries []dotnetDirectoryEntry
		if err := json.Unmarshal(raw, &entries); err != nil {
			return dotnetWorkspace{}, err
		}
		for _, entry := range entries {
			kind := strings.ToLower(entry.Kind)
			name := entry.Name
			if entry.Path == "" {
				entry.Path = filepath.Join(dir, name)
			}
			if kind == "dir" || kind == "directory" {
				if !dotnetSkipDirectory(name, entry.Excluded) {
					directories = append(directories, entry.Path)
				}
				continue
			}
			if dotnetProjectFileRE.MatchString(name) {
				addProject(entry.Path)
				continue
			}
			if !dotnetSolutionFileRE.MatchString(name) {
				continue
			}
			text, found, readErr := dotnetReadText(ctx, m, entry.Path)
			if readErr != nil || !found {
				continue
			}
			projects := dotnetSolutionProjectPaths(
				text,
				strings.HasSuffix(strings.ToLower(name), ".slnx"),
				filepath.Dir(entry.Path),
			)
			solutions = append(solutions, dotnetSolution{Path: entry.Path, Projects: projects})
			for _, project := range projects {
				addProject(project)
			}
		}
	}

	projects := make([]dotnetProject, 0, len(projectPaths))
	errorsFound := make([]string, 0)
	evaluated := map[string]bool{}
	for index := 0; index < len(projectPaths); index++ {
		path := projectPaths[index]
		key := filepath.ToSlash(path)
		if evaluated[key] {
			continue
		}
		evaluated[key] = true
		project, err := dotnetEvaluateProject(ctx, m, path, "Debug", "")
		if err != nil {
			if ctx.Err() != nil {
				return dotnetWorkspace{}, ctx.Err()
			}
			errorsFound = append(errorsFound, path+": "+err.Error())
			continue
		}
		projects = append(projects, project)
		for _, reference := range project.References {
			if dotnetProjectFileRE.MatchString(reference) {
				addProject(reference)
			}
		}
	}
	return dotnetWorkspace{Projects: projects, Solutions: solutions, Errors: errorsFound}, nil
}

type dotnetPackagesJSON struct {
	Projects []struct {
		Frameworks []struct {
			TopLevelPackages   []dotnetPackageJSON `json:"topLevelPackages"`
			TransitivePackages []dotnetPackageJSON `json:"transitivePackages"`
		} `json:"frameworks"`
	} `json:"projects"`
}

type dotnetPackageJSON struct {
	ID               string `json:"id"`
	ResolvedVersion  string `json:"resolvedVersion"`
	RequestedVersion string `json:"requestedVersion"`
	LatestVersion    string `json:"latestVersion"`
}

func dotnetPackagesFromOutput(text string) ([]dotnetPackage, error) {
	data, err := dotnetJSONOutput(text)
	if err != nil {
		return nil, err
	}
	var value dotnetPackagesJSON
	if err := json.Unmarshal(data, &value); err != nil {
		return nil, err
	}
	packages := make([]dotnetPackage, 0)
	indexByKey := map[string]int{}
	add := func(item dotnetPackageJSON, transitive bool) {
		version := item.ResolvedVersion
		if version == "" {
			version = item.RequestedVersion
		}
		key := item.ID + ":" + version
		if index, ok := indexByKey[key]; ok {
			if !transitive {
				packages[index].Transitive = false
			}
			return
		}
		indexByKey[key] = len(packages)
		packages = append(packages, dotnetPackage{
			ID:         item.ID,
			Version:    version,
			Latest:     item.LatestVersion,
			Transitive: transitive,
		})
	}
	for _, project := range value.Projects {
		for _, framework := range project.Frameworks {
			for _, item := range framework.TopLevelPackages {
				add(item, false)
			}
			for _, item := range framework.TransitivePackages {
				add(item, true)
			}
		}
	}
	return packages, nil
}

type dotnetSearchJSON struct {
	Problems []struct {
		Text string `json:"text"`
	} `json:"problems"`
	SearchResult []struct {
		Packages []struct {
			ID            string `json:"id"`
			LatestVersion string `json:"latestVersion"`
			Version       string `json:"version"`
		} `json:"packages"`
	} `json:"searchResult"`
}

func dotnetSearchFromOutput(text string, exact bool) ([]dotnetPackage, error) {
	data, err := dotnetJSONOutput(text)
	if err != nil {
		return nil, err
	}
	var value dotnetSearchJSON
	if err := json.Unmarshal(data, &value); err != nil {
		return nil, err
	}
	found := make([]dotnetPackage, 0)
	for _, source := range value.SearchResult {
		for _, item := range source.Packages {
			version := item.LatestVersion
			if version == "" {
				version = item.Version
			}
			found = append(found, dotnetPackage{ID: item.ID, Version: version})
		}
	}
	if len(value.Problems) > 0 {
		encoded, _ := json.Marshal(value.Problems)
		return nil, errors.New(string(encoded))
	}
	if exact {
		for left, right := 0, len(found)-1; left < right; left, right = left+1, right-1 {
			found[left], found[right] = found[right], found[left]
		}
	}
	return found, nil
}

func dotnetDiscoveredTests(output string) []string {
	ansi := regexp.MustCompile("\x1b\\[[0-9;]*m")
	lines := strings.Split(ansi.ReplaceAllString(output, ""), "\n")
	tests := make([]string, 0)
	seen := map[string]bool{}
	listing := false
	startRE := regexp.MustCompile(
		`(?i)following Tests are available|Discovered tests:|Discovered \d+ tests? in assembly`,
	)
	stopRE := regexp.MustCompile(
		`^(?:Process finished|Tests succeeded:|Tests failed:|Test run summary:|Discovered \d+ tests?\.|Total:)`,
	)
	for _, line := range lines {
		line = strings.TrimSuffix(line, "\r")
		if startRE.MatchString(line) {
			listing = true
			continue
		}
		if !listing {
			continue
		}
		name := strings.TrimSpace(line)
		if name == "" || stopRE.MatchString(name) {
			listing = false
			continue
		}
		if strings.HasPrefix(name, "at ") ||
			strings.HasPrefix(name, "[") ||
			strings.Contains(strings.ToLower(name), ".dll") {
			continue
		}
		if !seen[name] {
			seen[name] = true
			tests = append(tests, name)
		}
	}
	return tests
}

func dotnetTestArgs(
	project dotnetProject,
	framework string,
	extra []string,
	configuration string,
) []string {
	args := []string{"test"}
	if project.Runner == dotnetRunnerMTP {
		args = append(args, "--project")
	}
	args = append(args, project.Path, "--configuration", configuration)
	if framework != "" {
		args = append(args, "--framework", framework)
	}
	if project.Runner == dotnetRunnerMTPLegacy {
		args = append(args,
			"-p:TestingPlatformCaptureOutput=false",
			"-p:TestingPlatformDotnetTestSupport=true",
		)
		if len(extra) > 0 {
			args = append(args, "--")
		}
	}
	return append(args, extra...)
}

func dotnetCommand(
	params map[string]any,
	test bool,
) (string, []string) {
	command := dotnetParamString(params, "executable")
	if command == "" {
		command = "dotnet"
	}
	project := dotnetParamString(params, "project")
	framework := dotnetParamString(params, "framework")
	configuration := dotnetParamString(params, "configuration")
	args := []string{}
	if test {
		args = append(args, "test")
		if dotnetParamString(params, "runner") == string(dotnetRunnerMTP) {
			args = append(args, "--project")
		}
		args = append(args, project)
	} else {
		args = append(args, "run", "--project", project)
	}
	if framework != "" {
		args = append(args, "--framework", framework)
	}
	if configuration != "" {
		args = append(args, "--configuration", configuration)
	}
	if !test {
		if launchProfile := dotnetParamString(params, "launchProfile"); launchProfile != "" {
			args = append(args, "--launch-profile", launchProfile)
		} else {
			args = append(args, "--no-launch-profile")
		}
	}
	return command, args
}

func dotnetTestCommandLine(
	params map[string]any,
) (string, []string, error) {
	command, line := dotnetCommand(params, true)
	runner := dotnetParamString(params, "runner")
	adapterName := dotnetParamString(params, "adapter")
	if runner == string(dotnetRunnerMTPLegacy) {
		line = append(line,
			"-p:TestingPlatformCaptureOutput=false",
			"-p:TestingPlatformDotnetTestSupport=true",
		)
	}
	extra := adapter.ParseArgs(dotnetParamString(params, "programArgs"))
	if runner == string(dotnetRunnerMTPLegacy) {
		boundary := -1
		for index, arg := range extra {
			if arg == "--" {
				boundary = index
				break
			}
		}
		if boundary >= 0 {
			line = append(line, extra[:boundary]...)
			extra = extra[boundary+1:]
		} else {
			options := map[string]bool{
				"--configuration": true, "-c": true,
				"--framework": true, "-f": true,
				"--runtime": true, "-r": true,
				"--verbosity": true, "-v": true,
			}
			runnerArgs := make([]string, 0, len(extra))
			for index := 0; index < len(extra); index++ {
				arg := extra[index]
				if options[arg] {
					line = append(line, arg)
					if index+1 < len(extra) {
						index++
						line = append(line, extra[index])
					}
					continue
				}
				if dotnetTestOptionRE.MatchString(arg) {
					line = append(line, arg)
					continue
				}
				runnerArgs = append(runnerArgs, arg)
			}
			extra = runnerArgs
		}
	}
	if encodedMethods := dotnetParamString(params, "testMethods"); encodedMethods != "" {
		var methods []string
		if err := json.Unmarshal([]byte(encodedMethods), &methods); err != nil {
			return "", nil, fmt.Errorf("test methods: %w", err)
		}
		extra = dotnetWithoutFilters(extra)
		var selection []string
		if runner != string(dotnetRunnerVSTest) && runner != "" && adapterName == "xunit" {
			for _, method := range methods {
				if !strings.Contains(method, ".") {
					method = "*." + method
				}
				selection = append(selection, "--filter-method", method)
			}
		} else {
			filterParts := make([]string, 0, len(methods))
			for _, method := range methods {
				if strings.Contains(method, ".") {
					filterParts = append(filterParts,
						"FullyQualifiedName="+dotnetEscapeFilter(method))
				} else {
					filterParts = append(filterParts,
						"Name="+dotnetEscapeFilter(method))
				}
			}
			selection = []string{"--filter", strings.Join(filterParts, "|")}
		}
		boundary := -1
		for index, arg := range extra {
			if arg == "--" {
				boundary = index
				break
			}
		}
		if boundary < 0 {
			boundary = len(extra)
		}
		extra = append(extra[:boundary], append(selection, extra[boundary:]...)...)
	}
	if runner == string(dotnetRunnerMTPLegacy) &&
		(len(extra) == 0 || extra[0] != "--") {
		line = append(line, "--")
	}
	return command, append(line, extra...), nil
}

func dotnetWithoutFilters(args []string) []string {
	out := make([]string, 0, len(args))
	for index := 0; index < len(args); index++ {
		arg := args[index]
		if arg == "--filter" || arg == "--filter-method" {
			if index+1 < len(args) {
				index++
			}
			continue
		}
		if strings.HasPrefix(arg, "--filter=") {
			continue
		}
		out = append(out, arg)
	}
	return out
}

func dotnetEscapeFilter(value string) string {
	return strings.NewReplacer(
		`\`, `\\`,
		`(`, `\(`,
		`)`, `\)`,
		`&`, `\&`,
		`|`, `\|`,
		`=`, `\=`,
		`!`, `\!`,
		`~`, `\~`,
	).Replace(value)
}

func dotnetTestMethod(value string) string {
	return strings.TrimSpace(dotnetTestMethodRE.ReplaceAllString(value, ""))
}

func dotnetRunConfigObject(args []string, cwd string) map[string]any {
	return map[string]any{
		"id":              fmt.Sprintf("dotnet-task:%d", dotnetTaskSequence.Add(1)),
		"name":            "dotnet " + args[0],
		"kind":            "shell",
		"command":         "dotnet",
		"args":            args,
		"cwd":             cwd,
		"env":             map[string]string{"DOTNET_CLI_UI_LANGUAGE": "en"},
		"detected":        true,
		"isTest":          false,
		"debugAdapter":    nil,
		"emulateTerminal": false,
	}
}

func dotnetProjectIndex(projects []dotnetProject, path string) int {
	path = filepath.Clean(path)
	for index := range projects {
		if filepath.Clean(projects[index].Path) == path {
			return index
		}
	}
	return -1
}
