package app

import (
	"os"
	"path/filepath"
	"testing"

	"mayoi/internal/openidea"
)

func TestServiceRowsReadTheProject(t *testing.T) {
	dir := t.TempDir()
	write := func(name string) {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("pom.xml")
	m := &Model{root: dir}
	rows := m.serviceRows()
	has := func(label string) bool {
		for _, r := range rows {
			if r.label == label {
				return true
			}
		}
		return false
	}
	if !has("Maven") || has("Gradle") {
		t.Fatalf("the build rows are wrong: %+v", rows)
	}
	if !has("Java toolchain") || !has("Notifications") {
		t.Fatalf("the tools rows are missing: %+v", rows)
	}
	if has("No build system found") {
		t.Fatalf("a build system was reported missing")
	}

	write("build.gradle")
	rows = m.serviceRows()
	if !has("Gradle") {
		t.Fatalf("Gradle was not detected next to Maven: %+v", rows)
	}

	empty := (&Model{root: t.TempDir()}).serviceRows()
	found := false
	for _, r := range empty {
		if r.label == "No build system found" {
			found = true
		}
	}
	if !found {
		t.Fatalf("an empty project reported a build system: %+v", empty)
	}
}

func TestServiceRowsListRunConfigurations(t *testing.T) {
	m := &Model{}
	m.run.configs = append(m.run.configs, openideaRunConfig("App", "java"))
	if rows := m.serviceRows(); len(rows) == 0 || rows[0].section != "Build" {
		t.Fatalf("rows = %+v", rows)
	}
	found := false
	for _, r := range m.serviceRows() {
		if r.section == "Run" && r.label == "App" && r.run != nil {
			found = true
		}
	}
	if !found {
		t.Fatalf("the run configuration is not listed")
	}
}

func openideaRunConfig(name, kind string) openidea.RunConfig {
	return openidea.RunConfig{Name: name, Kind: kind}
}

func TestServicesShowsTheRunningProcessAndItsStop(t *testing.T) {
	m := &Model{}
	m.run.configs = []openidea.RunConfig{{Name: "App", Kind: "java", Detected: true}}
	m.run.sel = 0
	m.run.running = true
	rows := m.serviceRows()
	var stop *serviceRow
	seen := false
	for i := range rows {
		if rows[i].label == "App" {
			seen = true
			if rows[i].detail != "running" {
				t.Fatalf("the running row says %q", rows[i].detail)
			}
		}
		if rows[i].label == "Stop App" {
			stop = &rows[i]
		}
	}
	if !seen || stop == nil || stop.detail != "running" {
		t.Fatalf("the running process is not listed with its stop: %+v", rows)
	}
	stop.run(m)
	if m.run.err != "进程尚未启动" {
		t.Fatalf("the stop row did not reach StopRun: %q", m.run.err)
	}

	m.run.running = false
	code := int32(3)
	m.run.exitCode = &code
	for _, r := range m.serviceRows() {
		if r.label == "App" && r.detail != "exit 3" {
			t.Fatalf("the finished row says %q", r.detail)
		}
		if r.label == "Stop App" {
			t.Fatal("a stopped process still lists its stop")
		}
	}
}
