package tui

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"charm.land/bubbles/v2/spinner"
	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/fred01/quick-translate/internal/config"
	"github.com/fred01/quick-translate/internal/prompt"
	"github.com/fred01/quick-translate/internal/translate"
)

// errMissingVariant marks a tone the model failed to return in a multi-tone
// response, so its tab can show an error instead of empty text.
var errMissingVariant = errors.New("the model did not return this variant")

// Update implements tea.Model.
func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	// Messages handled the same way on every screen.
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.resize(msg.Width, msg.Height)
		return m, nil

	case tea.KeyboardEnhancementsMsg:
		m.keyDisambiguation = msg.SupportsKeyDisambiguation()
		return m, nil

	case spinner.TickMsg:
		if !m.busy && !m.setupBusy {
			return m, nil
		}
		var cmd tea.Cmd
		m.spin, cmd = m.spin.Update(msg)
		return m, cmd
	}

	if m.screen == screenSetup {
		return m.updateSetup(msg)
	}
	return m.updateTranslate(msg)
}

// --- Translate screen ---

func (m model) updateTranslate(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case resultMsg:
		if msg.seq != m.reqSeq {
			return m, nil
		}
		m.busy = false
		for _, tone := range m.reqTones {
			switch {
			case msg.err != nil:
				m.results[tone] = toneResult{err: msg.err, elapsed: msg.elapsed}
			default:
				text, ok := msg.results[tone]
				if !ok || strings.TrimSpace(text) == "" {
					m.results[tone] = toneResult{err: errMissingVariant, elapsed: msg.elapsed}
					continue
				}
				m.results[tone] = toneResult{text: text, outputChars: len([]rune(text)), elapsed: msg.elapsed}
			}
		}
		m.syncActiveResult()
		return m, nil

	case tea.MouseClickMsg:
		return m.handleMouseClickTranslate(msg)

	case tea.MouseWheelMsg:
		var cmd tea.Cmd
		m.result, cmd = m.result.Update(msg)
		return m, cmd

	case tea.KeyPressMsg:
		if cmd, handled := m.handleKeyTranslate(msg); handled {
			return m, cmd
		}
	}

	return m.forwardToTextareas(msg)
}

// handleKeyTranslate handles every key with dedicated behavior on the
// translate screen. It reports handled=false for keys that should fall
// through to the focused text area (ordinary typing and editing).
func (m *model) handleKeyTranslate(msg tea.KeyPressMsg) (tea.Cmd, bool) {
	switch msg.String() {
	case "ctrl+c":
		if m.cancel != nil {
			m.cancel()
		}
		return tea.Interrupt, true
	case "esc":
		if m.busy && m.cancel != nil {
			m.cancel()
			m.busy = false
			m.cancelled = true
		}
		return nil, true
	case "tab":
		return m.cycleFocus(1), true
	case "shift+tab":
		return m.cycleFocus(-1), true
	case "left":
		if m.focus == focusTone {
			m.moveToneCursor(-1)
			return nil, true
		}
		if m.focus == focusResultTabs {
			m.moveActiveResult(-1)
			return nil, true
		}
		return nil, false
	case "right":
		if m.focus == focusTone {
			m.moveToneCursor(1)
			return nil, true
		}
		if m.focus == focusResultTabs {
			m.moveActiveResult(1)
			return nil, true
		}
		return nil, false
	case "space":
		if m.focus == focusTone {
			m.toggleToneAtCursor()
			return nil, true
		}
		return nil, false
	case "pgup":
		m.result.PageUp()
		return nil, true
	case "pgdown":
		m.result.PageDown()
		return nil, true
	case "enter":
		return m.activateFocused(), true
	case "shift+enter", "alt+enter":
		if m.focus == focusSource || m.focus == focusContext {
			m.insertNewlineInFocused()
		}
		return nil, true
	}
	return nil, false
}

func (m *model) activateFocused() tea.Cmd {
	switch m.focus {
	case focusSource, focusContext, focusTone, focusTranslateBtn:
		return m.submit()
	case focusClearBtn:
		return m.clearAll()
	case focusCopyBtn:
		return m.copyResult()
	}
	return nil
}

// clearAll resets the translate screen to a blank slate: it cancels any
// in-flight batch, empties Source and Context, drops all results, and returns
// focus to Source. The tone selection and active profile are left untouched.
func (m *model) clearAll() tea.Cmd {
	if m.cancel != nil {
		m.cancel()
		m.cancel = nil
	}
	m.busy = false
	m.cancelled = false
	m.copied = false
	m.notice = ""
	m.everSubmitted = false
	m.source.SetValue("")
	m.context.SetValue("")
	m.reqTones = nil
	m.results = map[prompt.Tone]toneResult{}
	if tones := m.selectedTonesInOrder(); len(tones) > 0 {
		m.activeResult = tones[0]
	} else {
		m.activeResult = prompt.DefaultTone
	}
	m.result.SetContent("")
	// Reclaim the result-tab row now that there are no variants.
	if m.width > 0 {
		m.resize(m.width, m.height)
	}
	return m.setFocus(focusSource)
}

// moveToneCursor moves the tone-checkbox cursor dir steps through the visual
// order (Literal, Neutral, Diplomatic), wrapping at both ends.
func (m *model) moveToneCursor(dir int) {
	m.toneCursor = (m.toneCursor + dir + len(toneOptions)) % len(toneOptions)
}

// toggleToneAtCursor checks or unchecks the tone the cursor is on.
func (m *model) toggleToneAtCursor() {
	tone := toneOptions[m.toneCursor].tone
	m.toneSelected[tone] = !m.toneSelected[tone]
}

// selectedTonesInOrder returns the checked tones in display order.
func (m *model) selectedTonesInOrder() []prompt.Tone {
	var out []prompt.Tone
	for _, opt := range toneOptions {
		if m.toneSelected[opt.tone] {
			out = append(out, opt.tone)
		}
	}
	return out
}

// moveActiveResult switches the shown result tab dir steps through reqTones,
// wrapping at both ends.
func (m *model) moveActiveResult(dir int) {
	if len(m.reqTones) == 0 {
		return
	}
	idx := 0
	for i, t := range m.reqTones {
		if t == m.activeResult {
			idx = i
			break
		}
	}
	m.setActiveResult(m.reqTones[(idx+dir+len(m.reqTones))%len(m.reqTones)])
}

// setActiveResult shows tone's result in the Translation viewport.
func (m *model) setActiveResult(tone prompt.Tone) {
	m.activeResult = tone
	m.syncActiveResult()
}

// syncActiveResult loads the active tone's translation into the viewport (or
// clears it while that tone is still pending or errored) and resets copy state.
func (m *model) syncActiveResult() {
	m.copied = false
	m.setResultContent()
	m.result.GotoTop()
}

// activeResultText returns the active tone's successful translation, or "" if
// it is still pending or failed.
func (m *model) activeResultText() string {
	if r, ok := m.results[m.activeResult]; ok && r.err == nil {
		return r.text
	}
	return ""
}

// submit starts one translation request covering every selected tone. The
// model is asked for all variants at once, so N tones cost a single
// round-trip. It does nothing when a batch is already running, Source is empty
// or whitespace-only, or no tone is selected.
func (m *model) submit() tea.Cmd {
	if m.busy {
		return nil
	}
	source := m.source.Value()
	if strings.TrimSpace(source) == "" {
		return nil
	}
	tones := m.selectedTonesInOrder()
	if len(tones) == 0 {
		m.notice = "Select at least one tone to translate"
		return nil
	}

	m.reqSeq++
	seq := m.reqSeq

	ctx, cancel := context.WithCancel(context.Background())
	m.cancel = cancel
	m.busy = true
	m.cancelled = false
	m.copied = false
	m.notice = ""
	m.everSubmitted = true

	m.reqTones = tones
	m.results = map[prompt.Tone]toneResult{}
	m.activeResult = tones[0]
	m.result.SetContent("")
	// Recompute pane heights now that the result-tab row may be shown.
	if m.width > 0 {
		m.resize(m.width, m.height)
	}

	return tea.Batch(m.spin.Tick, translateCmd(ctx, m.translator, source, m.context.Value(), tones, translate.DiscardReporter{}, seq))
}

// copyResult copies the active translation to the system clipboard via OSC52.
// It does nothing when the active tab has no successful result yet.
func (m *model) copyResult() tea.Cmd {
	text := m.activeResultText()
	if text == "" {
		return nil
	}
	m.copied = true
	return tea.SetClipboard(text)
}

func (m *model) cycleFocus(dir int) tea.Cmd {
	order := m.focusOrder()
	idx := 0
	for i, t := range order {
		if t == m.focus {
			idx = i
			break
		}
	}
	next := order[(idx+dir+len(order))%len(order)]
	return m.setFocus(next)
}

// focusOrder returns the focusable elements in visual Tab order. The result
// tabs join only when more than one variant was requested; Copy joins once
// the active tab has a translation. Setup and Quit are deliberately excluded
// — they are mouse-only.
func (m *model) focusOrder() []focusTarget {
	order := []focusTarget{focusSource, focusContext, focusTone, focusTranslateBtn, focusClearBtn}
	if len(m.reqTones) > 1 {
		order = append(order, focusResultTabs)
	}
	if m.activeResultText() != "" {
		order = append(order, focusCopyBtn)
	}
	return order
}

func (m *model) setFocus(t focusTarget) tea.Cmd {
	m.source.Blur()
	m.context.Blur()
	m.focus = t
	switch t {
	case focusSource:
		return m.source.Focus()
	case focusContext:
		return m.context.Focus()
	}
	return nil
}

func (m *model) insertNewlineInFocused() {
	if m.focus == focusSource {
		m.source.InsertRune('\n')
		return
	}
	m.context.InsertRune('\n')
}

func (m model) forwardToTextareas(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd
	var cmd tea.Cmd
	m.source, cmd = m.source.Update(msg)
	cmds = append(cmds, cmd)
	m.context, cmd = m.context.Update(msg)
	cmds = append(cmds, cmd)
	return m, tea.Batch(cmds...)
}

func (m model) handleMouseClickTranslate(msg tea.MouseClickMsg) (tea.Model, tea.Cmd) {
	if msg.Button != tea.MouseLeft || m.holder == nil {
		return m, nil
	}
	// A click on a result tab switches the shown variant.
	for i, r := range m.holder.resultTabs {
		if inRect(r, msg.X, msg.Y) && i < len(m.reqTones) {
			m.setActiveResult(m.reqTones[i])
			cmd := m.setFocus(focusResultTabs)
			return m, cmd
		}
	}
	switch id := hitTest(m.holder.zones, msg.X, msg.Y); id {
	case btnTranslate:
		cmd := m.submit()
		return m, cmd
	case btnClear:
		cmd := m.clearAll()
		return m, cmd
	case btnCopy:
		cmd := m.copyResult()
		return m, cmd
	case btnSetup:
		cmd := m.openSetup()
		return m, cmd
	case btnQuit:
		return m, tea.Quit
	case btnToneLiteral, btnToneNeutral, btnToneDiplomatic:
		m.toggleToneByButton(id)
		cmd := m.setFocus(focusTone)
		return m, cmd
	}
	return m, nil
}

// toggleToneByButton checks or unchecks the tone from the clicked chip's
// button ID and moves the cursor onto it.
func (m *model) toggleToneByButton(id buttonID) {
	for i, opt := range toneOptions {
		if opt.id == id {
			m.toneSelected[opt.tone] = !m.toneSelected[opt.tone]
			m.toneCursor = i
			return
		}
	}
}

// hitTest returns the button whose rendered box contains (x, y), or btnNone.
func hitTest(zones map[buttonID]rect, x, y int) buttonID {
	for id, r := range zones {
		if x >= r.x0 && x <= r.x1 && y >= r.y0 && y <= r.y1 {
			return id
		}
	}
	return btnNone
}

func inRect(r rect, x, y int) bool {
	return x >= r.x0 && x <= r.x1 && y >= r.y0 && y <= r.y1
}

// --- Profile manager ---

// openSetup opens the profile manager in list mode, reloading the store from
// disk so external changes are reflected.
func (m *model) openSetup() tea.Cmd {
	if store, err := config.LoadStore(m.configPath); err == nil {
		m.store = store
	}
	m.screen = screenSetup
	m.setupMode = setupList
	m.setupErr = nil
	m.setupNotice = ""
	m.setupBusy = false
	m.listCursor = m.indexOfActive()
	return nil
}

func (m *model) indexOfActive() int {
	for i, name := range m.store.Names() {
		if name == m.store.Active {
			return i
		}
	}
	return 0
}

// leaveSetup returns to the translate screen and restores focus to Source.
func (m *model) leaveSetup() tea.Cmd {
	if m.setupCancel != nil {
		m.setupCancel()
		m.setupCancel = nil
	}
	m.screen = screenTranslate
	m.setupBusy = false
	m.setupErr = nil
	return m.setFocus(focusSource)
}

func (m model) updateSetup(msg tea.Msg) (tea.Model, tea.Cmd) {
	if rm, ok := msg.(setupResultMsg); ok {
		return m.handleSetupResult(rm)
	}
	if m.setupMode == setupEdit {
		return m.updateSetupEdit(msg)
	}
	return m.updateSetupList(msg)
}

func (m model) handleSetupResult(msg setupResultMsg) (tea.Model, tea.Cmd) {
	if msg.seq != m.setupSeq {
		return m, nil
	}
	m.setupBusy = false
	if msg.err != nil {
		m.setupErr = msg.err
		return m, nil
	}
	translator, err := m.newTranslator(msg.cfg)
	if err != nil {
		m.setupErr = err
		return m, nil
	}
	// TestAndSave persisted the profile and marked it active; reflect that.
	if store, e := config.LoadStore(m.configPath); e == nil {
		m.store = store
	}

	renamed := m.editName != "" && m.editName != msg.name
	if renamed {
		// The edit renamed the profile: TestAndSave saved it under the new
		// name and activated it, so the old name is now a stale duplicate.
		// Drop it (it is neither active nor the last profile after the save).
		delete(m.store.Profiles, m.editName)
		if err := config.SaveStore(m.configPath, m.store); err != nil {
			m.setupErr = err
			return m, nil
		}
	}

	m.translator = translator
	m.activeName = msg.name
	m.modelName = msg.cfg.Model
	m.host = translate.SafeHost(msg.cfg.BaseURL)
	m.setupMode = setupList
	m.setupErr = nil
	if renamed {
		m.setupNotice = fmt.Sprintf("Renamed to %q and activated", msg.name)
	} else {
		m.setupNotice = fmt.Sprintf("Saved and activated %q", msg.name)
	}
	m.listCursor = m.indexOfActive()
	return m, nil
}

func (m model) updateSetupList(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.MouseClickMsg:
		return m.handleMouseClickList(msg)
	case tea.KeyPressMsg:
		names := m.store.Names()
		switch msg.String() {
		case "ctrl+c":
			return m, tea.Interrupt
		case "esc":
			cmd := m.leaveSetup()
			return m, cmd
		case "up", "k":
			if m.listCursor > 0 {
				m.listCursor--
			}
			return m, nil
		case "down", "j":
			if m.listCursor < len(names)-1 {
				m.listCursor++
			}
			return m, nil
		case "enter":
			m.useSelected()
			return m, nil
		case "a":
			m.openEditNew()
			return m, m.setupInputs[editName].Focus()
		case "e":
			cmd := m.openEditSelected()
			return m, cmd
		case "d":
			m.deleteSelected()
			return m, nil
		}
	}
	return m, nil
}

func (m *model) selectedName() (string, bool) {
	names := m.store.Names()
	if m.listCursor < 0 || m.listCursor >= len(names) {
		return "", false
	}
	return names[m.listCursor], true
}

// useSelected makes the highlighted profile active, rebuilds the translator,
// and persists the change.
func (m *model) useSelected() {
	name, ok := m.selectedName()
	if !ok {
		return
	}
	cfg, err := m.resolveProfile(name)
	if err != nil {
		m.setupErr = err
		return
	}
	translator, err := m.newTranslator(cfg)
	if err != nil {
		m.setupErr = err
		return
	}
	m.store.Active = name
	if err := config.SaveStore(m.configPath, m.store); err != nil {
		m.setupErr = err
		return
	}
	m.translator = translator
	m.activeName = name
	m.modelName = cfg.Model
	m.host = translate.SafeHost(cfg.BaseURL)
	m.setupErr = nil
	m.setupNotice = fmt.Sprintf("Active profile is now %q", name)
}

func (m *model) deleteSelected() {
	name, ok := m.selectedName()
	if !ok {
		return
	}
	if err := m.store.Remove(name); err != nil {
		m.setupErr = err
		return
	}
	if err := config.SaveStore(m.configPath, m.store); err != nil {
		m.setupErr = err
		return
	}
	m.setupErr = nil
	m.setupNotice = fmt.Sprintf("Removed %q", name)
	if m.listCursor >= len(m.store.Names()) && m.listCursor > 0 {
		m.listCursor--
	}
}

// resolveProfile returns the resolved effective config for a stored profile,
// applying the QT_* field environment overrides for consistency with batch
// mode.
func (m *model) resolveProfile(name string) (config.Config, error) {
	cfg, ok := m.store.Profiles[name]
	if !ok {
		return config.Config{}, fmt.Errorf("no such profile: %q", name)
	}
	return config.ApplyEnv(cfg, m.getenv).Resolve()
}

func (m *model) openEditNew() {
	m.editName = ""
	m.setupInputs[editName].SetValue("")
	m.setupInputs[editBaseURL].SetValue("")
	m.setupInputs[editModel].SetValue("")
	m.setupInputs[editAPIKey].SetValue("")
	m.beginEdit()
}

func (m *model) openEditSelected() tea.Cmd {
	name, ok := m.selectedName()
	if !ok {
		return nil
	}
	p := m.store.Profiles[name]
	m.editName = name
	m.setupInputs[editName].SetValue(name)
	m.setupInputs[editBaseURL].SetValue(p.BaseURL)
	m.setupInputs[editModel].SetValue(p.Model)
	m.setupInputs[editAPIKey].SetValue("")
	m.beginEdit()
	return m.setupInputs[editName].Focus()
}

func (m *model) beginEdit() {
	m.setupMode = setupEdit
	m.setupErr = nil
	m.setupNotice = ""
	m.setupFocus = editName
	for i := range m.setupInputs {
		m.setupInputs[i].Blur()
	}
}

func (m model) updateSetupEdit(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.MouseClickMsg:
		return m.handleMouseClickEdit(msg)
	case tea.KeyPressMsg:
		switch msg.String() {
		case "ctrl+c":
			return m, tea.Interrupt
		case "esc":
			m.setupMode = setupList
			m.setupErr = nil
			return m, nil
		case "tab":
			cmd := m.cycleEditFocus(1)
			return m, cmd
		case "shift+tab":
			cmd := m.cycleEditFocus(-1)
			return m, cmd
		case "enter":
			return m.activateEditFocused()
		}
	}
	return m.forwardToEditInputs(msg)
}

func (m model) activateEditFocused() (tea.Model, tea.Cmd) {
	switch m.setupFocus {
	case editSaveBtn:
		cmd := m.saveEdit()
		return m, cmd
	case editCancelBtn:
		m.setupMode = setupList
		m.setupErr = nil
		return m, nil
	default:
		cmd := m.cycleEditFocus(1)
		return m, cmd
	}
}

// saveEdit validates the edited profile, then tests and saves it.
func (m *model) saveEdit() tea.Cmd {
	if m.setupBusy {
		return nil
	}
	name := strings.TrimSpace(m.setupInputs[editName].Value())
	if name == "" {
		m.setupErr = fmt.Errorf("profile name must not be empty")
		return nil
	}
	// A blank key keeps the key of the profile being edited (looked up by its
	// original name, so renaming preserves the key). New profiles have no key
	// to fall back to.
	candidate := config.Config{
		BaseURL: m.setupInputs[editBaseURL].Value(),
		Model:   m.setupInputs[editModel].Value(),
		APIKey:  resolveAPIKey(m.setupInputs[editAPIKey].Value(), m.existingKey()),
		// The setup form does not expose the timeout; preserve the edited
		// profile's existing value (0 for a new profile) so it is not wiped.
		TimeoutSeconds: m.store.Profiles[m.editName].TimeoutSeconds,
	}
	if _, err := candidate.Resolve(); err != nil {
		m.setupErr = err
		return nil
	}

	m.setupBusy = true
	m.setupErr = nil
	m.setupSeq++
	seq := m.setupSeq

	ctx, cancel := context.WithCancel(context.Background())
	m.setupCancel = cancel

	return tea.Batch(m.spin.Tick, setupTestCmd(ctx, m.configPath, name, candidate, m.newTranslator, seq))
}

// resolveAPIKey returns input unless it is blank, in which case it keeps the
// existing key, so a blank submission does not clear an existing key.
func resolveAPIKey(input, existingKey string) string {
	if strings.TrimSpace(input) == "" {
		return existingKey
	}
	return input
}

// existingKey returns the stored API key of the profile currently being
// edited, or "" for a new profile. A blank key input keeps this value.
func (m *model) existingKey() string {
	if m.editName == "" {
		return ""
	}
	return m.store.Profiles[m.editName].APIKey
}

func (m *model) cycleEditFocus(dir int) tea.Cmd {
	m.setupFocus = (m.setupFocus + dir + editTargetCount) % editTargetCount
	for i := range m.setupInputs {
		m.setupInputs[i].Blur()
	}
	if m.setupFocus <= editAPIKey {
		return m.setupInputs[m.setupFocus].Focus()
	}
	return nil
}

func (m model) forwardToEditInputs(msg tea.Msg) (tea.Model, tea.Cmd) {
	if m.setupFocus <= editAPIKey {
		var cmd tea.Cmd
		m.setupInputs[m.setupFocus], cmd = m.setupInputs[m.setupFocus].Update(msg)
		return m, cmd
	}
	return m, nil
}

func (m model) handleMouseClickList(msg tea.MouseClickMsg) (tea.Model, tea.Cmd) {
	if msg.Button != tea.MouseLeft || m.holder == nil {
		return m, nil
	}
	// A click on a profile row selects it.
	for i, r := range m.holder.rows {
		if inRect(r, msg.X, msg.Y) {
			m.listCursor = i
			return m, nil
		}
	}
	switch hitTest(m.holder.zones, msg.X, msg.Y) {
	case btnUse:
		m.useSelected()
		return m, nil
	case btnAdd:
		m.openEditNew()
		return m, m.setupInputs[editName].Focus()
	case btnEdit:
		cmd := m.openEditSelected()
		return m, cmd
	case btnDelete:
		m.deleteSelected()
		return m, nil
	case btnBack:
		cmd := m.leaveSetup()
		return m, cmd
	}
	return m, nil
}

func (m model) handleMouseClickEdit(msg tea.MouseClickMsg) (tea.Model, tea.Cmd) {
	if msg.Button != tea.MouseLeft || m.holder == nil {
		return m, nil
	}
	switch hitTest(m.holder.zones, msg.X, msg.Y) {
	case btnSave:
		cmd := m.saveEdit()
		return m, cmd
	case btnCancel:
		m.setupMode = setupList
		m.setupErr = nil
		return m, nil
	}
	return m, nil
}

// resize adjusts every component's dimensions to fit a width×height
// terminal.
func (m *model) resize(width, height int) {
	m.width = width
	m.height = height

	innerWidth := width - 4
	if innerWidth < 10 {
		innerWidth = 10
	}

	// chromeRows counts every rendered line except the flexible Source and
	// Translation box contents: header, blanks, three labels, the Context
	// box (2 borders + 2 content rows), the tone selector row and its blank,
	// the four remaining borders, the Translate button row, and the status and
	// help lines. The extra 1 is a safety margin so the view never exactly
	// fills (and thus scrolls) the screen.
	const chromeRows = 22
	available := height - chromeRows - 1
	// The result-tab row (plus its blank) is shown only when more than one
	// variant was requested; reserve its two lines so the panes don't overflow.
	if len(m.reqTones) > 1 {
		available -= 2
	}
	if available < 6 {
		available = 6
	}

	contextHeight := 2
	sourceHeight := available * 2 / 5
	if sourceHeight < 3 {
		sourceHeight = 3
	}
	resultHeight := available - sourceHeight
	if resultHeight < 3 {
		resultHeight = 3
	}

	m.source.SetWidth(innerWidth)
	m.source.SetHeight(sourceHeight)
	m.context.SetWidth(innerWidth)
	m.context.SetHeight(contextHeight)
	m.result.SetWidth(innerWidth)
	m.result.SetHeight(resultHeight)
	m.setResultContent()

	for i := range m.setupInputs {
		m.setupInputs[i].SetWidth(innerWidth)
	}
}

// setResultContent word-wraps the active tab's translation to the viewport
// width (CSS-style: break at spaces, split a token only when it alone exceeds
// the width) and loads it into the read-only Translation viewport. While the
// active tab is still pending or has failed, the viewport is cleared and the
// status line conveys its state instead.
func (m *model) setResultContent() {
	text := m.activeResultText()
	if text == "" {
		m.result.SetContent("")
		return
	}
	width := m.result.Width()
	if width < 1 {
		// The viewport has no size yet; store the text unwrapped. The
		// WindowSizeMsg that precedes the first real render will re-wrap it.
		m.result.SetContent(text)
		return
	}
	m.result.SetContent(ansi.Wrap(text, width, ""))
}
