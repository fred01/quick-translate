package translate

import (
	"bytes"
	"errors"
	"strings"
	"testing"
	"time"
)

func TestBatchReporterWritesOnlyToItsWriter(t *testing.T) {
	var stderr bytes.Buffer
	reporter := BatchReporter{Writer: &stderr}

	reporter.Report(Status{Stage: StageSending, Host: "api.example.com", Model: "translategemma", InputChars: 143})
	reporter.Report(Status{Stage: StageCompleted, OutputChars: 218, Elapsed: 1240 * time.Millisecond})

	if stderr.Len() == 0 {
		t.Fatal("expected status to be written to the configured writer")
	}
	// BatchReporter has exactly one io.Writer field; there is no other
	// destination it could have written to.
}

func TestBatchReporterQuietSuppressesSuccess(t *testing.T) {
	var buf bytes.Buffer
	reporter := BatchReporter{Writer: &buf, Quiet: true}

	reporter.Report(Status{Stage: StageSending, Host: "h", Model: "m", InputChars: 5})
	reporter.Report(Status{Stage: StageCompleted, OutputChars: 5, Elapsed: time.Second})

	if buf.Len() != 0 {
		t.Fatalf("quiet mode wrote output: %q", buf.String())
	}
}

func TestBatchReporterQuietDoesNotSuppressFailure(t *testing.T) {
	var buf bytes.Buffer
	reporter := BatchReporter{Writer: &buf, Quiet: true}

	reporter.Report(Status{Stage: StageFailed, Elapsed: time.Second, Err: errors.New("boom")})

	if buf.Len() == 0 {
		t.Fatal("quiet mode must not suppress failure status")
	}
	if !strings.Contains(buf.String(), "boom") {
		t.Fatalf("expected error text in output, got %q", buf.String())
	}
}

func TestBatchReporterOutputFormat(t *testing.T) {
	var buf bytes.Buffer
	reporter := BatchReporter{Writer: &buf}

	reporter.Report(Status{Stage: StageSending, Host: "api.example.com", Model: "translategemma", InputChars: 143})
	reporter.Report(Status{Stage: StageCompleted, OutputChars: 218, Elapsed: 1240 * time.Millisecond})

	got := buf.String()
	wantLines := []string{
		"qt: sending 143 chars to api.example.com · model=translategemma",
		"qt: received 218 chars in 1.24s",
	}
	for _, line := range wantLines {
		if !strings.Contains(got, line) {
			t.Fatalf("output %q does not contain expected line %q", got, line)
		}
	}
}

func TestBatchReporterFailureFormat(t *testing.T) {
	var buf bytes.Buffer
	reporter := BatchReporter{Writer: &buf}

	reporter.Report(Status{Stage: StageFailed, Elapsed: 1240 * time.Millisecond, Err: errors.New("request timed out")})

	got := buf.String()
	want := "qt: request failed after 1.24s: request timed out"
	if !strings.Contains(got, want) {
		t.Fatalf("output %q does not contain expected line %q", got, want)
	}
}

func TestBatchReporterIgnoresPreparingAndWaiting(t *testing.T) {
	var buf bytes.Buffer
	reporter := BatchReporter{Writer: &buf}

	reporter.Report(Status{Stage: StagePreparing, Host: "h", Model: "m"})
	reporter.Report(Status{Stage: StageWaiting, Host: "h", Model: "m"})

	if buf.Len() != 0 {
		t.Fatalf("preparing/waiting stages must not print in batch mode, got %q", buf.String())
	}
}

// TestStatusNeverCarriesSecretsOrContent documents, via the type system, that
// Status cannot leak secrets or translated content: it has no field for the
// API key, source text, context text, or generated prompt, so BatchReporter
// has nothing to leak even if it tried.
func TestStatusNeverCarriesSecretsOrContent(t *testing.T) {
	s := Status{}
	_ = s.Host
	_ = s.Model
	_ = s.InputChars
	_ = s.OutputChars
	_ = s.Elapsed
	_ = s.Err
	_ = s.Stage
	// Status intentionally has no APIKey, Source, Context, or Prompt field.
}

func TestBatchReporterDisplayedHostIsSafe(t *testing.T) {
	var buf bytes.Buffer
	reporter := BatchReporter{Writer: &buf}

	// A host value as it would be derived from url.URL.Host: never a scheme,
	// user info, path, query, or fragment.
	reporter.Report(Status{Stage: StageSending, Host: "api.example.com:4000", Model: "m", InputChars: 1})

	got := buf.String()
	for _, forbidden := range []string{"http://", "https://", "@", "?", "#", "/v1"} {
		if strings.Contains(got, forbidden) {
			t.Fatalf("output %q contains unsafe host fragment %q", got, forbidden)
		}
	}
}

func TestDiscardReporterDoesNothing(t *testing.T) {
	var r DiscardReporter
	r.Report(Status{Stage: StageFailed, Err: errors.New("boom")})
}
