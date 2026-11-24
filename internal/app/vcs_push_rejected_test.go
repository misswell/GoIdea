package app

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"mayoi/internal/openidea"
)

func TestRejectedPushOpensMergeChoiceAndRetriesInOrder(t *testing.T) {
	var mu sync.Mutex
	var postMu sync.Mutex
	var calls []string
	done := make(chan struct{}, 1)
	var m *Model
	m = &Model{
		ctx: context.Background(),
		invokeHook: func(_ context.Context, command string, args any) (json.RawMessage, error) {
			switch command {
			case "vcs_branch_info":
				head, upstream := "main", "origin/main"
				return json.Marshal(openidea.BranchInfo{Head: &head, Upstream: &upstream})
			case "vcs_log":
				return json.Marshal([]openidea.Commit{{Parents: []string{"1", "2"}}})
			case "vcs_remote":
				op := args.(map[string]any)["op"].(map[string]any)
				kind := op["op"].(string)
				if kind == "pull" {
					if op["rebase"] == true {
						kind = "pull:rebase"
					} else {
						kind = "pull:merge"
					}
				}
				mu.Lock()
				calls = append(calls, kind)
				mu.Unlock()
				return json.RawMessage(`null`), nil
			case "vcs_roots", "vcs_status":
				return json.RawMessage(`[]`), nil
			default:
				t.Fatalf("command = %q", command)
				return nil, nil
			}
		},
		postHook: func(fn func()) {
			postMu.Lock()
			defer postMu.Unlock()
			fn()
			if !m.vcsOps.busy && m.vcsOps.pushRejected == nil {
				select {
				case done <- struct{}{}:
				default:
				}
			}
		},
	}
	m.setRoot(t.TempDir())
	m.vcsOps = VCSOperationPane{kind: vcsOpPush, open: true, request: 7, channel: 9, busy: true}
	push := map[string]any{"op": "push", "force_with_lease": false, "force": false}
	rejected := errors.New("! [rejected] main -> main (fetch first)")
	err, deferred := m.handleRejectedPush(
		context.Background(), m.Root(), 7, 9, m.Root(), "Push", push, rejected, false, "merge",
	)
	if err != nil || !deferred || m.vcsOps.pushRejected == nil || !m.vcsOps.pushRejected.merges {
		t.Fatalf("rejected push dialog = %+v, err=%v deferred=%v", m.vcsOps.pushRejected, err, deferred)
	}
	m.resolveVCSRejectedPush("merge")
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("rejected push retry did not finish")
	}
	mu.Lock()
	defer mu.Unlock()
	if !reflect.DeepEqual(calls, []string{"pull:merge", "push"}) {
		t.Fatalf("rejected push calls = %v", calls)
	}
	if m.vcsOps.pushRejected != nil || m.vcsOps.busy || m.vcsOps.err != "" {
		t.Fatalf("rejected push left state = %+v", m.vcsOps)
	}
}

func TestVCSRejectedPushStopsAfterTenPushes(t *testing.T) {
	pushes := 0
	m := &Model{invokeHook: func(_ context.Context, command string, args any) (json.RawMessage, error) {
		if command != "vcs_remote" {
			t.Fatalf("command = %q", command)
		}
		op := args.(map[string]any)["op"].(map[string]any)
		if op["op"] == "push" {
			pushes++
			return nil, errors.New("! [rejected] main -> main (non-fast-forward)")
		}
		return json.RawMessage(`null`), nil
	}}
	err := m.retryVCSRemotePush(
		context.Background(),
		"/repo",
		3,
		map[string]any{"op": "push", "force": false, "force_with_lease": false},
		"merge",
		1,
	)
	if err == nil || pushes != 9 || !strings.Contains(err.Error(), "after 10 attempts") {
		t.Fatalf("retry error=%v pushes=%d", err, pushes)
	}
}

func TestPushRejectionClassificationIgnoresForcedPushes(t *testing.T) {
	rejected := errors.New("! [rejected] main -> main (non-fast-forward)")
	if !vcsPushWasRejected(rejected) {
		t.Fatal("non-fast-forward rejection was not recognized")
	}
	if vcsPushCanUpdate(map[string]any{"force_with_lease": true}, rejected) {
		t.Fatal("force-with-lease rejection was treated as an updateable Push")
	}
	if vcsPushWasRejected(errors.New("remote rejected by a pre-receive hook")) {
		t.Fatal("a hook refusal was mistaken for a non-fast-forward Push")
	}
	if !vcsPushLeaseRejected(errors.New("! [rejected] main -> main (stale info)")) {
		t.Fatal("stale force-with-lease rejection was not recognized")
	}
}

func TestPushRejectedRememberPersistsRuntimeChoices(t *testing.T) {
	written := make(chan string, 2)
	m := &Model{
		ctx: context.Background(),
		cfg: Config{SettingsPath: t.TempDir() + "/settings.json"},
		invokeHook: func(_ context.Context, command string, args any) (json.RawMessage, error) {
			if command != "settings_set" {
				t.Fatalf("command = %q", command)
			}
			request := args.(map[string]any)
			written <- request["key"].(string) + "=" + request["value"].(string)
			return json.RawMessage(`null`), nil
		},
	}
	m.setRoot(t.TempDir())
	m.nativeSettings.pages = map[string]nativeSettingsPageState{}
	m.saveVCSGitChoice("vcs.git.autoUpdateOnPushRejected", true)
	m.saveVCSGitChoice("vcs.git.updateMethod", "rebase")
	if !m.vcsGitBool("vcs.git.autoUpdateOnPushRejected") ||
		m.vcsGitUpdateMethod() != "rebase" {
		t.Fatal("remembered Push choices did not take effect immediately")
	}
	got := map[string]bool{}
	for len(got) < 2 {
		select {
		case write := <-written:
			got[write] = true
		case <-time.After(2 * time.Second):
			t.Fatalf("settings writes = %v", got)
		}
	}
	if !got[`vcs.git.autoUpdateOnPushRejected=true`] ||
		!got[`vcs.git.updateMethod="rebase"`] {
		t.Fatalf("settings writes = %v", got)
	}
}
