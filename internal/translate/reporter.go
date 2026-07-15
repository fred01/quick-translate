// Package translate performs Russian-to-English translation against an
// OpenAI-compatible Chat Completions endpoint and reports operational status.
package translate

import (
	"fmt"
	"io"
	"time"
)

// Stage identifies a point in the lifecycle of a translation request.
type Stage int

const (
	StagePreparing Stage = iota
	StageSending
	StageWaiting
	StageCompleted
	StageFailed
)

func (s Stage) String() string {
	switch s {
	case StagePreparing:
		return "preparing"
	case StageSending:
		return "sending"
	case StageWaiting:
		return "waiting"
	case StageCompleted:
		return "completed"
	case StageFailed:
		return "failed"
	default:
		return "unknown"
	}
}

// Status is a snapshot of translation progress, safe to display: it never
// carries the API key, source text, context text, or generated prompt.
type Status struct {
	Stage       Stage
	Host        string
	Model       string
	InputChars  int
	OutputChars int
	Elapsed     time.Duration
	Err         error
}

// Reporter receives status updates as a translation request progresses.
type Reporter interface {
	Report(Status)
}

// BatchReporter writes concise operational status for non-interactive
// invocations to a single writer (intended to be stderr). Quiet suppresses
// successful status lines; it never suppresses failures.
type BatchReporter struct {
	Writer io.Writer
	Quiet  bool
}

// Report implements Reporter.
func (r BatchReporter) Report(s Status) {
	switch s.Stage {
	case StageSending:
		if r.Quiet {
			return
		}
		fmt.Fprintf(r.Writer, "qt: sending %d chars to %s · model=%s\n", s.InputChars, s.Host, s.Model)
	case StageCompleted:
		if r.Quiet {
			return
		}
		fmt.Fprintf(r.Writer, "qt: received %d chars in %s\n", s.OutputChars, formatElapsed(s.Elapsed))
	case StageFailed:
		fmt.Fprintf(r.Writer, "qt: request failed after %s: %s\n", formatElapsed(s.Elapsed), s.Err)
	}
}

func formatElapsed(d time.Duration) string {
	return fmt.Sprintf("%.2fs", d.Seconds())
}

// DiscardReporter ignores all status reports. It is used where progress is
// already surfaced another way, such as inside a form's own spinner.
type DiscardReporter struct{}

// Report implements Reporter.
func (DiscardReporter) Report(Status) {}
