package app

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"
)

func taskByID(m *Model, id string) (BackgroundTask, bool) {
	for _, task := range m.tasks {
		if task.ID == id {
			return task, true
		}
	}
	return BackgroundTask{}, false
}

func buildTestModel(t *testing.T, ctx context.Context) *Model {
	t.Helper()
	m := &Model{
		ctx:      ctx,
		postHook: func(fn func()) { fn() },
	}
	m.run.trusted = true
	m.setRoot(t.TempDir())
	return m
}

func TestBuildPublishesOneTaskAndFinishesIt(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	applied := make(chan struct{}, 4)
	var command string
	m := buildTestModel(t, ctx)
	m.postHook = func(fn func()) {
		fn()
		applied <- struct{}{}
	}
	m.invokeHook = func(_ context.Context, got string, _ any) (json.RawMessage, error) {
		command = got
		return json.RawMessage(`{"success":true,"durationMs":125,"errors":0,"warnings":0}`), nil
	}

	m.BuildProject()
	taskID := m.build.taskID
	if taskID == "" || len(m.tasks) != 1 || m.tasks[0].ID != taskID {
		t.Fatalf("build tasks = %+v, id = %q", m.tasks, taskID)
	}
	select {
	case <-applied:
	case <-time.After(2 * time.Second):
		t.Fatal("build did not finish")
	}
	task, ok := taskByID(m, taskID)
	if command != "build_run" || !ok || !task.Done || task.Fraction != 1 ||
		task.Message != "Build succeeded in 125 ms" {
		t.Fatalf("completed build = command %q task %+v", command, task)
	}
}

func TestBuildRequestFailureFinishesTheTask(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	applied := make(chan struct{}, 4)
	m := buildTestModel(t, ctx)
	m.postHook = func(fn func()) {
		fn()
		applied <- struct{}{}
	}
	m.invokeHook = func(_ context.Context, got string, _ any) (json.RawMessage, error) {
		if got != "build_compile_file" {
			t.Errorf("command = %q", got)
		}
		return nil, errors.New("compiler unavailable")
	}
	path := m.Root() + "/src/Main.java"
	m.startBuild("compileFile", &path)
	taskID := m.build.taskID
	select {
	case <-applied:
	case <-time.After(2 * time.Second):
		t.Fatal("failed build did not finish")
	}
	task, ok := taskByID(m, taskID)
	if !ok || !task.Done || task.Message != "Build request failed" {
		t.Fatalf("failed build task = %+v", task)
	}
}

func TestCancellingABuildRemovesItsTaskWhenTheRequestEnds(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	started := make(chan struct{})
	cancelCalled := make(chan map[string]any, 1)
	finished := make(chan struct{}, 4)
	m := buildTestModel(t, ctx)
	m.postHook = func(fn func()) {
		fn()
		finished <- struct{}{}
	}
	m.invokeHook = func(callCtx context.Context, command string, input any) (json.RawMessage, error) {
		switch command {
		case "build_run":
			close(started)
			<-callCtx.Done()
			return nil, callCtx.Err()
		case "cancel_request":
			args, _ := input.(map[string]any)
			cancelCalled <- args
			return json.RawMessage(`null`), nil
		default:
			return nil, errors.New("unexpected command " + command)
		}
	}
	m.BuildProject()
	taskID := m.build.taskID
	select {
	case <-started:
	case <-time.After(2 * time.Second):
		t.Fatal("build request did not start")
	}
	m.StopBuild()
	task, ok := taskByID(m, taskID)
	if !ok || !task.Cancelling {
		t.Fatalf("task after StopBuild = %+v", task)
	}
	select {
	case args := <-cancelCalled:
		if args["id"] != m.build.request {
			t.Fatalf("cancel_request id = %#v, want %d", args["id"], m.build.request)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("cancel_request was not sent")
	}
	cancel()
	select {
	case <-finished:
	case <-time.After(2 * time.Second):
		t.Fatal("cancelled build did not finish")
	}
	if _, ok := taskByID(m, taskID); ok {
		t.Fatalf("cancelled task still remains: %+v", m.tasks)
	}
}

func TestStaleBuildReplyOnlyRemovesItsOwnTask(t *testing.T) {
	m := &Model{
		tasks: []BackgroundTask{
			{ID: "build:1", Title: "Old", Done: false},
			{ID: "build:2", Title: "New", Done: false},
		},
		build: BuildPane{taskID: "build:2", channel: 22},
	}
	m.removeTask("build:1")
	if _, ok := taskByID(m, "build:1"); ok {
		t.Fatal("stale task was not removed")
	}
	task, ok := taskByID(m, "build:2")
	if !ok || task.Done || task.Title != "New" {
		t.Fatalf("new build task was changed by stale reply: %+v", task)
	}
}
