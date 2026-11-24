package app

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"mayoi/internal/openidea"
	"mayoi/internal/workspace"
)

const (
	graphqlFileLimit = 10000
	graphqlReadLimit = 500
)

type graphqlPane struct {
	root string

	tab          string
	projects     []graphqlSchemaProject
	discovered   bool
	discovering  bool
	generation   uint64
	discoveryErr string
	waiters      []func()
	expanded     map[string]bool

	result  *graphqlQueryResult
	running *graphqlRun

	endpointBySchema   map[string]string
	variablesShown     map[string]bool
	variables          map[string]string
	variablesDraftPath string
	variablesDraft     string
	lastIntrospection  *graphqlLastIntrospection

	environmentDialog *graphqlEnvironmentDialog
	errorDialog       *graphqlErrorDialog
	sentByPath        map[string]graphqlSentContext
	diagnosticPaths   map[string]bool
	schemaEdits       map[string]graphqlSchemaEdit
	introspectionAsk  *graphqlIntrospectionConfirmation
}

type graphqlSchemaEdit struct {
	Version uint64
	Text    string
	Edited  time.Time
}

type graphqlIntrospectionConfirmation struct {
	Target   string
	SDL      string
	Endpoint graphqlEndpoint
}

type graphqlQueryResult struct {
	Path      string
	Endpoint  graphqlEndpoint
	Status    uint16
	HasStatus bool
	TotalMs   float64
	Body      string
	Size      int
	Error     string
}

type graphqlRun struct {
	request uint32
	cancel  context.CancelFunc
}

type graphqlLastIntrospection struct {
	Path     string
	Endpoint string
}

type graphqlEnvironmentDialog struct {
	ConfigPath string
	Variables  []string
	Values     map[string]string
	done       func(bool)
}

type graphqlErrorDialog struct {
	Title   string
	Message string
}

type graphqlSentContext struct {
	Version uint64
	Ranges  []graphqlRange
}

func (m *Model) resetGraphQL() {
	generation := m.graphql.generation + 1
	if m.graphql.running != nil && m.graphql.running.cancel != nil {
		m.graphql.running.cancel()
	}
	for path := range m.graphql.diagnosticPaths {
		m.applyDiagnostics(openidea.DiagnosticsEvent{Path: path, Source: "graphql-schema"})
	}
	m.graphql = graphqlPane{
		root:             m.Root(),
		generation:       generation,
		tab:              "schemas",
		expanded:         map[string]bool{},
		endpointBySchema: map[string]string{},
		variablesShown:   map[string]bool{},
		variables:        map[string]string{},
		diagnosticPaths:  map[string]bool{},
		sentByPath:       map[string]graphqlSentContext{},
		schemaEdits:      map[string]graphqlSchemaEdit{},
	}
}

func (m *Model) ensureGraphQL() {
	if m.graphql.root == m.Root() && m.graphql.expanded != nil &&
		m.graphql.endpointBySchema != nil && m.graphql.variablesShown != nil &&
		m.graphql.variables != nil && m.graphql.diagnosticPaths != nil &&
		m.graphql.schemaEdits != nil {
		if m.graphql.tab == "" {
			m.graphql.tab = "schemas"
		}
		return
	}
	m.resetGraphQL()
}

// ShowGraphQL opens GraphQL's Schemas and Project Structure page.
func (m *Model) ShowGraphQL() {
	m.ensureGraphQL()
	m.graphql.tab = "schemas"
	m.DockShow("graphql")
	m.ensureGraphQLDiscovered(nil)
	m.invalidate()
}

// ToggleGraphQL opens the tool window or closes it from the editor.
func (m *Model) ToggleGraphQL() {
	m.ensureGraphQL()
	m.DockToggle("graphql")
	m.ensureGraphQLDiscovered(nil)
}

func (m *Model) showGraphQLResult() {
	m.ensureGraphQL()
	m.graphql.tab = "result"
	m.DockShow("graphql")
	m.invalidate()
}

func (m *Model) graphqlConfigurationFor(path string) string {
	m.ensureGraphQL()
	if len(m.graphql.projects) == 0 {
		return ""
	}
	if path != "" {
		if project := graphqlSchemaOf(m.graphql.projects, path); project != nil &&
			project.ConfigPath != "" {
			return project.ConfigPath
		}
	}
	for _, project := range m.graphql.projects {
		if project.ConfigPath != "" {
			return project.ConfigPath
		}
	}
	return ""
}

func (m *Model) graphqlSchemaFor(path string) *graphqlSchemaProject {
	m.ensureGraphQL()
	return graphqlSchemaOf(m.graphql.projects, path)
}

func (m *Model) ensureGraphQLDiscovered(done func()) {
	m.ensureGraphQL()
	if m.graphql.discovered {
		if done != nil {
			done()
		}
		return
	}
	if done != nil && len(m.graphql.waiters) < 64 {
		m.graphql.waiters = append(m.graphql.waiters, done)
	}
	if !m.graphql.discovering {
		m.discoverGraphQLSchemas()
	}
}

func (m *Model) discoverGraphQLSchemas() {
	m.ensureGraphQL()
	root := m.Root()
	if root == "" {
		m.graphql.discovered = true
		m.graphql.projects = nil
		m.graphql.discoveryErr = ""
		return
	}
	m.graphql.generation++
	generation := m.graphql.generation
	m.graphql.discovering = true
	m.graphql.discoveryErr = ""
	m.invalidate()
	go func() {
		files, configs, err := scanGraphQLProject(root)
		var projects []graphqlSchemaProject
		if err == nil {
			projects = graphqlDiscoverSchemas(root, configs, files)
		}
		m.post(func() {
			if m.graphql.root != root || m.graphql.generation != generation {
				return
			}
			m.graphql.discovering = false
			if err != nil {
				m.graphql.discoveryErr = "Cannot discover GraphQL schemas: " + err.Error()
			} else {
				m.graphql.discovered = true
				m.graphql.projects = projects
				m.publishGraphQLDiagnostics(projects)
			}
			waiters := m.graphql.waiters
			m.graphql.waiters = nil
			for _, done := range waiters {
				done()
			}
			m.invalidate()
		})
	}()
}

func scanGraphQLProject(root string) ([]graphqlSchemaFile, []graphqlConfiguredSchema, error) {
	paths := []string{}
	for _, relative := range workspace.Index(root) {
		if graphqlIndexPathExcluded(relative) {
			continue
		}
		name := filepath.Base(relative)
		if graphqlIsSchemaFile(name) || graphqlIsConfigFile(name) {
			paths = append(paths, graphqlSlash(filepath.Join(root, filepath.FromSlash(relative))))
			if len(paths) >= graphqlFileLimit {
				break
			}
		}
	}
	sort.Strings(paths)
	files := []graphqlSchemaFile{}
	configs := []graphqlConfiguredSchema{}
	schemaReads := 0
	for _, path := range paths {
		name := filepath.Base(path)
		text := ""
		if graphqlIsConfigFile(name) || schemaReads < graphqlReadLimit {
			if raw, readErr := os.ReadFile(filepath.FromSlash(path)); readErr == nil {
				text = string(raw)
			}
		}
		if graphqlIsConfigFile(name) {
			configs = append(configs, graphqlConfiguredSchema{
				Path: path, Config: graphqlParseConfig(name, text),
			})
			continue
		}
		schemaReads++
		stats, defined, references := graphqlAnalyzeSchema(text)
		files = append(files, graphqlSchemaFile{
			Path: path, Stats: stats, Defined: defined, References: references,
		})
	}
	return files, configs, nil
}

func graphqlIndexPathExcluded(relative string) bool {
	parts := strings.Split(filepath.ToSlash(relative), "/")
	for _, part := range parts[:max(0, len(parts)-1)] {
		if excludedDir(part) {
			return true
		}
	}
	return false
}

func (m *Model) publishGraphQLDiagnostics(projects []graphqlSchemaProject) {
	byPath := map[string][]openidea.Diagnostic{}
	for _, project := range projects {
		for _, problem := range graphqlUnresolvedTypes(project) {
			byPath[problem.Path] = append(byPath[problem.Path], problem.Diagnostic)
		}
	}
	for path := range m.graphql.diagnosticPaths {
		if _, ok := byPath[path]; !ok {
			m.applyDiagnostics(openidea.DiagnosticsEvent{Path: path, Source: "graphql-schema"})
		}
	}
	for path, diagnostics := range byPath {
		m.applyDiagnostics(openidea.DiagnosticsEvent{
			Path: path, Source: "graphql-schema", Items: diagnostics,
		})
	}
	next := make(map[string]bool, len(byPath))
	for path := range byPath {
		next[path] = true
	}
	m.graphql.diagnosticPaths = next
}

func (m *Model) revalidateGraphQLSchema(path, text string) bool {
	path = graphqlSlash(realPath(path))
	for projectIndex := range m.graphql.projects {
		project := &m.graphql.projects[projectIndex]
		for fileIndex := range project.Files {
			file := &project.Files[fileIndex]
			if graphqlSlash(realPath(file.Path)) != path {
				continue
			}
			stats, defined, references := graphqlAnalyzeSchema(text)
			file.Stats = stats
			file.Defined = defined
			file.References = references
			m.publishGraphQLDiagnostics(m.graphql.projects)
			m.invalidate()
			return true
		}
	}
	return false
}

func (m *Model) graphqlDiagnosticsFor(path string) []openidea.Diagnostic {
	per := m.diags[realPath(path)]
	out := []openidea.Diagnostic{}
	for source, diagnostics := range per {
		if source == "graphql-schema" {
			continue
		}
		out = append(out, diagnostics...)
	}
	return out
}

// AddGraphQLConfiguration opens the existing config or creates graphql.config.yml.
func (m *Model) AddGraphQLConfiguration() {
	m.ensureGraphQLDiscovered(func() {
		if config := m.graphqlConfigurationFor(""); config != "" {
			m.openFile(config)
			return
		}
		root := m.Root()
		if root == "" {
			return
		}
		path := filepath.Join(root, graphqlConfigNames[0])
		file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
		if err != nil && !os.IsExist(err) {
			m.notifyError("graphql-config", "Cannot create the GraphQL configuration", err.Error())
			return
		}
		if err == nil {
			_, writeErr := file.WriteString(graphqlConfigTemplate)
			closeErr := file.Close()
			if writeErr != nil {
				_ = os.Remove(path)
				m.notifyError("graphql-config", "Cannot create the GraphQL configuration", writeErr.Error())
				return
			}
			if closeErr != nil {
				_ = os.Remove(path)
				m.notifyError("graphql-config", "Cannot create the GraphQL configuration", closeErr.Error())
				return
			}
		}
		m.openFile(path)
		m.graphql.discovered = false
		m.discoverGraphQLSchemas()
	})
}

func (m *Model) OpenGraphQLConfiguration() {
	path := ""
	if tab := m.tab(); tab != nil && tab.doc != nil {
		path = tab.doc.Path()
	}
	m.ensureGraphQLDiscovered(func() {
		if config := m.graphqlConfigurationFor(path); config != "" {
			m.openFile(config)
			return
		}
		m.status = "No graphql-config file was found"
		m.invalidate()
	})
}

func (m *Model) RestartGraphQLDiscovery() {
	m.ensureGraphQL()
	m.graphql.discovered = false
	m.discoverGraphQLSchemas()
}

func (m *Model) graphqlEndpointFor(path string) *graphqlEndpoint {
	project := m.graphqlSchemaFor(path)
	if project == nil || len(project.Endpoints) == 0 {
		return nil
	}
	chosen := m.graphql.endpointBySchema[project.Key]
	for index := range project.Endpoints {
		if project.Endpoints[index].Name == chosen {
			return &project.Endpoints[index]
		}
	}
	return &project.Endpoints[0]
}

func (m *Model) chooseGraphQLEndpoint(path, name string) {
	project := m.graphqlSchemaFor(path)
	if project == nil {
		return
	}
	m.graphql.endpointBySchema[project.Key] = name
	m.invalidate()
}

func (m *Model) toggleGraphQLVariables(path string) {
	m.ensureGraphQL()
	if m.graphql.variablesShown[path] {
		delete(m.graphql.variablesShown, path)
	} else {
		m.graphql.variablesShown[path] = true
	}
	m.invalidate()
}

func (m *Model) setGraphQLVariables(path, text string) {
	m.ensureGraphQL()
	m.graphql.variables[path] = text
	m.graphql.variablesDraftPath = path
	m.graphql.variablesDraft = text
	m.invalidate()
}

func (m *Model) graphqlEnvironment(configPath string) map[string]string {
	out := map[string]string{}
	if m.prefs.GraphQL.Environment == nil {
		return out
	}
	for name, value := range m.prefs.GraphQL.Environment[configPath] {
		out[name] = value
	}
	return out
}

func (m *Model) saveGraphQLEnvironment(configPath string, values map[string]string) {
	if configPath == "" {
		return
	}
	if m.prefs.GraphQL.Environment == nil {
		m.prefs.GraphQL.Environment = map[string]map[string]string{}
	}
	clean := map[string]string{}
	for name, value := range values {
		clean[name] = value
	}
	m.prefs.GraphQL.Environment[configPath] = clean
	m.savePreferences()
}

func (m *Model) EditGraphQLEnvironment(path string) {
	m.ensureGraphQLDiscovered(func() {
		project := m.graphqlSchemaFor(path)
		if project == nil || project.ConfigPath == "" || len(project.Variables) == 0 {
			return
		}
		m.openGraphQLEnvironment(project.ConfigPath, project.Variables, nil)
	})
}

func (m *Model) openGraphQLEnvironment(configPath string, variables []string, done func(bool)) {
	names := append([]string(nil), variables...)
	sort.Strings(names)
	values := m.graphqlEnvironment(configPath)
	m.graphql.environmentDialog = &graphqlEnvironmentDialog{
		ConfigPath: configPath, Variables: names, Values: values, done: done,
	}
	m.invalidate()
}

func (m *Model) closeGraphQLEnvironment(ok bool) {
	dialog := m.graphql.environmentDialog
	if dialog == nil {
		return
	}
	m.graphql.environmentDialog = nil
	if ok {
		m.saveGraphQLEnvironment(dialog.ConfigPath, dialog.Values)
	}
	if dialog.done != nil {
		dialog.done(ok)
	}
	m.invalidate()
}

func (m *Model) ensureGraphQLEnvironment(project *graphqlSchemaProject, endpoint graphqlEndpoint, done func(bool)) {
	if project == nil || project.ConfigPath == "" {
		done(true)
		return
	}
	values := m.graphqlEnvironment(project.ConfigPath)
	if len(graphqlMissingVariables(endpoint, values)) == 0 {
		done(true)
		return
	}
	m.openGraphQLEnvironment(project.ConfigPath, project.Variables, done)
}

func (m *Model) resolveGraphQLEndpoint(project *graphqlSchemaProject, endpoint graphqlEndpoint) graphqlEndpoint {
	if project == nil || project.ConfigPath == "" {
		return endpoint
	}
	return graphqlExpandEndpoint(endpoint, m.graphqlEnvironment(project.ConfigPath))
}

func (m *Model) RunGraphQLAtCaret() {
	tab := m.tab()
	if tab == nil || tab.doc == nil || tab.state == nil ||
		!strings.EqualFold(tab.doc.Info().Language, "graphql") {
		m.status = "Open a GraphQL file to execute a query"
		m.invalidate()
		return
	}
	selection := tab.state.Selection()
	from, to := int(selection.Start()), int(selection.End())
	if from == to {
		from = int(selection.Head)
		to = from
	}
	context := graphqlQueryContext(tab.doc.Text(), &graphqlRange{From: from, To: to})
	if strings.TrimSpace(context.Text) == "" {
		m.status = "There is no GraphQL query at the caret"
		m.invalidate()
		return
	}
	m.markGraphQLSent(tab, context.Ranges)
	m.RunGraphQL(tab.doc.Path(), context.Text)
}

func (m *Model) RunGraphQL(path, query string) {
	if strings.TrimSpace(query) == "" {
		return
	}
	m.ensureGraphQLDiscovered(func() {
		project := m.graphqlSchemaFor(path)
		endpoint := m.graphqlEndpointFor(path)
		if project == nil || endpoint == nil {
			m.notifyInfo(
				"graphql-run", "No GraphQL endpoint",
				"Declare one under extensions.endpoints in graphql.config.yml.",
			)
			return
		}
		configured := *endpoint
		m.ensureGraphQLEnvironment(project, configured, func(ok bool) {
			if !ok {
				return
			}
			variables, err := graphqlParseVariables(m.graphql.variables[path])
			if err != nil {
				m.notifyError("graphql-run", "Invalid GraphQL variables", err.Error())
				return
			}
			m.sendGraphQLQuery(path, project, configured, query, variables, false)
		})
	})
}

func (m *Model) sendGraphQLQuery(
	path string,
	project *graphqlSchemaProject,
	configured graphqlEndpoint,
	query string,
	variables map[string]any,
	introspection bool,
) {
	if m.invokeHook == nil && m.mgr == nil {
		m.graphql.result = &graphqlQueryResult{
			Path: path, Endpoint: configured, Error: "The GoIDEA backend is not connected",
		}
		m.showGraphQLResult()
		return
	}
	if m.graphql.running != nil && m.graphql.running.cancel != nil {
		m.graphql.running.cancel()
	}
	request := m.nextRequest()
	ctx, cancel := context.WithCancel(m.ctx)
	run := &graphqlRun{request: request, cancel: cancel}
	m.graphql.running = run
	m.invalidate()
	endpoint := m.resolveGraphQLEndpoint(project, configured)
	text := graphqlHTTPRequestText(endpoint, query, variables)
	go func() {
		raw, err := m.invoke(ctx, "http_execute", map[string]any{
			"request": request,
			"path":    nil,
			"text":    text,
			"index":   uint32(0),
			"env":     nil,
		})
		cancel()
		var exchange openidea.HTTPExchange
		if err == nil {
			err = json.Unmarshal(raw, &exchange)
		}
		m.post(func() {
			if m.graphql.running == nil || m.graphql.running.request != request {
				return
			}
			m.graphql.running = nil
			result := &graphqlQueryResult{Path: path, Endpoint: configured}
			if err != nil {
				result.Error = err.Error()
			} else {
				if exchange.Entry.Error != nil {
					result.Error = *exchange.Entry.Error
				}
				if exchange.Response != nil {
					result.Status = exchange.Response.Status
					result.HasStatus = true
					result.TotalMs = exchange.Response.TotalMs
					if exchange.Response.Body != nil {
						result.Body = *exchange.Response.Body
					}
					result.Size = int(exchange.Response.Size)
				} else if exchange.Entry.TotalMs != nil {
					result.TotalMs = *exchange.Entry.TotalMs
				}
			}
			if result.Error != "" && !isCancelledError(err) {
				retry := func(model *Model) {
					if introspection {
						model.introspectGraphQLEndpoint(path, configured.Name)
					} else {
						model.RunGraphQL(path, query)
					}
				}
				showError := func(model *Model) {
					model.openGraphQLErrorDialog("GraphQL error", result.Error)
				}
				m.notifyWarning(
					"graphql-run", "GraphQL error",
					fmt.Sprintf("GraphQL request to %s failed.", endpoint.URL),
					notifyAction{label: "Retry", run: retry},
					notifyAction{label: "Show error", run: showError},
				)
			}
			if introspection && err == nil && result.Error == "" {
				m.finishGraphQLIntrospection(project, configured, result.Body)
				return
			}
			m.graphql.result = result
			m.showGraphQLResult()
		})
	}()
}

func (m *Model) cancelGraphQLRun() {
	if m.graphql.running == nil {
		return
	}
	run := m.graphql.running
	run.cancel()
	m.graphql.running = nil
	go func() {
		ctx, cancel := context.WithTimeout(m.ctx, 10*time.Second)
		defer cancel()
		_, _ = m.invoke(ctx, "cancel_request", map[string]any{"id": run.request})
	}()
	m.invalidate()
}

func (m *Model) IntrospectGraphQL(path string) {
	m.introspectGraphQLEndpoint(path, "")
}

func (m *Model) introspectGraphQLEndpoint(path, name string) {
	m.ensureGraphQLDiscovered(func() {
		project := m.graphqlSchemaFor(path)
		if project == nil || len(project.Endpoints) == 0 {
			m.notifyInfo(
				"graphql-run", "No GraphQL endpoint",
				"Declare one under extensions.endpoints in graphql.config.yml.",
			)
			return
		}
		endpoint := project.Endpoints[0]
		if name != "" {
			for _, candidate := range project.Endpoints {
				if candidate.Name == name {
					endpoint = candidate
					break
				}
			}
		} else if chosen := m.graphqlEndpointFor(path); chosen != nil {
			endpoint = *chosen
		}
		m.graphql.lastIntrospection = &graphqlLastIntrospection{Path: path, Endpoint: endpoint.Name}
		m.ensureGraphQLEnvironment(project, endpoint, func(ok bool) {
			if ok {
				m.sendGraphQLQuery(
					path, project, endpoint, graphqlIntrospectionQuery, nil, true,
				)
			}
		})
	})
}

func (m *Model) RerunGraphQLIntrospection(path string) {
	name := ""
	if project := m.graphqlSchemaFor(path); project != nil && m.graphql.lastIntrospection != nil {
		lastProject := graphqlSchemaOf(m.graphql.projects, m.graphql.lastIntrospection.Path)
		if lastProject != nil && lastProject.Key == project.Key {
			name = m.graphql.lastIntrospection.Endpoint
		}
	}
	m.introspectGraphQLEndpoint(path, name)
}

func (m *Model) finishGraphQLIntrospection(
	project *graphqlSchemaProject,
	endpoint graphqlEndpoint,
	body string,
) {
	sdl, err := graphqlIntrospectionToSDL(body)
	if err != nil {
		m.notifyError("graphql-introspect", "Cannot read the GraphQL schema", err.Error())
		return
	}
	target := graphqlIntrospectionTarget(*project)
	current, readErr := os.ReadFile(target)
	if readErr == nil {
		if !strings.HasPrefix(string(current), graphqlGeneratedHeader) {
			m.graphql.introspectionAsk = &graphqlIntrospectionConfirmation{
				Target: target, SDL: sdl, Endpoint: endpoint,
			}
			m.invalidate()
			return
		}
		m.writeGraphQLIntrospection(target, sdl, endpoint)
		return
	} else if os.IsNotExist(readErr) {
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			m.notifyError("graphql-introspect", "Cannot write the GraphQL schema", err.Error())
			return
		}
		if err := os.WriteFile(target, []byte(sdl), 0o644); err != nil {
			m.notifyError("graphql-introspect", "Cannot write the GraphQL schema", err.Error())
			return
		}
		m.openFile(target)
	} else {
		m.notifyError("graphql-introspect", "Cannot write the GraphQL schema", readErr.Error())
		return
	}
	m.notifyInfo(
		"graphql-introspect", "GraphQL schema updated",
		filepath.Base(target)+" ← "+endpoint.URL,
	)
	m.RestartGraphQLDiscovery()
}

func (m *Model) writeGraphQLIntrospection(
	target, sdl string,
	endpoint graphqlEndpoint,
) {
	m.openWith(target, openSpec{
		at: -1,
		done: func(tab *Tab) {
			if err := replaceEditorText(tab, sdl); err != nil {
				m.notifyError("graphql-introspect", "Cannot write the GraphQL schema", err.Error())
				return
			}
			m.finishGraphQLIntrospectionWrite(target, endpoint)
		},
		failed: func(_ string, err error) {
			m.notifyError("graphql-introspect", "Cannot write the GraphQL schema", err.Error())
		},
	})
}

func (m *Model) finishGraphQLIntrospectionWrite(target string, endpoint graphqlEndpoint) {
	m.notifyInfo(
		"graphql-introspect", "GraphQL schema updated",
		filepath.Base(target)+" ← "+endpoint.URL,
	)
	m.RestartGraphQLDiscovery()
}

func (m *Model) confirmGraphQLIntrospection() {
	confirmation := m.graphql.introspectionAsk
	if confirmation == nil {
		return
	}
	m.graphql.introspectionAsk = nil
	m.writeGraphQLIntrospection(confirmation.Target, confirmation.SDL, confirmation.Endpoint)
	m.invalidate()
}

func (m *Model) cancelGraphQLIntrospection() {
	m.graphql.introspectionAsk = nil
	m.invalidate()
}

func replaceEditorText(tab *Tab, text string) error {
	if tab == nil || tab.state == nil || tab.state.Buffer == nil {
		return fmt.Errorf("the editor is not available")
	}
	tab.state.Buffer.Set(text)
	return nil
}

func (m *Model) OpenGraphQLIntrospectionSchema() {
	path := ""
	if tab := m.tab(); tab != nil && tab.doc != nil {
		path = tab.doc.Path()
	}
	m.ensureGraphQLDiscovered(func() {
		project := m.graphqlSchemaFor(path)
		if project == nil {
			return
		}
		target := graphqlIntrospectionTarget(*project)
		if _, err := os.Stat(target); err != nil {
			m.notifyInfo(
				"graphql-introspect", "No introspection schema yet",
				"Run Introspection Query on an endpoint to generate it.",
			)
			return
		}
		m.openFile(target)
	})
}

func graphqlNodeLabel(node graphqlSchemaNode) string {
	switch node.Kind {
	case "project":
		if node.Project.ConfigPath != "" {
			return filepath.Base(node.Project.Root)
		}
		return "Default project-wide schema"
	case "summary":
		return "Schema discovery summary"
	case "file":
		return filepath.Base(node.File.Path)
	case "remote":
		return node.Remote
	case "errors":
		return "Schema errors"
	case "error":
		if node.Diagnostic != nil {
			return node.Diagnostic.Message
		}
	case "noErrors":
		return "No errors found"
	case "endpoints":
		return "Endpoints"
	case "endpoint":
		if node.Endpoint != nil {
			return node.Endpoint.Name
		}
	case "noEndpoints":
		if node.Project.ConfigPath != "" {
			return "No endpoints available in this schema"
		}
		return "No endpoints available in the default schema"
	}
	return node.Kind
}

func graphqlNodeIcon(node graphqlSchemaNode) string {
	switch node.Kind {
	case "file":
		return "file-graphql"
	case "remote":
		return "general/web"
	case "errors":
		return "nodes/folder"
	case "error":
		if node.Diagnostic != nil && node.Diagnostic.Severity == "warning" {
			return "status/warning"
		}
		return "status/error"
	case "noErrors":
		return "status/check"
	case "endpoints":
		return "nodes/folder"
	case "noEndpoints":
		return "status/info"
	}
	return "general/file"
}

func graphqlStatsSummary(project graphqlSchemaProject) string {
	if project.Dynamic {
		return "the configuration is code and is not evaluated"
	}
	if len(project.Files) == 0 && len(project.Remote) == 0 {
		return "schema is empty"
	}
	stats := graphqlSchemaStatsOf(project)
	return fmt.Sprintf(
		"%d types, %d inputs, %d enums, %d directives",
		stats.Types, stats.Inputs, stats.Enums, stats.Directives,
	)
}
