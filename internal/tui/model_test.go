package tui

import (
	"context"
	"errors"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/fred01/quick-translate/internal/prompt"
	"github.com/fred01/quick-translate/internal/translate"
)

// fakeTranslator returns a canned response or error synchronously, so tests
// never touch the network or a real clock.
type fakeTranslator struct {
	response string
	err      error
	calls    int
}

func (f *fakeTranslator) Translate(_ context.Context, _ translate.TranslationInput, _ translate.Reporter) (string, error) {
	f.calls++
	if f.err != nil {
		return "", f.err
	}
	return f.response, nil
}

func (f *fakeTranslator) TranslateMulti(_ context.Context, _, _ string, tones []prompt.Tone, _ translate.Reporter) (map[prompt.Tone]string, error) {
	f.calls++
	if f.err != nil {
		return nil, f.err
	}
	out := make(map[prompt.Tone]string, len(tones))
	for _, t := range tones {
		out[t] = f.response
	}
	return out, nil
}

// collectMsgs runs cmd (and, recursively, any tea.BatchMsg it produces) and
// returns every resulting message. This stands in for the real Bubble Tea
// runtime, which is what normally unpacks batched commands.
func collectMsgs(cmd tea.Cmd) []tea.Msg {
	if cmd == nil {
		return nil
	}
	msg := cmd()
	if msg == nil {
		return nil
	}
	if batch, ok := msg.(tea.BatchMsg); ok {
		var out []tea.Msg
		for _, c := range batch {
			out = append(out, collectMsgs(c)...)
		}
		return out
	}
	return []tea.Msg{msg}
}

// findResultMsg returns the resultMsg among msgs, if any.
func findResultMsg(msgs []tea.Msg) (resultMsg, bool) {
	for _, msg := range msgs {
		if rm, ok := msg.(resultMsg); ok {
			return rm, true
		}
	}
	return resultMsg{}, false
}

func newTestModel(translator translate.Translator) model {
	return newModel(translator, "translategemma", "api.example.com")
}

func update(m model, msg tea.Msg) (model, tea.Cmd) {
	newM, cmd := m.Update(msg)
	return newM.(model), cmd
}

func TestSourceInitiallyFocused(t *testing.T) {
	m := newTestModel(&fakeTranslator{})
	if m.focus != focusSource {
		t.Fatalf("focus = %v, want focusSource", m.focus)
	}
	if !m.source.Focused() {
		t.Fatal("source textarea must be focused initially")
	}
	if m.context.Focused() {
		t.Fatal("context textarea must not be focused initially")
	}
}

func TestTabAndShiftTabChangeFields(t *testing.T) {
	m := newTestModel(&fakeTranslator{})

	m, _ = update(m, tea.KeyPressMsg{Code: tea.KeyTab})
	if m.focus != focusContext {
		t.Fatalf("after Tab, focus = %v, want focusContext", m.focus)
	}
	if !m.context.Focused() || m.source.Focused() {
		t.Fatal("after Tab, Context must be focused and Source must not")
	}

	m, _ = update(m, tea.KeyPressMsg{Code: tea.KeyTab, Mod: tea.ModShift})
	if m.focus != focusSource {
		t.Fatalf("after Shift+Tab, focus = %v, want focusSource", m.focus)
	}
	if !m.source.Focused() || m.context.Focused() {
		t.Fatal("after Shift+Tab, Source must be focused and Context must not")
	}
}

func TestEnterSubmits(t *testing.T) {
	fake := &fakeTranslator{response: "Hello!"}
	m := newTestModel(fake)
	m.source.SetValue("hello")

	m, cmd := update(m, tea.KeyPressMsg{Code: tea.KeyEnter})
	if !m.busy {
		t.Fatal("Enter with non-empty Source must start a request")
	}
	if !m.everSubmitted {
		t.Fatal("everSubmitted must be true after a submission")
	}
	if cmd == nil {
		t.Fatal("Enter must return a command to run the translation")
	}
}

func TestEnterDoesNotInsertNewline(t *testing.T) {
	m := newTestModel(&fakeTranslator{response: "x"})
	m.source.SetValue("hello")

	m, _ = update(m, tea.KeyPressMsg{Code: tea.KeyEnter})
	if m.source.Value() != "hello" {
		t.Fatalf("Source = %q, want unchanged %q (Enter must not insert a newline)", m.source.Value(), "hello")
	}
}

func TestShiftEnterInsertsNewline(t *testing.T) {
	m := newTestModel(&fakeTranslator{})
	m.source.SetValue("hello")

	m, cmd := update(m, tea.KeyPressMsg{Code: tea.KeyEnter, Mod: tea.ModShift})
	if cmd != nil {
		t.Fatal("Shift+Enter must not submit")
	}
	if m.source.Value() != "hello\n" {
		t.Fatalf("Source = %q, want %q", m.source.Value(), "hello\n")
	}
	if m.busy {
		t.Fatal("Shift+Enter must not start a request")
	}
}

func TestAltEnterInsertsNewline(t *testing.T) {
	m := newTestModel(&fakeTranslator{})
	m.source.SetValue("hello")

	m, cmd := update(m, tea.KeyPressMsg{Code: tea.KeyEnter, Mod: tea.ModAlt})
	if cmd != nil {
		t.Fatal("Alt+Enter must not submit")
	}
	if m.source.Value() != "hello\n" {
		t.Fatalf("Source = %q, want %q", m.source.Value(), "hello\n")
	}
}

func TestShiftEnterInsertsIntoFocusedContext(t *testing.T) {
	m := newTestModel(&fakeTranslator{})
	m, _ = update(m, tea.KeyPressMsg{Code: tea.KeyTab}) // focus Context
	m.context.SetValue("ctx")

	m, _ = update(m, tea.KeyPressMsg{Code: tea.KeyEnter, Mod: tea.ModShift})
	if m.context.Value() != "ctx\n" {
		t.Fatalf("Context = %q, want %q", m.context.Value(), "ctx\n")
	}
	if m.source.Value() != "" {
		t.Fatalf("Source = %q, want empty (newline must go to the focused field only)", m.source.Value())
	}
}

func TestEmptySourceDoesNotSubmit(t *testing.T) {
	for _, source := range []string{"", "   ", "\n\t "} {
		m := newTestModel(&fakeTranslator{})
		m.source.SetValue(source)

		m, cmd := update(m, tea.KeyPressMsg{Code: tea.KeyEnter})
		if cmd != nil {
			t.Fatalf("source %q: Enter returned a command, want nil", source)
		}
		if m.busy {
			t.Fatalf("source %q: busy = true, want false", source)
		}
		if m.everSubmitted {
			t.Fatalf("source %q: everSubmitted = true, want false", source)
		}
	}
}

func TestDuplicateSubmissionWhileBusyIsIgnored(t *testing.T) {
	fake := &fakeTranslator{response: "x"}
	m := newTestModel(fake)
	m.source.SetValue("hello")

	m, _ = update(m, tea.KeyPressMsg{Code: tea.KeyEnter})
	if !m.busy {
		t.Fatal("expected busy after first Enter")
	}
	seqAfterFirst := m.reqSeq
	statusBefore := m.statusText()

	m, cmd := update(m, tea.KeyPressMsg{Code: tea.KeyEnter})
	if cmd != nil {
		t.Fatal("duplicate submission while busy must return no command")
	}
	if m.reqSeq != seqAfterFirst {
		t.Fatalf("reqSeq changed on duplicate submission: got %d, want %d", m.reqSeq, seqAfterFirst)
	}
	if m.statusText() != statusBefore {
		t.Fatalf("status changed on duplicate submission: got %q, want %q", m.statusText(), statusBefore)
	}
}

func TestSuccessUpdatesTranslationAndStatus(t *testing.T) {
	fake := &fakeTranslator{response: "Yes, I finished it yesterday."}
	m := newTestModel(fake)
	m.source.SetValue("Да, закончил вчера")

	m, cmd := update(m, tea.KeyPressMsg{Code: tea.KeyEnter})
	msgs := collectMsgs(cmd)
	rm, ok := findResultMsg(msgs)
	if !ok {
		t.Fatal("expected a resultMsg from the submit command")
	}

	m, _ = update(m, rm)
	if m.busy {
		t.Fatal("busy must be false after a successful result")
	}
	if r := m.results[m.activeResult]; r.err != nil {
		t.Fatalf("active result err = %v, want nil", r.err)
	}
	if got := m.result.GetContent(); got != "Yes, I finished it yesterday." {
		t.Fatalf("Translation content = %q", got)
	}
	if got := m.results[m.activeResult].outputChars; got != len([]rune("Yes, I finished it yesterday.")) {
		t.Fatalf("active result outputChars = %d", got)
	}
}

func TestErrorPreservesInputs(t *testing.T) {
	fake := &fakeTranslator{err: errors.New("request timed out")}
	m := newTestModel(fake)
	m.source.SetValue("Да, закончил вчера")
	m.context.SetValue("some context")

	m, cmd := update(m, tea.KeyPressMsg{Code: tea.KeyEnter})
	msgs := collectMsgs(cmd)
	rm, ok := findResultMsg(msgs)
	if !ok {
		t.Fatal("expected a resultMsg from the submit command")
	}

	m, _ = update(m, rm)
	if m.busy {
		t.Fatal("busy must be false after a failed result")
	}
	if r := m.results[m.activeResult]; r.err == nil {
		t.Fatal("active result err must be set after a failed result")
	}
	if m.source.Value() != "Да, закончил вчера" {
		t.Fatalf("Source = %q, want preserved", m.source.Value())
	}
	if m.context.Value() != "some context" {
		t.Fatalf("Context = %q, want preserved", m.context.Value())
	}
}

func TestAnotherSubmissionAllowedAfterError(t *testing.T) {
	fake := &fakeTranslator{err: errors.New("boom")}
	m := newTestModel(fake)
	m.source.SetValue("hello")

	m, cmd := update(m, tea.KeyPressMsg{Code: tea.KeyEnter})
	rm, _ := findResultMsg(collectMsgs(cmd))
	m, _ = update(m, rm)

	m, cmd = update(m, tea.KeyPressMsg{Code: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("a new submission must be allowed after a failure")
	}
	if !m.busy {
		t.Fatal("busy must be true for the new submission")
	}
}

func TestEscCancelsActiveRequest(t *testing.T) {
	m := newTestModel(&fakeTranslator{response: "x"})
	m.source.SetValue("hello")

	m, _ = update(m, tea.KeyPressMsg{Code: tea.KeyEnter})
	if !m.busy {
		t.Fatal("expected busy after Enter")
	}

	m, cmd := update(m, tea.KeyPressMsg{Code: tea.KeyEscape})
	if cmd != nil {
		t.Fatal("Esc must not return a command")
	}
	if m.busy {
		t.Fatal("Esc must cancel the active request")
	}
	if !m.cancelled {
		t.Fatal("cancelled must be true after Esc cancels a request")
	}
}

func TestEscHasNoEffectWhenIdle(t *testing.T) {
	m := newTestModel(&fakeTranslator{})
	before := m

	m, cmd := update(m, tea.KeyPressMsg{Code: tea.KeyEscape})
	if cmd != nil {
		t.Fatal("Esc while idle must not return a command")
	}
	if m.busy != before.busy || m.cancelled != before.cancelled || m.everSubmitted != before.everSubmitted {
		t.Fatal("Esc while idle must not change request state")
	}
}

func TestStaleResultAfterCancelIsIgnored(t *testing.T) {
	m := newTestModel(&fakeTranslator{response: "x"})
	m.source.SetValue("hello")

	m, cmd := update(m, tea.KeyPressMsg{Code: tea.KeyEnter})
	staleSeq := m.reqSeq
	rm, _ := findResultMsg(collectMsgs(cmd))

	m, _ = update(m, tea.KeyPressMsg{Code: tea.KeyEscape})
	m, cmd = update(m, tea.KeyPressMsg{Code: tea.KeyEnter}) // start a fresh request
	if m.reqSeq == staleSeq {
		t.Fatal("a new submission must use a new sequence number")
	}

	m, _ = update(m, rm) // the stale result from the cancelled request arrives late
	if !m.busy {
		t.Fatal("a stale result must not affect the current in-flight request")
	}
	_ = cmd
}

func TestCtrlCExits(t *testing.T) {
	m := newTestModel(&fakeTranslator{})
	_, cmd := update(m, tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl})
	if cmd == nil {
		t.Fatal("Ctrl+C must return a command")
	}
	msg := cmd()
	if _, ok := msg.(tea.InterruptMsg); !ok {
		t.Fatalf("Ctrl+C command produced %T, want tea.InterruptMsg", msg)
	}
}

func TestCtrlCCancelsActiveRequest(t *testing.T) {
	m := newTestModel(&fakeTranslator{response: "x"})
	m.source.SetValue("hello")
	m, _ = update(m, tea.KeyPressMsg{Code: tea.KeyEnter})
	if m.cancel == nil {
		t.Fatal("expected an active cancel func after Enter")
	}

	ctx, cancel := context.WithCancel(context.Background())
	m.cancel = cancel
	_, cmd := update(m, tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl})
	if cmd == nil {
		t.Fatal("Ctrl+C must return a command")
	}
	if ctx.Err() == nil {
		t.Fatal("Ctrl+C must cancel the active request's context")
	}
}

func TestResizeUpdatesComponentSizes(t *testing.T) {
	m := newTestModel(&fakeTranslator{})
	m, _ = update(m, tea.WindowSizeMsg{Width: 100, Height: 40})

	if m.source.Width() <= 0 {
		t.Fatal("Source width must be positive after resize")
	}
	if m.context.Width() <= 0 {
		t.Fatal("Context width must be positive after resize")
	}
	if m.result.Width() <= 0 {
		t.Fatal("Translation width must be positive after resize")
	}
	if m.source.Height() <= 0 || m.context.Height() <= 0 || m.result.Height() <= 0 {
		t.Fatal("component heights must be positive after resize")
	}

	m, _ = update(m, tea.WindowSizeMsg{Width: 40, Height: 20})
	if m.source.Width() >= 100 {
		t.Fatal("Source width must shrink for a smaller window")
	}
}

func TestPageUpPageDownScrollTranslation(t *testing.T) {
	m := newTestModel(&fakeTranslator{})
	m, _ = update(m, tea.WindowSizeMsg{Width: 60, Height: 20})

	var lines string
	for i := 0; i < 200; i++ {
		lines += "line\n"
	}
	m.result.SetContent(lines)

	m, _ = update(m, tea.KeyPressMsg{Code: tea.KeyPgDown})
	afterPgDown := m.result.YOffset()
	if afterPgDown <= 0 {
		t.Fatalf("YOffset after PageDown = %d, want > 0", afterPgDown)
	}

	m, _ = update(m, tea.KeyPressMsg{Code: tea.KeyPgUp})
	afterPgUp := m.result.YOffset()
	if afterPgUp >= afterPgDown {
		t.Fatalf("YOffset after PageUp = %d, want < %d", afterPgUp, afterPgDown)
	}
}

func TestHelpChangesWithKeyboardEnhancementSupport(t *testing.T) {
	m := newTestModel(&fakeTranslator{})
	if m.keyDisambiguation {
		t.Fatal("keyDisambiguation must start false")
	}
	if got := m.helpLine(); !strings.Contains(got, "Alt+Enter") {
		t.Fatalf("helpLine() = %q, want it to mention Alt+Enter", got)
	}

	m, _ = update(m, tea.KeyboardEnhancementsMsg{Flags: 1})
	if !m.keyDisambiguation {
		t.Fatal("keyDisambiguation must be true after a non-zero KeyboardEnhancementsMsg")
	}
	if got := m.helpLine(); !strings.Contains(got, "Shift+Enter") {
		t.Fatalf("helpLine() = %q, want it to mention Shift+Enter", got)
	}
}
