package tui

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/fred01/quick-translate/internal/config"
	"github.com/fred01/quick-translate/internal/prompt"
	"github.com/fred01/quick-translate/internal/translate"
)

// recordingTranslator captures every TranslationInput it is given, so tests
// can assert what the submit path actually sent (for example, which tones). In
// tests the batch's commands run sequentially through collectMsgs, so no
// locking is needed.
type recordingTranslator struct {
	inputs []translate.TranslationInput
}

func (r *recordingTranslator) Translate(_ context.Context, input translate.TranslationInput, _ translate.Reporter) (string, error) {
	r.inputs = append(r.inputs, input)
	return "ok", nil
}

func (r *recordingTranslator) tones() []prompt.Tone {
	var out []prompt.Tone
	for _, in := range r.inputs {
		out = append(out, in.Tone)
	}
	return out
}

var errSetupTest = errors.New("endpoint test failed")

// --- Word wrapping ---

func TestResultWrapsByWord(t *testing.T) {
	m := newTestModel(&fakeTranslator{})
	m, _ = update(m, tea.WindowSizeMsg{Width: 40, Height: 30})

	long := "This is a long sentence that should wrap at word boundaries rather than being cut off."
	m.results[m.activeResult] = toneResult{text: long}
	m.setResultContent()

	content := m.result.GetContent()
	if !strings.Contains(content, "\n") {
		t.Fatalf("expected wrapped content to contain newlines, got %q", content)
	}
	// No visual line should exceed the viewport width.
	width := m.result.Width()
	for _, line := range strings.Split(content, "\n") {
		if w := len([]rune(line)); w > width {
			t.Fatalf("wrapped line %q width %d exceeds viewport width %d", line, w, width)
		}
	}
	// Words must stay intact (no mid-word break of "boundaries").
	for _, line := range strings.Split(content, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasSuffix(trimmed, "bound") || strings.HasPrefix(trimmed, "aries") {
			t.Fatalf("word was split across lines: %q", line)
		}
	}
}

func TestSuccessStoresResultTextForCopy(t *testing.T) {
	fake := &fakeTranslator{response: "translated result"}
	m := newTestModel(fake)
	m, _ = update(m, tea.WindowSizeMsg{Width: 80, Height: 30})
	m.source.SetValue("hello")

	m, cmd := update(m, tea.KeyPressMsg{Code: tea.KeyEnter})
	rm, _ := findResultMsg(collectMsgs(cmd))
	m, _ = update(m, rm)

	if got := m.activeResultText(); got != "translated result" {
		t.Fatalf("activeResultText = %q, want %q", got, "translated result")
	}
}

// --- Focus ring with buttons ---

func TestFocusRingExcludesMouseOnlyButtons(t *testing.T) {
	m := newTestModel(&fakeTranslator{})

	// No result yet: the result tabs and Copy are not focusable; Setup and
	// Quit are never in the ring (they are mouse-only). Context is drawn
	// above Tone and Source on screen but is pulled down to second place in
	// the ring; everything else follows screen order top to bottom.
	order := m.focusOrder()
	want := []focusTarget{focusTone, focusContext, focusSource, focusTranslateBtn, focusClearBtn}
	if len(order) != len(want) {
		t.Fatalf("focusOrder without result = %v, want %v", order, want)
	}
	for i := range want {
		if order[i] != want[i] {
			t.Fatalf("focusOrder without result = %v, want %v", order, want)
		}
	}

	// A single-variant result adds Copy but no tabs (tabs need >1 variant).
	m.reqTones = []prompt.Tone{prompt.DefaultTone}
	m.results[prompt.DefaultTone] = toneResult{text: "something"}
	order = m.focusOrder()
	wantWithCopy := []focusTarget{focusTone, focusContext, focusSource, focusTranslateBtn, focusClearBtn, focusCopyBtn}
	if len(order) != len(wantWithCopy) {
		t.Fatalf("focusOrder with one result = %v, want %v", order, wantWithCopy)
	}

	// Two variants add the result tabs as well.
	m.reqTones = []prompt.Tone{prompt.ToneLiteral, prompt.ToneDiplomatic}
	m.activeResult = prompt.ToneLiteral
	m.results = map[prompt.Tone]toneResult{prompt.ToneLiteral: {text: "x"}, prompt.ToneDiplomatic: {text: "y"}}
	order = m.focusOrder()
	wantWithTabs := []focusTarget{focusTone, focusContext, focusSource, focusTranslateBtn, focusClearBtn, focusResultTabs, focusCopyBtn}
	if len(order) != len(wantWithTabs) {
		t.Fatalf("focusOrder with two results = %v, want %v", order, wantWithTabs)
	}
	for i := range wantWithTabs {
		if order[i] != wantWithTabs[i] {
			t.Fatalf("focusOrder with two results = %v, want %v", order, wantWithTabs)
		}
	}
}

func TestTabCyclesFocusableElements(t *testing.T) {
	m := newTestModel(&fakeTranslator{})
	// One variant result: Copy is focusable, tabs are not. Starting focus is
	// Source; the ring runs Tone, Context, Source, Translate, Clear, Copy.
	m.reqTones = []prompt.Tone{prompt.DefaultTone}
	m.results[prompt.DefaultTone] = toneResult{text: "x"}

	seq := []focusTarget{focusTranslateBtn, focusClearBtn, focusCopyBtn, focusTone, focusContext, focusSource}
	for i, wantFocus := range seq {
		m, _ = update(m, tea.KeyPressMsg{Code: tea.KeyTab})
		if m.focus != wantFocus {
			t.Fatalf("after %d tabs, focus = %v, want %v", i+1, m.focus, wantFocus)
		}
	}
}

func TestTranslateButtonEnterSubmits(t *testing.T) {
	fake := &fakeTranslator{response: "x"}
	m := newTestModel(fake)
	m.source.SetValue("hello")
	m = focusButton(t, m, focusTranslateBtn)

	m, cmd := update(m, tea.KeyPressMsg{Code: tea.KeyEnter})
	if !m.busy {
		t.Fatal("Enter on the Translate button must submit")
	}
	if cmd == nil {
		t.Fatal("Enter on the Translate button must return a command")
	}
}

func TestMouseClickSetupOpensManager(t *testing.T) {
	m, _ := newManagerModel(t, &fakeTranslator{})
	m, _ = update(m, tea.WindowSizeMsg{Width: 100, Height: 34})
	_ = m.View()
	zone, ok := m.holder.zones[btnSetup]
	if !ok {
		t.Fatal("Setup button zone was not recorded during render")
	}

	m, _ = update(m, tea.MouseClickMsg{X: zone.x0, Y: zone.y0, Button: tea.MouseLeft})
	if m.screen != screenSetup {
		t.Fatal("clicking the Setup button must open the profile manager")
	}
}

func TestMouseClickQuit(t *testing.T) {
	m := newTestModel(&fakeTranslator{})
	m, _ = update(m, tea.WindowSizeMsg{Width: 100, Height: 34})
	_ = m.View()
	zone, ok := m.holder.zones[btnQuit]
	if !ok {
		t.Fatal("Quit button zone was not recorded during render")
	}

	_, cmd := update(m, tea.MouseClickMsg{X: zone.x0, Y: zone.y0, Button: tea.MouseLeft})
	if cmd == nil {
		t.Fatal("clicking Quit must return a command")
	}
	if _, ok := cmd().(tea.QuitMsg); !ok {
		t.Fatalf("clicking Quit must produce tea.QuitMsg, got %T", cmd())
	}
}

// --- Copy ---

func TestCopyWithoutResultDoesNothing(t *testing.T) {
	m := newTestModel(&fakeTranslator{})
	cmd := m.copyResult()
	if cmd != nil {
		t.Fatal("copyResult with no result must return nil")
	}
	if m.copied {
		t.Fatal("copied must stay false with no result")
	}
}

func TestCopyWithResultSetsClipboard(t *testing.T) {
	m := newTestModel(&fakeTranslator{})
	m.results[m.activeResult] = toneResult{text: "copy me"}
	cmd := m.copyResult()
	if cmd == nil {
		t.Fatal("copyResult with a result must return a clipboard command")
	}
	if !m.copied {
		t.Fatal("copied must be true after copyResult")
	}
	// Executing the clipboard command must not panic and must yield a message
	// (tea.SetClipboard emits an OSC52 clipboard message).
	if cmd() == nil {
		t.Fatal("clipboard command produced no message")
	}
}

// --- Mouse ---

func TestMouseClickTranslateButtonSubmits(t *testing.T) {
	fake := &fakeTranslator{response: "x"}
	m := newTestModel(fake)
	m, _ = update(m, tea.WindowSizeMsg{Width: 90, Height: 32})
	m.source.SetValue("hello")

	// Render once to populate the button hit-boxes.
	_ = m.View()
	zone, ok := m.holder.zones[btnTranslate]
	if !ok {
		t.Fatal("Translate button zone was not recorded during render")
	}

	m, cmd := update(m, tea.MouseClickMsg{X: zone.x0, Y: zone.y0, Button: tea.MouseLeft})
	if !m.busy {
		t.Fatal("clicking the Translate button must submit")
	}
	if cmd == nil {
		t.Fatal("clicking the Translate button must return a command")
	}
}

func TestMouseClickContextFieldFocuses(t *testing.T) {
	m := newTestModel(&fakeTranslator{})
	m, _ = update(m, tea.WindowSizeMsg{Width: 90, Height: 32})
	_ = m.View()

	zone, ok := m.holder.zones[btnContextField]
	if !ok {
		t.Fatal("Context field zone was not recorded during render")
	}

	m, _ = update(m, tea.MouseClickMsg{X: zone.x0, Y: zone.y0, Button: tea.MouseLeft})
	if m.focus != focusContext {
		t.Fatalf("clicking the Context field must focus it, focus = %v", m.focus)
	}
	if !m.context.Focused() || m.source.Focused() {
		t.Fatal("clicking Context must focus it and blur Source")
	}
}

func TestMouseClickSourceFieldFocuses(t *testing.T) {
	m := newTestModel(&fakeTranslator{})
	m, _ = update(m, tea.WindowSizeMsg{Width: 90, Height: 32})
	m, _ = update(m, tea.KeyPressMsg{Code: tea.KeyTab, Mod: tea.ModShift}) // start off Source, on Context
	_ = m.View()

	zone, ok := m.holder.zones[btnSourceField]
	if !ok {
		t.Fatal("Source field zone was not recorded during render")
	}

	m, _ = update(m, tea.MouseClickMsg{X: zone.x0, Y: zone.y0, Button: tea.MouseLeft})
	if m.focus != focusSource {
		t.Fatalf("clicking the Source field must focus it, focus = %v", m.focus)
	}
	if !m.source.Focused() || m.context.Focused() {
		t.Fatal("clicking Source must focus it and blur Context")
	}
}

func TestHitTest(t *testing.T) {
	zones := map[buttonID]rect{
		btnTranslate: {x0: 1, y0: 5, x1: 12, y1: 5},
		btnSetup:     {x0: 70, y0: 0, x1: 80, y1: 0},
	}
	if got := hitTest(zones, 6, 5); got != btnTranslate {
		t.Fatalf("hitTest inside translate = %v, want btnTranslate", got)
	}
	if got := hitTest(zones, 75, 0); got != btnSetup {
		t.Fatalf("hitTest inside setup = %v, want btnSetup", got)
	}
	if got := hitTest(zones, 6, 6); got != btnNone {
		t.Fatalf("hitTest outside = %v, want btnNone", got)
	}
}

// --- Profile manager ---

func newManagerModel(t *testing.T, fake *fakeTranslator) (model, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.json")
	store := config.Store{
		Active: "litellm",
		Profiles: map[string]config.Config{
			"litellm": {BaseURL: "http://localhost:4000/v1", Model: "gemma", APIKey: "lite-key"},
			"nvidia":  {BaseURL: "https://integrate.api.nvidia.com/v1", Model: "llama", APIKey: "nv-key"},
		},
	}
	if err := config.SaveStore(path, store); err != nil {
		t.Fatalf("SaveStore() error: %v", err)
	}
	m := newModel(fake, "gemma", "localhost:4000")
	m.configPath = path
	m.getenv = func(string) string { return "" }
	m.newTranslator = func(config.Config) (translate.Translator, error) { return fake, nil }
	m.store = store
	m.activeName = "litellm"
	return m, path
}

func TestOpenSetupShowsListWithActiveSelected(t *testing.T) {
	m, _ := newManagerModel(t, &fakeTranslator{})
	m.openSetup()

	if m.screen != screenSetup {
		t.Fatal("openSetup must switch to the setup screen")
	}
	if m.setupMode != setupList {
		t.Fatal("openSetup must start in list mode")
	}
	// Cursor should land on the active profile (litellm is index 0 sorted).
	if name, _ := m.selectedName(); name != "litellm" {
		t.Fatalf("cursor on %q, want the active profile litellm", name)
	}
}

func TestManagerUseSwitchesActiveAndRebuildsTranslator(t *testing.T) {
	fake := &fakeTranslator{}
	m, path := newManagerModel(t, fake)
	m.openSetup()

	// Move cursor to nvidia (index 1) and Use it.
	m, _ = update(m, tea.KeyPressMsg{Code: tea.KeyDown})
	if name, _ := m.selectedName(); name != "nvidia" {
		t.Fatalf("cursor on %q, want nvidia", name)
	}
	m, _ = update(m, tea.KeyPressMsg{Code: tea.KeyEnter})

	if m.activeName != "nvidia" {
		t.Fatalf("active = %q, want nvidia", m.activeName)
	}
	if m.modelName != "llama" {
		t.Fatalf("model = %q, want llama", m.modelName)
	}
	saved, _ := config.LoadStore(path)
	if saved.Active != "nvidia" {
		t.Fatalf("persisted active = %q, want nvidia", saved.Active)
	}
}

func TestManagerEditPrefillsWithoutAPIKey(t *testing.T) {
	m, _ := newManagerModel(t, &fakeTranslator{})
	m.openSetup()
	m.openEditSelected() // litellm

	if m.setupMode != setupEdit {
		t.Fatal("editing must switch to edit mode")
	}
	if got := m.setupInputs[editName].Value(); got != "litellm" {
		t.Fatalf("name prefill = %q, want litellm", got)
	}
	if got := m.setupInputs[editBaseURL].Value(); got != "http://localhost:4000/v1" {
		t.Fatalf("base URL prefill = %q", got)
	}
	if got := m.setupInputs[editModel].Value(); got != "gemma" {
		t.Fatalf("model prefill = %q", got)
	}
	if got := m.setupInputs[editAPIKey].Value(); got != "" {
		t.Fatalf("API key field = %q, want empty (existing key must never be prefilled)", got)
	}
}

func TestManagerEditBlankKeyKeepsExistingKey(t *testing.T) {
	fake := &fakeTranslator{response: "ok"}
	m, path := newManagerModel(t, fake)
	m.openSetup()
	m.openEditSelected() // litellm, existing key "lite-key"
	m.setupInputs[editModel].SetValue("gemma-v2")
	// leave API key blank

	if m.existingKey() != "lite-key" {
		t.Fatalf("existingKey() = %q, want lite-key", m.existingKey())
	}

	cmd := m.saveEdit()
	if cmd == nil || m.setupErr != nil {
		t.Fatalf("saveEdit with blank key on an existing profile must succeed; err=%v", m.setupErr)
	}
	msg, _ := findSetupResultMsg(collectMsgs(cmd))
	if msg.err != nil {
		t.Fatalf("setup test unexpectedly failed: %v", msg.err)
	}

	saved, _ := config.LoadStore(path)
	if got := saved.Profiles["litellm"].APIKey; got != "lite-key" {
		t.Fatalf("API key after blank-key edit = %q, want the preserved lite-key", got)
	}
	if got := saved.Profiles["litellm"].Model; got != "gemma-v2" {
		t.Fatalf("model after edit = %q, want gemma-v2", got)
	}
}

// managerWithTimeout builds a manager model whose active "litellm" profile has
// a stored per-request timeout, then opens it in the editor.
func managerWithTimeout(t *testing.T, seconds int) (model, string) {
	t.Helper()
	fake := &fakeTranslator{response: "ok"}
	m, path := newManagerModel(t, fake)
	prof := m.store.Profiles["litellm"]
	prof.TimeoutSeconds = seconds
	m.store.Profiles["litellm"] = prof
	if err := config.SaveStore(path, m.store); err != nil {
		t.Fatalf("SaveStore() error: %v", err)
	}
	m.openSetup()
	m.openEditSelected() // litellm
	return m, path
}

func TestManagerEditPrefillsAndPreservesTimeout(t *testing.T) {
	m, path := managerWithTimeout(t, 240)

	// The stored timeout is prefilled into the editable field.
	if got := m.setupInputs[editTimeout].Value(); got != "240" {
		t.Fatalf("Timeout field = %q, want prefilled 240", got)
	}
	// Editing another field and leaving Timeout untouched preserves it.
	m.setupInputs[editModel].SetValue("gemma-v2")

	cmd := m.saveEdit()
	if cmd == nil || m.setupErr != nil {
		t.Fatalf("saveEdit must succeed; err=%v", m.setupErr)
	}
	if msg, _ := findSetupResultMsg(collectMsgs(cmd)); msg.err != nil {
		t.Fatalf("setup test unexpectedly failed: %v", msg.err)
	}

	saved, _ := config.LoadStore(path)
	if got := saved.Profiles["litellm"].TimeoutSeconds; got != 240 {
		t.Fatalf("TimeoutSeconds after edit = %d, want the preserved 240", got)
	}
	if got := saved.Profiles["litellm"].Model; got != "gemma-v2" {
		t.Fatalf("model after edit = %q, want gemma-v2", got)
	}
}

func TestMouseClickTimeoutFieldFocusesIt(t *testing.T) {
	m, _ := managerWithTimeout(t, 240)
	m, _ = update(m, tea.WindowSizeMsg{Width: 100, Height: 40})
	_ = m.View()
	zone, ok := m.holder.zones[btnEditTimeoutField]
	if !ok {
		t.Fatal("Timeout field zone was not recorded during render")
	}

	m, _ = update(m, tea.MouseClickMsg{X: zone.x0, Y: zone.y0, Button: tea.MouseLeft})
	if m.setupFocus != editTimeout {
		t.Fatalf("clicking the Timeout field must focus it; setupFocus=%d", m.setupFocus)
	}
}

func TestManagerEditChangesTimeout(t *testing.T) {
	m, path := managerWithTimeout(t, 240)
	m.setupInputs[editTimeout].SetValue("300")

	cmd := m.saveEdit()
	if cmd == nil || m.setupErr != nil {
		t.Fatalf("saveEdit must succeed; err=%v", m.setupErr)
	}
	if msg, _ := findSetupResultMsg(collectMsgs(cmd)); msg.err != nil {
		t.Fatalf("setup test unexpectedly failed: %v", msg.err)
	}

	saved, _ := config.LoadStore(path)
	if got := saved.Profiles["litellm"].TimeoutSeconds; got != 300 {
		t.Fatalf("TimeoutSeconds after edit = %d, want 300", got)
	}
}

func TestManagerEditBlankTimeoutMeansDefault(t *testing.T) {
	m, path := managerWithTimeout(t, 240)
	m.setupInputs[editTimeout].SetValue("") // clear → default

	cmd := m.saveEdit()
	if cmd == nil || m.setupErr != nil {
		t.Fatalf("saveEdit must succeed; err=%v", m.setupErr)
	}
	if msg, _ := findSetupResultMsg(collectMsgs(cmd)); msg.err != nil {
		t.Fatalf("setup test unexpectedly failed: %v", msg.err)
	}

	saved, _ := config.LoadStore(path)
	if got := saved.Profiles["litellm"].TimeoutSeconds; got != 0 {
		t.Fatalf("TimeoutSeconds after clearing = %d, want 0 (default)", got)
	}
}

func TestManagerEditRejectsInvalidTimeout(t *testing.T) {
	for _, bad := range []string{"abc", "-5", "1.5"} {
		m, path := managerWithTimeout(t, 240)
		m.setupInputs[editTimeout].SetValue(bad)

		if cmd := m.saveEdit(); cmd != nil {
			t.Fatalf("saveEdit(%q) must not proceed to testing", bad)
		}
		if m.setupErr == nil {
			t.Fatalf("saveEdit(%q) must set a validation error", bad)
		}
		// Nothing was saved: the stored timeout is unchanged.
		saved, _ := config.LoadStore(path)
		if got := saved.Profiles["litellm"].TimeoutSeconds; got != 240 {
			t.Fatalf("TimeoutSeconds after rejected edit = %d, want the unchanged 240", got)
		}
	}
}

func TestManagerRenameRemovesOldProfile(t *testing.T) {
	fake := &fakeTranslator{response: "ok"}
	m, path := newManagerModel(t, fake)
	m.openSetup()
	m.openEditSelected() // litellm (active), key "lite-key"
	m.setupInputs[editName].SetValue("litellm-renamed")
	// leave base/model/key as prefilled (key blank keeps lite-key)

	cmd := m.saveEdit()
	if cmd == nil || m.setupErr != nil {
		t.Fatalf("saveEdit rename must succeed; err=%v", m.setupErr)
	}
	msg, _ := findSetupResultMsg(collectMsgs(cmd))
	if msg.err != nil {
		t.Fatalf("setup test unexpectedly failed: %v", msg.err)
	}
	m, _ = update(m, msg)

	saved, _ := config.LoadStore(path)
	if _, ok := saved.Profiles["litellm"]; ok {
		t.Fatal("the old profile name must be removed after a rename")
	}
	renamed, ok := saved.Profiles["litellm-renamed"]
	if !ok {
		t.Fatal("the renamed profile must exist")
	}
	if renamed.APIKey != "lite-key" {
		t.Fatalf("renamed profile key = %q, want the preserved lite-key", renamed.APIKey)
	}
	if saved.Active != "litellm-renamed" {
		t.Fatalf("active = %q, want litellm-renamed", saved.Active)
	}
	// The other profile is untouched.
	if _, ok := saved.Profiles["nvidia"]; !ok {
		t.Fatal("unrelated profiles must be preserved through a rename")
	}
}

func TestManagerNewProfileRequiresKey(t *testing.T) {
	m, _ := newManagerModel(t, &fakeTranslator{response: "ok"})
	m.openSetup()
	m.openEditNew()
	m.setupInputs[editName].SetValue("local")
	m.setupInputs[editBaseURL].SetValue("http://localhost:9/v1")
	m.setupInputs[editModel].SetValue("m")
	// leave API key blank — a new profile has no key to keep

	if m.existingKey() != "" {
		t.Fatalf("existingKey() = %q, want empty for a new profile", m.existingKey())
	}
	cmd := m.saveEdit()
	if cmd != nil {
		t.Fatal("saveEdit must not proceed with a blank key for a new profile")
	}
	if m.setupErr == nil {
		t.Fatal("a blank key on a new profile must produce an error")
	}
}

func TestManagerAddStartsBlankEditor(t *testing.T) {
	m, _ := newManagerModel(t, &fakeTranslator{})
	m.openSetup()
	m.openEditNew()

	if m.setupMode != setupEdit {
		t.Fatal("adding must switch to edit mode")
	}
	if m.editName != "" {
		t.Fatalf("editName = %q, want empty for a new profile", m.editName)
	}
	for _, i := range []int{editName, editBaseURL, editModel, editAPIKey} {
		if m.setupInputs[i].Value() != "" {
			t.Fatalf("input %d = %q, want empty for a new profile", i, m.setupInputs[i].Value())
		}
	}
}

func TestMouseClickEditFieldFocuses(t *testing.T) {
	m, _ := newManagerModel(t, &fakeTranslator{})
	m.openSetup()
	m.openEditSelected()
	m, _ = update(m, tea.WindowSizeMsg{Width: 90, Height: 32})
	_ = m.View()

	zone, ok := m.holder.zones[btnEditBaseURLField]
	if !ok {
		t.Fatal("Base URL field zone was not recorded during render")
	}

	m, _ = update(m, tea.MouseClickMsg{X: zone.x0, Y: zone.y0, Button: tea.MouseLeft})
	if m.setupFocus != editBaseURL {
		t.Fatalf("clicking the Base URL field must focus it, setupFocus = %v", m.setupFocus)
	}
	if !m.setupInputs[editBaseURL].Focused() {
		t.Fatal("clicking Base URL must focus its input")
	}
	if m.setupInputs[editName].Focused() {
		t.Fatal("clicking Base URL must blur the Name input")
	}
}

func TestManagerSaveInvalidCandidateDoesNotTest(t *testing.T) {
	fake := &fakeTranslator{response: "Hello!"}
	m, _ := newManagerModel(t, fake)
	m.openSetup()
	m.openEditNew()
	m.setupInputs[editName].SetValue("local")
	m.setupInputs[editBaseURL].SetValue("not a url")
	m.setupInputs[editModel].SetValue("m")
	m.setupInputs[editAPIKey].SetValue("k")

	cmd := m.saveEdit()
	if cmd != nil {
		t.Fatal("saveEdit with an invalid candidate must not return a test command")
	}
	if m.setupErr == nil {
		t.Fatal("saveEdit with an invalid candidate must set setupErr")
	}
	if fake.calls != 0 {
		t.Fatalf("translator must not be called for an invalid candidate, got %d calls", fake.calls)
	}
}

func TestManagerSaveSuccessPersistsAndActivates(t *testing.T) {
	fake := &fakeTranslator{response: "Hello!"}
	m, path := newManagerModel(t, fake)
	m.openSetup()
	m.openEditNew()
	m.setupInputs[editName].SetValue("local")
	m.setupInputs[editBaseURL].SetValue("http://localhost:9/v1")
	m.setupInputs[editModel].SetValue("local-model")
	m.setupInputs[editAPIKey].SetValue("local-key")

	cmd := m.saveEdit()
	if cmd == nil || !m.setupBusy {
		t.Fatal("saveEdit with a valid candidate must test in a busy command")
	}
	msg, ok := findSetupResultMsg(collectMsgs(cmd))
	if !ok || msg.err != nil {
		t.Fatalf("expected a successful setupResultMsg, got %+v ok=%v", msg, ok)
	}

	m, _ = update(m, msg)
	if m.setupMode != setupList {
		t.Fatal("a successful save must return to list mode")
	}
	if m.activeName != "local" {
		t.Fatalf("active = %q, want local (just-saved profile)", m.activeName)
	}

	saved, _ := config.LoadStore(path)
	if saved.Active != "local" {
		t.Fatalf("persisted active = %q, want local", saved.Active)
	}
	want := config.Config{BaseURL: "http://localhost:9/v1", Model: "local-model", APIKey: "local-key"}
	if saved.Profiles["local"] != want {
		t.Fatalf("saved profile = %+v, want %+v", saved.Profiles["local"], want)
	}
	if _, ok := saved.Profiles["nvidia"]; !ok {
		t.Fatal("existing profiles must be preserved")
	}
}

func TestManagerSaveFailureStaysInEditor(t *testing.T) {
	fake := &fakeTranslator{err: errSetupTest}
	m, path := newManagerModel(t, fake)
	m.openSetup()
	m.openEditNew()
	m.setupInputs[editName].SetValue("local")
	m.setupInputs[editBaseURL].SetValue("http://localhost:9/v1")
	m.setupInputs[editModel].SetValue("local-model")
	m.setupInputs[editAPIKey].SetValue("local-key")

	cmd := m.saveEdit()
	msg, _ := findSetupResultMsg(collectMsgs(cmd))
	m, _ = update(m, msg)

	if m.setupMode != setupEdit {
		t.Fatal("a failed save must stay in the editor")
	}
	if m.setupErr == nil {
		t.Fatal("a failed save must show an error")
	}
	saved, _ := config.LoadStore(path)
	if _, ok := saved.Profiles["local"]; ok {
		t.Fatal("a failed test must not persist the profile")
	}
}

func TestManagerDeleteNonActive(t *testing.T) {
	m, path := newManagerModel(t, &fakeTranslator{})
	m.openSetup()
	// Move to nvidia (non-active) and delete.
	m, _ = update(m, tea.KeyPressMsg{Code: tea.KeyDown})
	m, _ = update(m, tea.KeyPressMsg{Code: 'd'})

	if m.setupErr != nil {
		t.Fatalf("deleting a non-active profile errored: %v", m.setupErr)
	}
	saved, _ := config.LoadStore(path)
	if _, ok := saved.Profiles["nvidia"]; ok {
		t.Fatal("nvidia should have been deleted")
	}
}

func TestManagerDeleteActiveIsError(t *testing.T) {
	m, path := newManagerModel(t, &fakeTranslator{})
	m.openSetup() // cursor on active litellm
	m, _ = update(m, tea.KeyPressMsg{Code: 'd'})

	if m.setupErr == nil {
		t.Fatal("deleting the active profile must set an error")
	}
	saved, _ := config.LoadStore(path)
	if _, ok := saved.Profiles["litellm"]; !ok {
		t.Fatal("the active profile must not be deleted")
	}
}

func TestManagerEscFromEditReturnsToList(t *testing.T) {
	m, _ := newManagerModel(t, &fakeTranslator{})
	m.openSetup()
	m.openEditNew()

	m, _ = update(m, tea.KeyPressMsg{Code: tea.KeyEscape})
	if m.setupMode != setupList {
		t.Fatal("Esc in the editor must return to list mode")
	}
	if m.screen != screenSetup {
		t.Fatal("Esc in the editor must not leave the setup screen")
	}
}

func TestManagerEscFromListReturnsToTranslate(t *testing.T) {
	m, _ := newManagerModel(t, &fakeTranslator{})
	m.openSetup()

	m, _ = update(m, tea.KeyPressMsg{Code: tea.KeyEscape})
	if m.screen != screenTranslate {
		t.Fatal("Esc in list mode must return to the translate screen")
	}
}

func TestManagerResolveAPIKey(t *testing.T) {
	if got := resolveAPIKey("", "existing"); got != "existing" {
		t.Fatalf("resolveAPIKey blank = %q, want existing", got)
	}
	if got := resolveAPIKey("new", "existing"); got != "new" {
		t.Fatalf("resolveAPIKey non-blank = %q, want new", got)
	}
}

// helpers

func focusButton(t *testing.T, m model, target focusTarget) model {
	t.Helper()
	// Tab until the desired button is focused (guard against infinite loops).
	for i := 0; i < 10; i++ {
		if m.focus == target {
			return m
		}
		m, _ = update(m, tea.KeyPressMsg{Code: tea.KeyTab})
	}
	t.Fatalf("could not focus %v via Tab", target)
	return m
}

// --- Tone selector (checkboxes) ---

func TestToneDefaultsToNeutralChecked(t *testing.T) {
	m := newTestModel(&fakeTranslator{})
	if prompt.DefaultTone != prompt.ToneNeutral {
		t.Fatalf("DefaultTone = %v, want ToneNeutral", prompt.DefaultTone)
	}
	if !m.toneSelected[prompt.DefaultTone] {
		t.Fatalf("default tone %v must start checked", prompt.DefaultTone)
	}
	for _, opt := range toneOptions {
		if opt.tone != prompt.DefaultTone && m.toneSelected[opt.tone] {
			t.Fatalf("%v must start unchecked", opt.tone)
		}
	}
	if m.toneCursor != toneIndex(prompt.DefaultTone) {
		t.Fatalf("toneCursor = %d, want %d", m.toneCursor, toneIndex(prompt.DefaultTone))
	}
}

func TestToneArrowsMoveCursorWhenFocused(t *testing.T) {
	m := newTestModel(&fakeTranslator{})
	m = focusButton(t, m, focusTone)
	m.toneCursor = toneIndex(prompt.ToneNeutral) // known start (index 1)

	// Visual order is Literal(0), Neutral(1), Diplomatic(2).
	m, _ = update(m, tea.KeyPressMsg{Code: tea.KeyRight})
	if m.toneCursor != toneIndex(prompt.ToneDiplomatic) {
		t.Fatalf("Right from Neutral cursor = %d, want Diplomatic", m.toneCursor)
	}
	m, _ = update(m, tea.KeyPressMsg{Code: tea.KeyRight})
	if m.toneCursor != toneIndex(prompt.ToneLiteral) {
		t.Fatalf("Right from Diplomatic must wrap to Literal, got %d", m.toneCursor)
	}
	m, _ = update(m, tea.KeyPressMsg{Code: tea.KeyLeft})
	if m.toneCursor != toneIndex(prompt.ToneDiplomatic) {
		t.Fatalf("Left from Literal must wrap to Diplomatic, got %d", m.toneCursor)
	}
}

func TestToneSpaceTogglesCheckboxUnderCursor(t *testing.T) {
	m := newTestModel(&fakeTranslator{})
	m = focusButton(t, m, focusTone)
	m.toneCursor = toneIndex(prompt.ToneLiteral)

	m, _ = update(m, tea.KeyPressMsg{Code: tea.KeySpace})
	if !m.toneSelected[prompt.ToneLiteral] {
		t.Fatal("Space must check the tone under the cursor")
	}
	m, _ = update(m, tea.KeyPressMsg{Code: tea.KeySpace})
	if m.toneSelected[prompt.ToneLiteral] {
		t.Fatal("Space again must uncheck it")
	}
}

func TestToneArrowsIgnoredWhenSourceFocused(t *testing.T) {
	m := newTestModel(&fakeTranslator{})
	if m.focus != focusSource {
		t.Fatal("expected Source focused initially")
	}
	before := m.toneCursor
	m, _ = update(m, tea.KeyPressMsg{Code: tea.KeyRight})
	if m.toneCursor != before {
		t.Fatalf("Right while Source is focused must not move the tone cursor, got %d", m.toneCursor)
	}
}

func TestToneMouseClickTogglesAndFocuses(t *testing.T) {
	m := newTestModel(&fakeTranslator{})
	m, _ = update(m, tea.WindowSizeMsg{Width: 100, Height: 34})
	_ = m.View()

	zone, ok := m.holder.zones[btnToneLiteral]
	if !ok {
		t.Fatal("Literal checkbox zone was not recorded during render")
	}
	if m.toneSelected[prompt.ToneLiteral] {
		t.Fatal("Literal must start unchecked")
	}
	m, _ = update(m, tea.MouseClickMsg{X: zone.x0, Y: zone.y0, Button: tea.MouseLeft})
	if !m.toneSelected[prompt.ToneLiteral] {
		t.Fatal("clicking Literal must check it")
	}
	if m.focus != focusTone {
		t.Fatalf("clicking a tone checkbox must focus the tone selector, focus = %v", m.focus)
	}
	if m.toneCursor != toneIndex(prompt.ToneLiteral) {
		t.Fatalf("clicking Literal must move the cursor onto it, got %d", m.toneCursor)
	}
}

func TestEnterOnToneSubmits(t *testing.T) {
	m := newTestModel(&fakeTranslator{response: "x"})
	m.source.SetValue("hello")
	m = focusButton(t, m, focusTone)

	m, cmd := update(m, tea.KeyPressMsg{Code: tea.KeyEnter})
	if !m.busy {
		t.Fatal("Enter on the tone selector must submit")
	}
	if cmd == nil {
		t.Fatal("Enter on the tone selector must return a command")
	}
}

func TestSubmitWithNoToneSelectedDoesNothing(t *testing.T) {
	m := newTestModel(&fakeTranslator{})
	m.source.SetValue("hello")
	m.toneSelected = map[prompt.Tone]bool{} // uncheck everything

	m, cmd := update(m, tea.KeyPressMsg{Code: tea.KeyEnter})
	if cmd != nil {
		t.Fatal("submit with no tone selected must return no command")
	}
	if m.busy {
		t.Fatal("submit with no tone selected must not start a request")
	}
	if m.notice == "" {
		t.Fatal("submit with no tone selected should set an explanatory notice")
	}
}

func TestSubmitFiresAllSelectedTones(t *testing.T) {
	rec := &recordingTranslator{}
	m := newTestModel(rec)
	m, _ = update(m, tea.WindowSizeMsg{Width: 80, Height: 40})
	m.source.SetValue("hello")
	m.toneSelected = map[prompt.Tone]bool{
		prompt.ToneLiteral:    true,
		prompt.ToneNeutral:    true,
		prompt.ToneDiplomatic: true,
	}

	_, cmd := update(m, tea.KeyPressMsg{Code: tea.KeyEnter})
	msgs := collectMsgs(cmd)

	// One request per selected tone, in display order.
	want := []prompt.Tone{prompt.ToneLiteral, prompt.ToneNeutral, prompt.ToneDiplomatic}
	got := rec.tones()
	if len(got) != len(want) {
		t.Fatalf("fired %v tones, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("fired tones = %v, want %v", got, want)
		}
	}

	// Each request yields a resultMsg tagged with its tone.
	seen := map[prompt.Tone]bool{}
	for _, msg := range msgs {
		if rm, ok := msg.(resultMsg); ok {
			seen[rm.tone] = true
		}
	}
	for _, tone := range want {
		if !seen[tone] {
			t.Fatalf("missing resultMsg for tone %v", tone)
		}
	}
}

func TestResultTabsSwitchActiveVariant(t *testing.T) {
	m := newTestModel(&fakeTranslator{})
	m, _ = update(m, tea.WindowSizeMsg{Width: 80, Height: 40})
	// Simulate a completed two-variant batch.
	m.reqTones = []prompt.Tone{prompt.ToneLiteral, prompt.ToneDiplomatic}
	m.results = map[prompt.Tone]toneResult{
		prompt.ToneLiteral:    {text: "literal text", outputChars: 12},
		prompt.ToneDiplomatic: {text: "diplomatic text", outputChars: 15},
	}
	m.activeResult = prompt.ToneLiteral
	m.syncActiveResult()
	if !strings.Contains(m.result.GetContent(), "literal text") {
		t.Fatalf("viewport = %q, want literal text", m.result.GetContent())
	}

	// Focus the tabs and switch with the right arrow.
	m = focusButton(t, m, focusResultTabs)
	m, _ = update(m, tea.KeyPressMsg{Code: tea.KeyRight})
	if m.activeResult != prompt.ToneDiplomatic {
		t.Fatalf("Right on tabs → active %v, want Diplomatic", m.activeResult)
	}
	if !strings.Contains(m.result.GetContent(), "diplomatic text") {
		t.Fatalf("viewport = %q, want diplomatic text", m.result.GetContent())
	}
}

func TestClearResetsScreen(t *testing.T) {
	m := newTestModel(&fakeTranslator{})
	m, _ = update(m, tea.WindowSizeMsg{Width: 80, Height: 40})
	m.source.SetValue("hello")
	m.context.SetValue("ctx")
	m.reqTones = []prompt.Tone{prompt.ToneLiteral}
	m.results = map[prompt.Tone]toneResult{prompt.ToneLiteral: {text: "done"}}
	m.everSubmitted = true

	cmd := m.clearAll()
	if m.source.Value() != "" || m.context.Value() != "" {
		t.Fatal("clear must empty Source and Context")
	}
	if len(m.reqTones) != 0 || len(m.results) != 0 {
		t.Fatal("clear must drop all results")
	}
	if m.everSubmitted {
		t.Fatal("clear must reset everSubmitted")
	}
	if m.focus != focusSource {
		t.Fatalf("clear must return focus to Source, got %v", m.focus)
	}
	_ = cmd
}

func TestClearButtonReachableAndClears(t *testing.T) {
	m := newTestModel(&fakeTranslator{})
	m, _ = update(m, tea.WindowSizeMsg{Width: 90, Height: 40})
	m.source.SetValue("hello")
	_ = m.View()

	zone, ok := m.holder.zones[btnClear]
	if !ok {
		t.Fatal("Clear button zone was not recorded during render")
	}
	m, _ = update(m, tea.MouseClickMsg{X: zone.x0, Y: zone.y0, Button: tea.MouseLeft})
	if m.source.Value() != "" {
		t.Fatalf("clicking Clear must empty Source, got %q", m.source.Value())
	}
}

func findSetupResultMsg(msgs []tea.Msg) (setupResultMsg, bool) {
	for _, msg := range msgs {
		if sm, ok := msg.(setupResultMsg); ok {
			return sm, true
		}
	}
	return setupResultMsg{}, false
}
