package tui

import (
	"context"
	"fmt"
	"strings"
	"time"

	"charm.land/bubbles/v2/spinner"
	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/fred01/quick-translate/internal/config"
	"github.com/fred01/quick-translate/internal/translate"
)

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
	case statusMsg:
		if msg.seq != m.reqSeq {
			return m, nil
		}
		m.stage = msg.status.Stage
		return m, nil

	case resultMsg:
		if msg.seq != m.reqSeq {
			return m, nil
		}
		m.busy = false
		m.lastElapsed = msg.elapsed
		if msg.err != nil {
			m.err = msg.err
			return m, nil
		}
		m.err = nil
		m.cancelled = false
		m.resultText = msg.text
		m.lastOutputChars = len([]rune(msg.text))
		m.setResultContent()
		m.result.GotoTop()
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
			m.err = nil
		}
		return nil, true
	case "tab":
		return m.cycleFocus(1), true
	case "shift+tab":
		return m.cycleFocus(-1), true
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
	case focusSource, focusContext, focusTranslateBtn:
		return m.submit()
	case focusCopyBtn:
		return m.copyResult()
	}
	return nil
}

// submit starts a new translation request, unless one is already active or
// Source is empty or whitespace-only.
func (m *model) submit() tea.Cmd {
	if m.busy {
		return nil
	}
	source := m.source.Value()
	if strings.TrimSpace(source) == "" {
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
	m.err = nil
	m.everSubmitted = true
	m.stage = translate.StagePreparing
	m.requestStarted = time.Now()

	input := translate.TranslationInput{Source: source, Context: m.context.Value()}
	reporter := &programReporter{program: m.holder.program, seq: seq}

	return tea.Batch(m.spin.Tick, translateCmd(ctx, m.translator, input, reporter, seq))
}

// copyResult copies the current translation to the system clipboard via
// OSC52. It does nothing when there is no result yet.
func (m *model) copyResult() tea.Cmd {
	if m.resultText == "" {
		return nil
	}
	m.copied = true
	return tea.SetClipboard(m.resultText)
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

// focusOrder returns the focusable elements in visual Tab order. Copy is
// only focusable once a translation exists. Setup and Quit are deliberately
// excluded — they are mouse-only.
func (m *model) focusOrder() []focusTarget {
	order := []focusTarget{focusSource, focusContext, focusTranslateBtn}
	if m.resultText != "" {
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
	switch hitTest(m.holder.zones, msg.X, msg.Y) {
	case btnTranslate:
		cmd := m.submit()
		return m, cmd
	case btnCopy:
		cmd := m.copyResult()
		return m, cmd
	case btnSetup:
		cmd := m.openSetup()
		return m, cmd
	case btnQuit:
		return m, tea.Quit
	}
	return m, nil
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
	// box (2 borders + 2 content rows), the four remaining borders, the
	// Translate button row, and the status and help lines. The extra 1 is a
	// safety margin so the view never exactly fills (and thus scrolls) the
	// screen.
	const chromeRows = 20
	available := height - chromeRows - 1
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

// setResultContent word-wraps the current translation to the viewport width
// (CSS-style: break at spaces, split a token only when it alone exceeds the
// width) and loads it into the read-only Translation viewport.
func (m *model) setResultContent() {
	if m.resultText == "" {
		return
	}
	width := m.result.Width()
	if width < 1 {
		// The viewport has no size yet; store the text unwrapped. The
		// WindowSizeMsg that precedes the first real render will re-wrap it.
		m.result.SetContent(m.resultText)
		return
	}
	m.result.SetContent(ansi.Wrap(m.resultText, width, ""))
}
