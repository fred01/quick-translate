package translate

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"

	"github.com/fred01/quick-translate/internal/prompt"
)

// fallbackTimeout guards against a non-positive timeout being passed to
// NewOpenAIClient; the canonical default lives in config.DefaultTimeout. The
// interactive UI runs one tone at a time, so this bound applies to a single
// request that never contends with another for the same GPU.
const fallbackTimeout = 180 * time.Second

// PromptKind selects which prompt a request is built with.
type PromptKind int

const (
	// PromptProse is ordinary text: one passage in, one passage out. It is the
	// zero value, so every existing caller keeps its exact behaviour.
	PromptProse PromptKind = iota
	// PromptScript is a numbered batch of narration lines, translated under the
	// rules that keep the numbering - and therefore the timing - intact.
	PromptScript
)

// TranslationInput is the material for one translation request.
type TranslationInput struct {
	Source  string
	Context string
	Tone    prompt.Tone
	Kind    PromptKind
}

func (i TranslationInput) buildPrompt() string {
	if i.Kind == PromptScript {
		return prompt.BuildScript(i.Source, i.Context, i.Tone)
	}
	return prompt.Build(i.Source, i.Context, i.Tone)
}

// Translator turns Russian source text into English, optionally informed by
// reference context, reporting progress through reporter.
type Translator interface {
	Translate(ctx context.Context, input TranslationInput, reporter Reporter) (string, error)
}

// OpenAIClient is a Translator backed by an OpenAI-compatible Chat
// Completions endpoint, using the official OpenAI Go SDK.
type OpenAIClient struct {
	client openai.Client
	model  string
	host   string
}

// NewOpenAIClient builds a Translator against baseURL (an OpenAI-compatible
// API root including "/v1") using apiKey and model. timeout bounds each
// request; a non-positive value falls back to fallbackTimeout.
func NewOpenAIClient(baseURL, apiKey, model string, timeout time.Duration) (*OpenAIClient, error) {
	host := SafeHost(baseURL)
	if host == "" {
		return nil, fmt.Errorf("invalid base URL: %q", baseURL)
	}
	if timeout <= 0 {
		timeout = fallbackTimeout
	}

	httpClient := &http.Client{Timeout: timeout}
	client := openai.NewClient(
		option.WithBaseURL(baseURL),
		option.WithAPIKey(apiKey),
		option.WithMaxRetries(0),
		option.WithHTTPClient(httpClient),
	)

	return &OpenAIClient{client: client, model: model, host: host}, nil
}

// SafeHost extracts just the host component (host:port, no scheme, user
// info, path, query, or fragment) from a base URL, suitable for display in
// status output.
func SafeHost(baseURL string) string {
	u, err := url.Parse(baseURL)
	if err != nil {
		return ""
	}
	return u.Host
}

// Translate implements Translator.
func (c *OpenAIClient) Translate(ctx context.Context, input TranslationInput, reporter Reporter) (string, error) {
	reporter.Report(Status{Stage: StagePreparing, Host: c.host, Model: c.model})

	builtPrompt := input.buildPrompt()
	inputChars := len([]rune(input.Source))

	start := time.Now()
	reporter.Report(Status{Stage: StageSending, Host: c.host, Model: c.model, InputChars: inputChars})
	reporter.Report(Status{Stage: StageWaiting, Host: c.host, Model: c.model, InputChars: inputChars})

	resp, err := c.client.Chat.Completions.New(ctx, openai.ChatCompletionNewParams{
		Model:       c.model,
		Messages:    []openai.ChatCompletionMessageParamUnion{openai.UserMessage(builtPrompt)},
		Temperature: openai.Float(0),
	})
	elapsed := time.Since(start)

	if err != nil {
		safeErr := sanitizeError(err)
		reporter.Report(Status{Stage: StageFailed, Host: c.host, Model: c.model, InputChars: inputChars, Elapsed: elapsed, Err: safeErr})
		return "", safeErr
	}

	if len(resp.Choices) == 0 {
		noChoicesErr := errors.New("no choices returned")
		reporter.Report(Status{Stage: StageFailed, Host: c.host, Model: c.model, InputChars: inputChars, Elapsed: elapsed, Err: noChoicesErr})
		return "", noChoicesErr
	}

	content := strings.TrimSpace(resp.Choices[0].Message.Content)
	if content == "" {
		emptyErr := errors.New("empty response content")
		reporter.Report(Status{Stage: StageFailed, Host: c.host, Model: c.model, InputChars: inputChars, Elapsed: elapsed, Err: emptyErr})
		return "", emptyErr
	}

	outputChars := len([]rune(content))
	reporter.Report(Status{Stage: StageCompleted, Host: c.host, Model: c.model, InputChars: inputChars, OutputChars: outputChars, Elapsed: elapsed})
	return content, nil
}

// sanitizeError converts an SDK or transport error into a safe error that
// never exposes the API key, Authorization header, source or context text,
// the generated prompt, or a request/response body.
func sanitizeError(err error) error {
	var apiErr *openai.Error
	if errors.As(err, &apiErr) {
		return fmt.Errorf("API request failed with status %d", apiErr.StatusCode)
	}
	if errors.Is(err, context.Canceled) {
		return errors.New("request canceled")
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return errors.New("request timed out")
	}
	return errors.New("request failed")
}
