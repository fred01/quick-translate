package translate

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/fred01/quick-translate/internal/prompt"
)

// fakeReporter records every reported status for assertions.
type fakeReporter struct {
	statuses []Status
}

func (f *fakeReporter) Report(s Status) {
	f.statuses = append(f.statuses, s)
}

func (f *fakeReporter) stages() []Stage {
	stages := make([]Stage, len(f.statuses))
	for i, s := range f.statuses {
		stages[i] = s.Stage
	}
	return stages
}

type capturedRequest struct {
	Method        string
	Path          string
	Authorization string
	Body          chatCompletionRequestBody
}

type chatCompletionRequestBody struct {
	Model       string  `json:"model"`
	Temperature float64 `json:"temperature"`
	Messages    []struct {
		Role    string `json:"role"`
		Content string `json:"content"`
	} `json:"messages"`
}

func newFakeServer(t *testing.T, handler func(w http.ResponseWriter, r *http.Request, req capturedRequest)) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body chatCompletionRequestBody
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode request body: %v", err)
		}
		handler(w, r, capturedRequest{
			Method:        r.Method,
			Path:          r.URL.Path,
			Authorization: r.Header.Get("Authorization"),
			Body:          body,
		})
	}))
}

func writeChatCompletion(w http.ResponseWriter, content string) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"id":      "chatcmpl-test",
		"object":  "chat.completion",
		"created": 0,
		"model":   "test-model",
		"choices": []map[string]any{
			{
				"index":         0,
				"finish_reason": "stop",
				"message": map[string]any{
					"role":    "assistant",
					"content": content,
				},
			},
		},
	})
}

func TestTranslateRequestShape(t *testing.T) {
	var captured capturedRequest
	server := newFakeServer(t, func(w http.ResponseWriter, r *http.Request, req capturedRequest) {
		captured = req
		writeChatCompletion(w, "Yes, I finished it yesterday.")
	})
	defer server.Close()

	client, err := NewOpenAIClient(server.URL+"/v1", "sk-test-key", "translategemma")
	if err != nil {
		t.Fatalf("NewOpenAIClient() error: %v", err)
	}

	reporter := &fakeReporter{}
	input := TranslationInput{Source: "Да, закончил вчера", Context: "Did you finish the report?"}
	got, err := client.Translate(context.Background(), input, reporter)
	if err != nil {
		t.Fatalf("Translate() error: %v", err)
	}
	if got != "Yes, I finished it yesterday." {
		t.Fatalf("Translate() = %q", got)
	}

	if captured.Method != http.MethodPost {
		t.Fatalf("Method = %q, want POST", captured.Method)
	}
	if captured.Path != "/v1/chat/completions" {
		t.Fatalf("Path = %q, want /v1/chat/completions", captured.Path)
	}
	if captured.Authorization != "Bearer sk-test-key" {
		t.Fatalf("Authorization = %q, want Bearer sk-test-key", captured.Authorization)
	}
	if captured.Body.Model != "translategemma" {
		t.Fatalf("Model = %q, want translategemma", captured.Body.Model)
	}
	if captured.Body.Temperature != 0 {
		t.Fatalf("Temperature = %v, want 0", captured.Body.Temperature)
	}
	if len(captured.Body.Messages) != 1 {
		t.Fatalf("Messages = %d, want exactly 1", len(captured.Body.Messages))
	}
	if captured.Body.Messages[0].Role != "user" {
		t.Fatalf("Messages[0].Role = %q, want user", captured.Body.Messages[0].Role)
	}
	wantPrompt := prompt.Build(input.Source, input.Context, input.Tone)
	if captured.Body.Messages[0].Content != wantPrompt {
		t.Fatalf("Messages[0].Content mismatch\n--- got ---\n%s\n--- want ---\n%s", captured.Body.Messages[0].Content, wantPrompt)
	}
}

func TestTranslateReportsStagesOnSuccess(t *testing.T) {
	server := newFakeServer(t, func(w http.ResponseWriter, r *http.Request, req capturedRequest) {
		writeChatCompletion(w, "translated")
	})
	defer server.Close()

	client, err := NewOpenAIClient(server.URL+"/v1", "sk-test-key", "m")
	if err != nil {
		t.Fatalf("NewOpenAIClient() error: %v", err)
	}

	reporter := &fakeReporter{}
	_, err = client.Translate(context.Background(), TranslationInput{Source: "hello"}, reporter)
	if err != nil {
		t.Fatalf("Translate() error: %v", err)
	}

	want := []Stage{StagePreparing, StageSending, StageWaiting, StageCompleted}
	got := reporter.stages()
	if len(got) != len(want) {
		t.Fatalf("stages = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("stages = %v, want %v", got, want)
		}
	}
	final := reporter.statuses[len(reporter.statuses)-1]
	if final.OutputChars != len("translated") {
		t.Fatalf("OutputChars = %d, want %d", final.OutputChars, len("translated"))
	}
	if final.Host == "" {
		t.Fatal("Host must be populated")
	}
	if strings.Contains(final.Host, "@") {
		t.Fatalf("Host must not contain user info: %q", final.Host)
	}
}

func TestTranslateNoChoices(t *testing.T) {
	server := newFakeServer(t, func(w http.ResponseWriter, r *http.Request, req capturedRequest) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"id":      "chatcmpl-test",
			"object":  "chat.completion",
			"created": 0,
			"model":   "test-model",
			"choices": []map[string]any{},
		})
	})
	defer server.Close()

	client, err := NewOpenAIClient(server.URL+"/v1", "sk-test-key", "m")
	if err != nil {
		t.Fatalf("NewOpenAIClient() error: %v", err)
	}

	reporter := &fakeReporter{}
	_, err = client.Translate(context.Background(), TranslationInput{Source: "hello"}, reporter)
	if err == nil {
		t.Fatal("Translate() = nil error, want error for no choices")
	}
	assertSafeError(t, err, "sk-test-key")
}

func TestTranslateEmptyContent(t *testing.T) {
	server := newFakeServer(t, func(w http.ResponseWriter, r *http.Request, req capturedRequest) {
		writeChatCompletion(w, "   ")
	})
	defer server.Close()

	client, err := NewOpenAIClient(server.URL+"/v1", "sk-test-key", "m")
	if err != nil {
		t.Fatalf("NewOpenAIClient() error: %v", err)
	}

	_, err = client.Translate(context.Background(), TranslationInput{Source: "hello"}, &fakeReporter{})
	if err == nil {
		t.Fatal("Translate() = nil error, want error for empty content")
	}
	assertSafeError(t, err, "sk-test-key")
}

func TestTranslateTrimsSurroundingWhitespace(t *testing.T) {
	server := newFakeServer(t, func(w http.ResponseWriter, r *http.Request, req capturedRequest) {
		writeChatCompletion(w, "  translated text  \n")
	})
	defer server.Close()

	client, err := NewOpenAIClient(server.URL+"/v1", "sk-test-key", "m")
	if err != nil {
		t.Fatalf("NewOpenAIClient() error: %v", err)
	}

	got, err := client.Translate(context.Background(), TranslationInput{Source: "hello"}, &fakeReporter{})
	if err != nil {
		t.Fatalf("Translate() error: %v", err)
	}
	if got != "translated text" {
		t.Fatalf("Translate() = %q, want %q", got, "translated text")
	}
}

func TestTranslateNon2xxResponse(t *testing.T) {
	const secretSource = "very secret source text"
	const secretContext = "very secret context text"
	const secretKey = "sk-super-secret-key"

	server := newFakeServer(t, func(w http.ResponseWriter, r *http.Request, req capturedRequest) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		json.NewEncoder(w).Encode(map[string]any{
			"error": map[string]any{
				"message": "invalid api key: " + secretKey,
				"type":    "invalid_request_error",
				"code":    "invalid_api_key",
			},
		})
	})
	defer server.Close()

	client, err := NewOpenAIClient(server.URL+"/v1", secretKey, "m")
	if err != nil {
		t.Fatalf("NewOpenAIClient() error: %v", err)
	}

	_, err = client.Translate(context.Background(), TranslationInput{Source: secretSource, Context: secretContext}, &fakeReporter{})
	if err == nil {
		t.Fatal("Translate() = nil error, want error for non-2xx response")
	}
	assertSafeError(t, err, secretKey, secretSource, secretContext)
	if !strings.Contains(err.Error(), "401") {
		t.Fatalf("error = %q, want to mention status 401", err.Error())
	}
}

func TestTranslateMalformedResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte("{not valid json"))
	}))
	defer server.Close()

	client, err := NewOpenAIClient(server.URL+"/v1", "sk-test-key", "m")
	if err != nil {
		t.Fatalf("NewOpenAIClient() error: %v", err)
	}

	_, err = client.Translate(context.Background(), TranslationInput{Source: "hello"}, &fakeReporter{})
	if err == nil {
		t.Fatal("Translate() = nil error, want error for malformed response")
	}
	assertSafeError(t, err, "sk-test-key")
}

func TestTranslateTimeout(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(200 * time.Millisecond)
		writeChatCompletion(w, "too slow")
	}))
	defer server.Close()

	client, err := NewOpenAIClient(server.URL+"/v1", "sk-test-key", "m")
	if err != nil {
		t.Fatalf("NewOpenAIClient() error: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()

	reporter := &fakeReporter{}
	_, err = client.Translate(ctx, TranslationInput{Source: "hello"}, reporter)
	if err == nil {
		t.Fatal("Translate() = nil error, want timeout error")
	}
	if !strings.Contains(err.Error(), "timed out") {
		t.Fatalf("error = %q, want to mention timeout", err.Error())
	}
	assertSafeError(t, err, "sk-test-key")

	final := reporter.statuses[len(reporter.statuses)-1]
	if final.Stage != StageFailed {
		t.Fatalf("final stage = %v, want failed", final.Stage)
	}
}

func TestTranslateCancellation(t *testing.T) {
	release := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-release
		writeChatCompletion(w, "too late")
	}))
	defer server.Close()
	defer close(release)

	client, err := NewOpenAIClient(server.URL+"/v1", "sk-test-key", "m")
	if err != nil {
		t.Fatalf("NewOpenAIClient() error: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(20 * time.Millisecond)
		cancel()
	}()

	_, err = client.Translate(ctx, TranslationInput{Source: "hello"}, &fakeReporter{})
	if err == nil {
		t.Fatal("Translate() = nil error, want cancellation error")
	}
	if !strings.Contains(err.Error(), "canceled") {
		t.Fatalf("error = %q, want to mention cancellation", err.Error())
	}
	assertSafeError(t, err, "sk-test-key")
}

// assertSafeError verifies err never exposes any of the given secrets, and
// separately never exposes the words "Authorization" or "Bearer".
func assertSafeError(t *testing.T, err error, secrets ...string) {
	t.Helper()
	msg := err.Error()
	for _, secret := range secrets {
		if strings.Contains(msg, secret) {
			t.Fatalf("error %q leaks secret %q", msg, secret)
		}
	}
	if strings.Contains(msg, "Authorization") || strings.Contains(msg, "Bearer") {
		t.Fatalf("error %q leaks Authorization header", msg)
	}
}

func TestSafeHost(t *testing.T) {
	tests := []struct {
		raw  string
		want string
	}{
		{raw: "http://localhost:4000/v1", want: "localhost:4000"},
		{raw: "https://integrate.api.nvidia.com/v1", want: "integrate.api.nvidia.com"},
		{raw: "https://user:pass@host:1234/v1?x=y#z", want: "host:1234"},
		{raw: "not a url with spaces and \x7f control", want: ""},
	}
	for _, tt := range tests {
		if got := SafeHost(tt.raw); got != tt.want {
			t.Errorf("SafeHost(%q) = %q, want %q", tt.raw, got, tt.want)
		}
	}
}

func TestSanitizeErrorFallbackNeverLeaksRawError(t *testing.T) {
	raw := errors.New("dial tcp 10.0.0.1:443: some low-level detail with sk-leaked-secret")
	got := sanitizeError(raw)
	if strings.Contains(got.Error(), "sk-leaked-secret") {
		t.Fatalf("sanitizeError() = %q, must not leak raw error text", got.Error())
	}
}
