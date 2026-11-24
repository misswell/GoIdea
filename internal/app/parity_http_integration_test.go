package app

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"mayoi/internal/openidea"
)

func recordParityIntegrationEvidence(t *testing.T, keys ...string) {
	t.Helper()
	path := strings.TrimSpace(os.Getenv("GOIDEA_PARITY_REPORT"))
	if path == "" {
		return
	}
	evidence := map[string]map[string]bool{"featureParity": {}}
	if raw, err := os.ReadFile(path); err == nil {
		if len(strings.TrimSpace(string(raw))) > 0 {
			if err := json.Unmarshal(raw, &evidence); err != nil {
				t.Fatalf("read parity report: %v", err)
			}
			if evidence["featureParity"] == nil {
				evidence["featureParity"] = map[string]bool{}
			}
		}
	} else if !os.IsNotExist(err) {
		t.Fatalf("read parity report: %v", err)
	}
	for _, key := range keys {
		evidence["featureParity"][key] = true
	}
	raw, err := json.MarshalIndent(evidence, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	temp := fmt.Sprintf("%s.%d.tmp", path, os.Getpid())
	if err := os.WriteFile(temp, append(raw, '\n'), 0o644); err != nil {
		t.Fatalf("write parity report: %v", err)
	}
	if err := os.Rename(temp, path); err != nil {
		t.Fatalf("replace parity report: %v", err)
	}
}

func launchParityIntegrationServer(t *testing.T) (*openidea.Manager, string) {
	t.Helper()
	server := strings.TrimSpace(os.Getenv("GOIDEA_SERVER"))
	if server == "" {
		t.Skip("GOIDEA_SERVER is not set")
	}
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	manager := openidea.NewManager(openidea.LaunchConfig{
		ServerPath:     server,
		Home:           t.TempDir(),
		Log:            io.Discard,
		StartupTimeout: 60 * time.Second,
	})
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	if err := manager.Start(ctx); err != nil {
		t.Fatalf("start openidea-server: %v", err)
	}
	t.Cleanup(func() { _ = manager.Close() })
	if _, err := manager.Invoke(ctx, Window, "open_project", map[string]any{"path": root}); err != nil {
		t.Fatalf("open_project: %v", err)
	}
	if _, err := manager.Invoke(ctx, Window, "project_trust_set", map[string]any{
		"path": root, "trusted": true, "folder": false,
	}); err != nil {
		t.Fatalf("trust isolated project: %v", err)
	}
	deadline := time.Now().Add(60 * time.Second)
	for {
		raw, err := manager.Invoke(ctx, Window, "project_current", nil)
		if err == nil && strings.TrimSpace(string(raw)) != "null" {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("project never became current: %v (reply %s)", err, raw)
		}
		time.Sleep(50 * time.Millisecond)
	}
	return manager, root
}

func TestIntegrationGraphQLAndHTTPClientAgainstLocalFixtures(t *testing.T) {
	manager, root := launchParityIntegrationServer(t)

	t.Run("GraphQL", func(t *testing.T) {
		type capturedRequest struct {
			Method      string
			ContentType string
			Query       string
			Variables   map[string]any
		}
		captured := make(chan capturedRequest, 1)
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			var payload struct {
				Query     string         `json:"query"`
				Variables map[string]any `json:"variables"`
			}
			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
				t.Errorf("decode GraphQL request: %v", err)
				http.Error(w, "bad request", http.StatusBadRequest)
				return
			}
			captured <- capturedRequest{
				Method: r.Method, ContentType: r.Header.Get("Content-Type"),
				Query: payload.Query, Variables: payload.Variables,
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, `{"data":{"answer":42}}`)
		}))
		t.Cleanup(server.Close)

		model := &Model{
			ctx:      context.Background(),
			mgr:      manager,
			postHook: func(fn func()) { fn() },
		}
		model.setRoot(root)
		model.ensureGraphQL()
		queryPath := filepath.Join(root, "query.graphql")
		if err := os.WriteFile(queryPath, []byte("query Live { answer }\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		project := &graphqlSchemaProject{ConfigPath: "", Root: root}
		endpoint := graphqlEndpoint{
			Name: "local", URL: server.URL + "/graphql",
			Headers: map[string]string{"X-GoIDEA": "graphql"},
		}
		model.sendGraphQLQuery(
			queryPath, project, endpoint, "query Live { answer }",
			map[string]any{"code": "VN"}, false,
		)

		var request capturedRequest
		select {
		case request = <-captured:
		case <-time.After(20 * time.Second):
			t.Fatalf("the real backend did not send the GraphQL request: %+v", model.graphql.result)
		}
		if request.Method != http.MethodPost ||
			!strings.HasPrefix(request.ContentType, "application/json") ||
			request.Query != "query Live { answer }" ||
			request.Variables["code"] != "VN" {
			t.Fatalf("GraphQL request = %+v", request)
		}
		waitFor(t, "the real GraphQL response", func() bool {
			return model.graphql.result != nil
		})
		if model.graphql.result.Status != http.StatusOK ||
			!strings.Contains(model.graphql.result.Body, `"answer":42`) {
			t.Fatalf("GraphQL result = %+v", model.graphql.result)
		}
	})

	t.Run("HTTPClient", func(t *testing.T) {
		type capturedRequest struct {
			Method string
			Path   string
			Header string
			Body   string
		}
		captured := make(chan capturedRequest, 1)
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			body, _ := io.ReadAll(r.Body)
			captured <- capturedRequest{
				Method: r.Method, Path: r.URL.Path,
				Header: r.Header.Get("X-GoIDEA"), Body: string(body),
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, `{"live":true}`)
		}))
		t.Cleanup(server.Close)

		environment := map[string]any{
			"dev": map[string]any{"BASE": server.URL},
		}
		rawEnvironment, err := json.Marshal(environment)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(
			filepath.Join(root, "http-client.env.json"), rawEnvironment, 0o644,
		); err != nil {
			t.Fatal(err)
		}

		model := &Model{
			ctx:      context.Background(),
			mgr:      manager,
			postHook: func(fn func()) { fn() },
		}
		model.setRoot(root)
		model.resetHTTPClient()
		model.http.envByFile[filepath.Join(root, "requests.http")] = "dev"
		text := "POST {{BASE}}/echo\nX-GoIDEA: live\n\nlive-body\n"
		if !model.runHTTPRequest(filepath.Join(root, "requests.http"), text, 0) {
			t.Fatalf("the real backend did not complete the HTTP Client request: %+v", model.notify.items)
		}

		select {
		case request := <-captured:
			if request.Method != http.MethodPost || request.Path != "/echo" ||
				request.Header != "live" || !strings.Contains(request.Body, "live-body") {
				t.Fatalf("HTTP Client request = %+v", request)
			}
		case <-time.After(20 * time.Second):
			t.Fatal("the real backend did not send the HTTP Client request")
		}
		if model.http.exchange == nil || model.http.exchange.Response == nil ||
			model.http.exchange.Response.Status != http.StatusOK ||
			model.http.exchange.Response.Body == nil ||
			!strings.Contains(*model.http.exchange.Response.Body, `"live":true`) {
			t.Fatalf("HTTP Client exchange = %+v", model.http.exchange)
		}
		if len(model.http.history) != 1 {
			t.Fatalf("HTTP Client history = %+v", model.http.history)
		}
	})
	recordParityIntegrationEvidence(t, "graphqlRoundTrip", "httpClientRoundTrip")
}

func TestIntegrationDeploymentLocalFilesystemRoundTrip(t *testing.T) {
	manager, root := launchParityIntegrationServer(t)
	remoteRoot := t.TempDir()
	settings := openidea.DeploymentSettings{
		Servers: []openidea.DeploymentServer{{
			Kind: openidea.DeploymentLocal, ID: "local", Name: "Local",
			RootPath: remoteRoot,
			Mappings: []openidea.DeploymentMapping{{}},
		}},
		DefaultServer:      "local",
		AutoUpload:         openidea.AutoUploadNever,
		PreserveTimestamps: true,
	}
	encoded, err := json.Marshal(settings)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	if _, err := manager.Invoke(ctx, Window, "settings_set", map[string]any{
		"scope": "project", "key": "deployment", "value": string(encoded),
	}); err != nil {
		t.Fatalf("save deployment settings: %v", err)
	}

	localPath := filepath.Join(root, "site", "index.html")
	if err := os.MkdirAll(filepath.Dir(localPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(localPath, []byte("uploaded\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	var coverage []openidea.DeploymentCoverage
	if err := manager.InvokeInto(ctx, Window, "deploy_coverage", map[string]any{
		"paths": []string{localPath},
	}, &coverage); err != nil {
		t.Fatalf("deploy_coverage: %v", err)
	}
	if len(coverage) != 1 || coverage[0].Server != "local" ||
		len(coverage[0].Paths) != 1 {
		t.Fatalf("deployment coverage = %+v", coverage)
	}

	var upload openidea.TransferReport
	if err := manager.InvokeInto(ctx, Window, "deploy_upload", map[string]any{
		"paths": []string{localPath}, "server": "local", "request": uint32(7101),
	}, &upload); err != nil {
		t.Fatalf("deploy_upload: %v", err)
	}
	if upload.Files != 1 || len(upload.Errors) != 0 {
		t.Fatalf("upload report = %+v", upload)
	}
	remotePath := filepath.Join(remoteRoot, "site", "index.html")
	if remote, err := os.ReadFile(remotePath); err != nil || string(remote) != "uploaded\n" {
		t.Fatalf("uploaded remote file = %q, %v", remote, err)
	}

	if err := os.WriteFile(remotePath, []byte("remote-newer\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	var entries []openidea.SyncEntry
	if err := manager.InvokeInto(ctx, Window, "deploy_compare", map[string]any{
		"path": localPath, "server": "local", "request": uint32(7102),
	}, &entries); err != nil {
		t.Fatalf("deploy_compare: %v", err)
	}
	if len(entries) != 1 || entries[0].Status != openidea.SyncDifferent {
		t.Fatalf("deployment comparison = %+v", entries)
	}

	var download openidea.TransferReport
	if err := manager.InvokeInto(ctx, Window, "deploy_download", map[string]any{
		"paths": []string{localPath}, "server": "local", "request": uint32(7103),
	}, &download); err != nil {
		t.Fatalf("deploy_download: %v", err)
	}
	if download.Files != 1 || len(download.Errors) != 0 {
		t.Fatalf("download report = %+v", download)
	}
	if local, err := os.ReadFile(localPath); err != nil || string(local) != "remote-newer\n" {
		t.Fatalf("downloaded local file = %q, %v", local, err)
	}

	var deleted openidea.TransferReport
	if err := manager.InvokeInto(ctx, Window, "deploy_delete_remote", map[string]any{
		"paths": []string{localPath}, "server": "local",
	}, &deleted); err != nil {
		t.Fatalf("deploy_delete_remote: %v", err)
	}
	if deleted.Files != 1 || len(deleted.Errors) != 0 {
		t.Fatalf("delete report = %+v", deleted)
	}
	if _, err := os.Stat(remotePath); !os.IsNotExist(err) {
		t.Fatalf("remote file still exists after delete: %v", err)
	}
	recordParityIntegrationEvidence(t, "deploymentLocalRoundTrip")
}
