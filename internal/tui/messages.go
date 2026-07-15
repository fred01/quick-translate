package tui

import (
	"context"

	tea "charm.land/bubbletea/v2"

	"github.com/fred01/quick-translate/internal/config"
	"github.com/fred01/quick-translate/internal/setup"
	"github.com/fred01/quick-translate/internal/translate"
)

// statusMsg carries a non-terminal progress update (preparing, sending, or
// waiting) from an in-flight translation back into the update loop. seq
// identifies which submission it belongs to, so a stale message from a
// cancelled or superseded request can be ignored.
type statusMsg struct {
	seq    int
	status translate.Status
}

// resultMsg carries the outcome of a translation request: the translated
// text on success, or a safe error on failure.
type resultMsg struct {
	seq     int
	text    string
	err     error
	elapsed float64 // seconds
}

// setupResultMsg carries the outcome of an in-TUI profile test-and-save,
// tagged with seq so a stale attempt is ignored.
type setupResultMsg struct {
	seq         int
	name        string
	cfg         config.Config
	translation string
	err         error
}

// programReporter forwards non-terminal translate.Status reports into the
// running Bubble Tea program as statusMsg values. Terminal stages
// (completed/failed) are delivered instead through the translation
// command's own return value, which also carries the translated text.
type programReporter struct {
	program *tea.Program
	seq     int
}

// Report implements translate.Reporter.
func (r *programReporter) Report(s translate.Status) {
	if r.program == nil {
		return
	}
	switch s.Stage {
	case translate.StageCompleted, translate.StageFailed:
		return
	}
	r.program.Send(statusMsg{seq: r.seq, status: s})
}

// setupTestCmd tests candidate through the real production translation path
// and, on success, upserts it into the store under name and marks it active,
// reporting the outcome as a setupResultMsg.
func setupTestCmd(
	ctx context.Context,
	configPath string,
	name string,
	candidate config.Config,
	newTranslator func(config.Config) (translate.Translator, error),
	seq int,
) tea.Cmd {
	return func() tea.Msg {
		result, err := setup.TestAndSave(ctx, configPath, name, candidate, newTranslator, translate.DiscardReporter{})
		if err != nil {
			return setupResultMsg{seq: seq, name: name, err: err}
		}
		return setupResultMsg{seq: seq, name: result.Name, cfg: result.Config, translation: result.Translation}
	}
}
