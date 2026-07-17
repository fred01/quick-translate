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
// NewOpenAIClient; the canonical default lives in config.DefaultTimeout.
const fallbackTimeout = 60 * time.Second

// TranslationInput is the material for one translation request.
type TranslationInput struct {
	Source  string
	Context string
	Tone    prompt.Tone
}

// Translator turns Russian source text into English, optionally informed by
// reference context, reporting progress through reporter. TranslateMulti
// renders several tones of the same source; it asks the model for all of them
// in a single request and splits the labeled response into per-tone results.
type Translator interface {
	Translate(ctx context.Context, input TranslationInput, reporter Reporter) (string, error)
	TranslateMulti(ctx context.Context, source, context string, tones []prompt.Tone, reporter Reporter) (map[prompt.Tone]string, error)
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

// Translate implements Translator for a single tone.
func (c *OpenAIClient) Translate(ctx context.Context, input TranslationInput, reporter Reporter) (string, error) {
	builtPrompt := prompt.Build(input.Source, input.Context, input.Tone)
	return c.complete(ctx, builtPrompt, len([]rune(input.Source)), reporter)
}

// TranslateMulti implements Translator. For a single tone it behaves like
// Translate. For several tones it sends one request that asks for all of them
// at once and splits the labeled response, so N variants cost one round-trip
// and one pass over the source rather than N. A tone the model omits is simply
// absent from the returned map.
func (c *OpenAIClient) TranslateMulti(ctx context.Context, source, context string, tones []prompt.Tone, reporter Reporter) (map[prompt.Tone]string, error) {
	if len(tones) == 0 {
		return nil, errors.New("no tones requested")
	}
	inputChars := len([]rune(source))
	if len(tones) == 1 {
		text, err := c.complete(ctx, prompt.Build(source, context, tones[0]), inputChars, reporter)
		if err != nil {
			return nil, err
		}
		return map[prompt.Tone]string{tones[0]: text}, nil
	}
	raw, err := c.complete(ctx, prompt.BuildMulti(source, context, tones), inputChars, reporter)
	if err != nil {
		return nil, err
	}
	return prompt.ParseMulti(raw, tones), nil
}

// complete sends one prompt to the model, reporting progress, and returns the
// trimmed response content.
func (c *OpenAIClient) complete(ctx context.Context, promptText string, inputChars int, reporter Reporter) (string, error) {
	reporter.Report(Status{Stage: StagePreparing, Host: c.host, Model: c.model})

	start := time.Now()
	reporter.Report(Status{Stage: StageSending, Host: c.host, Model: c.model, InputChars: inputChars})
	reporter.Report(Status{Stage: StageWaiting, Host: c.host, Model: c.model, InputChars: inputChars})

	resp, err := c.client.Chat.Completions.New(ctx, openai.ChatCompletionNewParams{
		Model:       c.model,
		Messages:    []openai.ChatCompletionMessageParamUnion{openai.UserMessage(promptText)},
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
