// Package integration contains end-to-end tests that exercise a real Cobra
// command tree, the real config and translate packages, and a fake
// OpenAI-compatible HTTP server together.
package integration

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/fred01/quick-translate/internal/app"
	"github.com/fred01/quick-translate/internal/cli"
	"github.com/fred01/quick-translate/internal/config"
	"github.com/fred01/quick-translate/internal/translate"
)

// fakeChatServer returns an httptest.Server that answers any Chat
// Completions request with content, and records the raw request bodies it
// received.
func fakeChatServer(t *testing.T, content string) (*httptest.Server, *[]string) {
	t.Helper()
	var bodies []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		data, _ := io.ReadAll(r.Body)
		bodies = append(bodies, string(data))
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"id": "chatcmpl-test", "object": "chat.completion", "created": 0, "model": "test-model",
			"choices": []map[string]any{
				{"index": 0, "finish_reason": "stop", "message": map[string]any{"role": "assistant", "content": content}},
			},
		})
	}))
	t.Cleanup(server.Close)
	return server, &bodies
}

func baseDeps(t *testing.T, server *httptest.Server) (app.Dependencies, *bytes.Buffer, *bytes.Buffer) {
	t.Helper()
	var stdout, stderr bytes.Buffer

	env := map[string]string{
		config.EnvBaseURL: server.URL + "/v1",
		config.EnvModel:   "test-model",
		config.EnvAPIKey:  "sk-integration-test",
	}

	deps := app.Dependencies{
		Stdin:  strings.NewReader(""),
		Stdout: &stdout,
		Stderr: &stderr,

		ConfigPath: filepath.Join(t.TempDir(), "config.json"),
		Getenv:     func(k string) string { return env[k] },

		StdinIsTerminal:  func() bool { return false },
		StdoutIsTerminal: func() bool { return false },

		NewTranslator: translate.NewTranslator,
	}
	return deps, &stdout, &stderr
}

func TestEndToEndPipedStdin(t *testing.T) {
	server, bodies := fakeChatServer(t, "Yes, I finished it yesterday.")
	deps, stdout, stderr := baseDeps(t, server)
	deps.Stdin = strings.NewReader("Да, закончил вчера")

	root := cli.NewRootCommand(deps)
	root.SetArgs([]string{})

	if err := root.Execute(); err != nil {
		t.Fatalf("Execute() error: %v", err)
	}

	if got, want := stdout.String(), "Yes, I finished it yesterday.\n"; got != want {
		t.Fatalf("stdout = %q, want exactly %q", got, want)
	}

	stderrText := stderr.String()
	if !strings.Contains(stderrText, "sending") || !strings.Contains(stderrText, "received") {
		t.Fatalf("stderr = %q, want it to mention sending and receiving", stderrText)
	}
	if strings.Contains(stderrText, "sk-integration-test") {
		t.Fatal("stderr must never contain the API key")
	}

	if len(*bodies) != 1 {
		t.Fatalf("server received %d requests, want 1", len(*bodies))
	}
	if !strings.Contains((*bodies)[0], "Да, закончил вчера") {
		t.Fatalf("request body did not contain the source text: %s", (*bodies)[0])
	}
}

func TestEndToEndTextFlagWithContext(t *testing.T) {
	server, bodies := fakeChatServer(t, "Yes, I finished it yesterday.")
	deps, stdout, _ := baseDeps(t, server)

	root := cli.NewRootCommand(deps)
	root.SetArgs([]string{"--context", "Did you finish the report?", "--text", "Да, закончил вчера"})

	if err := root.Execute(); err != nil {
		t.Fatalf("Execute() error: %v", err)
	}
	if got, want := stdout.String(), "Yes, I finished it yesterday.\n"; got != want {
		t.Fatalf("stdout = %q, want exactly %q", got, want)
	}

	body := (*bodies)[0]
	if !strings.Contains(body, "BEGIN REFERENCE CONTEXT") || !strings.Contains(body, "Did you finish the report?") {
		t.Fatalf("request body did not contain the reference context: %s", body)
	}
}

func TestEndToEndQuietSuppressesSuccessStatus(t *testing.T) {
	server, _ := fakeChatServer(t, "translated")
	deps, stdout, stderr := baseDeps(t, server)

	root := cli.NewRootCommand(deps)
	root.SetArgs([]string{"--text", "hello", "--quiet"})

	if err := root.Execute(); err != nil {
		t.Fatalf("Execute() error: %v", err)
	}
	if got, want := stdout.String(), "translated\n"; got != want {
		t.Fatalf("stdout = %q, want %q", got, want)
	}
	if stderr.Len() != 0 {
		t.Fatalf("stderr = %q, want empty under --quiet on success", stderr.String())
	}
}

func TestEndToEndTranslateSubcommandSameBehavior(t *testing.T) {
	server, _ := fakeChatServer(t, "translated")
	deps, stdout, _ := baseDeps(t, server)

	root := cli.NewRootCommand(deps)
	root.SetArgs([]string{"translate", "--text", "hello"})

	if err := root.Execute(); err != nil {
		t.Fatalf("Execute() error: %v", err)
	}
	if got, want := stdout.String(), "translated\n"; got != want {
		t.Fatalf("stdout = %q, want %q", got, want)
	}
}

func TestEndToEndSetupAddSavesProfile(t *testing.T) {
	server, bodies := fakeChatServer(t, "Hello!")
	deps, stdout, stderr := baseDeps(t, server)

	root := cli.NewRootCommand(deps)
	root.SetArgs([]string{
		"setup", "add", "litellm",
		"--base-url", server.URL + "/v1",
		"--model", "test-model",
		"--api-key", "sk-integration-test",
	})

	if err := root.Execute(); err != nil {
		t.Fatalf("Execute() error: %v", err)
	}
	if stdout.Len() != 0 {
		t.Fatalf("stdout = %q, want empty", stdout.String())
	}
	if !strings.Contains(stderr.String(), "Hello!") {
		t.Fatalf("stderr = %q, want it to mention the test translation", stderr.String())
	}
	if len(*bodies) != 1 {
		t.Fatalf("server received %d requests, want 1", len(*bodies))
	}
	if !strings.Contains((*bodies)[0], "Привет!") {
		t.Fatalf("setup must use the fixed test source: %s", (*bodies)[0])
	}

	store, err := config.LoadStore(deps.ConfigPath)
	if err != nil {
		t.Fatalf("LoadStore() error: %v", err)
	}
	if store.Active != "litellm" {
		t.Fatalf("active = %q, want litellm", store.Active)
	}
	want := config.Config{BaseURL: server.URL + "/v1", Model: "test-model", APIKey: "sk-integration-test"}
	if store.Profiles["litellm"] != want {
		t.Fatalf("saved profile = %+v, want %+v", store.Profiles["litellm"], want)
	}
}

func TestEndToEndProfilesLifecycle(t *testing.T) {
	server, _ := fakeChatServer(t, "ok")
	deps, stdout, _ := baseDeps(t, server)

	run := func(args ...string) {
		t.Helper()
		stdout.Reset()
		root := cli.NewRootCommand(deps)
		root.SetArgs(args)
		if err := root.Execute(); err != nil {
			t.Fatalf("Execute(%v) error: %v", args, err)
		}
	}

	// Add two profiles (each tested + saved + activated on add).
	run("setup", "add", "litellm", "--base-url", server.URL+"/v1", "--model", "gemma", "--api-key", "lk")
	run("setup", "add", "nvidia", "--base-url", server.URL+"/v1", "--model", "llama", "--api-key", "nk")

	// nvidia was added last, so it is active. Switch back to litellm.
	run("setup", "use", "litellm")
	store, _ := config.LoadStore(deps.ConfigPath)
	if store.Active != "litellm" {
		t.Fatalf("active = %q, want litellm", store.Active)
	}

	// A one-off translation against the nvidia profile via --profile must not
	// change the active profile.
	run("--profile", "nvidia", "--text", "hello")
	store, _ = config.LoadStore(deps.ConfigPath)
	if store.Active != "litellm" {
		t.Fatalf("--profile must not change the active profile; active = %q", store.Active)
	}

	// list shows both, marking litellm active.
	run("setup", "list")
	if out := stdout.String(); !strings.Contains(out, "* litellm") || !strings.Contains(out, "nvidia") {
		t.Fatalf("list output = %q", out)
	}

	// Remove the non-active profile.
	run("setup", "remove", "nvidia")
	store, _ = config.LoadStore(deps.ConfigPath)
	if _, ok := store.Profiles["nvidia"]; ok {
		t.Fatal("nvidia should have been removed")
	}
}

func TestEndToEndMigratesFlatConfig(t *testing.T) {
	server, _ := fakeChatServer(t, "translated")
	deps, stdout, _ := baseDeps(t, server)

	// Write an old-style flat config and clear the env so it is actually used.
	flat := `{"base_url":"` + server.URL + `/v1","model":"legacy-model","api_key":"legacy-key"}`
	if err := os.WriteFile(deps.ConfigPath, []byte(flat), 0o600); err != nil {
		t.Fatalf("WriteFile() error: %v", err)
	}
	deps.Getenv = func(string) string { return "" }

	root := cli.NewRootCommand(deps)
	root.SetArgs([]string{"--text", "hello"})
	if err := root.Execute(); err != nil {
		t.Fatalf("Execute() error: %v", err)
	}
	if got := stdout.String(); got != "translated\n" {
		t.Fatalf("stdout = %q, want %q", got, "translated\n")
	}
}

func TestEndToEndFailedRequestExitsNonZeroWithSafeStderr(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		json.NewEncoder(w).Encode(map[string]any{
			"error": map[string]any{"message": "invalid api key: sk-integration-test", "type": "invalid_request_error"},
		})
	}))
	t.Cleanup(server.Close)

	deps, stdout, stderr := baseDeps(t, server)

	root := cli.NewRootCommand(deps)
	root.SetArgs([]string{"--text", "hello"})

	err := root.Execute()
	if err == nil {
		t.Fatal("Execute() = nil error, want error")
	}
	if cli.ExitCode(err) != 1 {
		t.Fatalf("ExitCode = %d, want 1", cli.ExitCode(err))
	}
	if stdout.Len() != 0 {
		t.Fatalf("stdout = %q, want empty on failure", stdout.String())
	}
	if !strings.Contains(stderr.String(), "request failed") {
		t.Fatalf("stderr = %q, want it to mention the request failure", stderr.String())
	}
	if strings.Contains(stderr.String(), "sk-integration-test") {
		t.Fatal("stderr must never contain the API key")
	}
}
