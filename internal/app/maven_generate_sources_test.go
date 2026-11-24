package app

import (
	"context"
	"encoding/json"
	"sync/atomic"
	"testing"
	"time"

	"mayoi/internal/openidea"
)

func generateSourcesModel(t *testing.T) (*Model, *runChainHook, *atomic.Int32) {
	t.Helper()
	hook := &runChainHook{hold: true}
	m := newRunChainModel(t, hook)
	hookInvoke := hook.hook
	var syncs atomic.Int32
	m.invokeHook = func(ctx context.Context, command string, args any) (json.RawMessage, error) {
		if command != "build_sync" {
			return hookInvoke(ctx, command, args)
		}
		syncs.Add(1)
		return mustBuildToolJSON(t, buildModel{
			Maven: &mavenModel{
				Projects: m.maven.model.MavenProjects(),
				SyncedAt: floatPointer(1),
			},
		}), nil
	}
	return m, hook, &syncs
}

func waitForGenerateRun(t *testing.T, m *Model, hook *runChainHook, started int) uint32 {
	t.Helper()
	waitRunChain(t, 3*time.Second, func() bool {
		var id uint32
		m.post(func() { id = m.run.runID })
		hook.mu.Lock()
		defer hook.mu.Unlock()
		return len(hook.started) == started && id != 0
	})
	var id uint32
	m.post(func() { id = m.run.runID })
	return id
}

func finishGenerateRun(t *testing.T, m *Model, id uint32, code int32) {
	t.Helper()
	m.post(func() {
		m.acceptExit(openidea.RunExitedEvent{Run: id, Code: &code})
	})
}

func TestGenerateMavenSourcesRunsEveryTopLevelProjectThenReloads(t *testing.T) {
	m, hook, syncs := generateSourcesModel(t)
	projectRoot := "/p"
	m.maven.model.Maven = &mavenModel{Projects: []mavenProject{
		{Pom: "pom.xml", Dir: "", DisplayName: "root", Command: "mvn", ProjectRoot: &projectRoot},
		{Pom: "aggregator/pom.xml", Dir: "aggregator", DisplayName: "aggregator", Command: "mvn", Aggregated: true},
		{Pom: "app/pom.xml", Dir: "app", DisplayName: "app", Command: "mvn", ProjectRoot: &projectRoot},
	}}

	m.GenerateMavenSources("")
	first := waitForGenerateRun(t, m, hook, 1)
	finishGenerateRun(t, m, first, 0)
	second := waitForGenerateRun(t, m, hook, 2)
	finishGenerateRun(t, m, second, 1)

	waitRunChain(t, 3*time.Second, func() bool { return syncs.Load() == 1 })
	hook.mu.Lock()
	defer hook.mu.Unlock()
	if len(hook.configs) != 2 {
		t.Fatalf("generate-sources configurations = %+v", hook.configs)
	}
	if hook.configs[0].Cwd != nil {
		t.Fatalf("root working directory = %#v", hook.configs[0].Cwd)
	}
	if hook.configs[1].Cwd == nil || *hook.configs[1].Cwd != "app" {
		t.Fatalf("module working directory = %#v", hook.configs[1].Cwd)
	}
	for _, config := range hook.configs {
		if len(config.Args) == 0 || config.Args[0] != "generate-sources" {
			t.Fatalf("goals = %v", config.Args)
		}
	}
}

func TestGenerateMavenSourcesSelectedPOMRunsOnceAndReloads(t *testing.T) {
	m, hook, syncs := generateSourcesModel(t)
	m.maven.model.Maven = &mavenModel{Projects: []mavenProject{
		{Pom: "pom.xml", DisplayName: "root", Command: "mvn"},
		{Pom: "app/pom.xml", Dir: "app", DisplayName: "app", Command: "mvn"},
	}}

	m.GenerateMavenSources("app/pom.xml")
	id := waitForGenerateRun(t, m, hook, 1)
	finishGenerateRun(t, m, id, 0)
	waitRunChain(t, 3*time.Second, func() bool { return syncs.Load() == 1 })
	hook.mu.Lock()
	defer hook.mu.Unlock()
	if len(hook.configs) != 1 ||
		hook.configs[0].Cwd == nil ||
		*hook.configs[0].Cwd != "app" {
		t.Fatalf("selected generate-sources configurations = %+v", hook.configs)
	}
}

func TestGenerateMavenSourcesDoesNotReloadWhenEveryProjectFails(t *testing.T) {
	m, hook, syncs := generateSourcesModel(t)
	m.maven.model.Maven = &mavenModel{Projects: []mavenProject{
		{Pom: "one/pom.xml", Dir: "one", DisplayName: "one", Command: "mvn"},
		{Pom: "two/pom.xml", Dir: "two", DisplayName: "two", Command: "mvn"},
	}}

	m.GenerateMavenSources("")
	first := waitForGenerateRun(t, m, hook, 1)
	finishGenerateRun(t, m, first, 1)
	second := waitForGenerateRun(t, m, hook, 2)
	finishGenerateRun(t, m, second, 1)
	waitRunChain(t, 3*time.Second, func() bool {
		var busy bool
		m.post(func() { busy = m.run.busy || m.run.awaiting || m.run.running })
		return !busy
	})
	if got := syncs.Load(); got != 0 {
		t.Fatalf("Maven reloaded after %d successful projects; want none", got)
	}
}

func TestGenerateMavenSourcesStopsTheQueueWhenStopped(t *testing.T) {
	m, hook, syncs := generateSourcesModel(t)
	m.maven.model.Maven = &mavenModel{Projects: []mavenProject{
		{Pom: "one/pom.xml", Dir: "one", DisplayName: "one", Command: "mvn"},
		{Pom: "two/pom.xml", Dir: "two", DisplayName: "two", Command: "mvn"},
	}}

	m.GenerateMavenSources("")
	id := waitForGenerateRun(t, m, hook, 1)
	m.post(func() { m.markRunPluginStopped() })
	finishGenerateRun(t, m, id, 0)
	waitRunChain(t, 3*time.Second, func() bool {
		var busy bool
		m.post(func() { busy = m.run.busy || m.run.awaiting || m.run.running })
		return !busy
	})
	hook.mu.Lock()
	defer hook.mu.Unlock()
	if len(hook.started) != 1 {
		t.Fatalf("stopped Generate Sources started another project: %v", hook.started)
	}
	if got := syncs.Load(); got != 0 {
		t.Fatalf("stopped Generate Sources reloaded Maven %d times", got)
	}
}

func TestMavenProjectNameFallsBackFromDisplayName(t *testing.T) {
	project := mavenProject{DisplayName: "app", ArtifactID: "artifact", Pom: "pom.xml"}
	if got := mavenProjectName(project); got != "app" {
		t.Fatalf("display name = %q", got)
	}
	project.DisplayName = ""
	if got := mavenProjectName(project); got != "artifact" {
		t.Fatalf("artifact fallback = %q", got)
	}
	project.ArtifactID = ""
	if got := mavenProjectName(project); got != "pom.xml" {
		t.Fatalf("POM fallback = %q", got)
	}
}
