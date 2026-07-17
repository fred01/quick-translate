package tui

import (
	"context"

	tea "charm.land/bubbletea/v2"

	"github.com/fred01/quick-translate/internal/config"
	"github.com/fred01/quick-translate/internal/prompt"
	"github.com/fred01/quick-translate/internal/setup"
	"github.com/fred01/quick-translate/internal/translate"
)

// resultMsg carries the outcome of one multi-tone translation request: a map
// of tone to translated text on success, or a request-level error. A requested
// tone the model omitted is simply absent from results.
type resultMsg struct {
	seq     int
	results map[prompt.Tone]string
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
