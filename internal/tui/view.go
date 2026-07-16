package tui

import (
	"fmt"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/fred01/quick-translate/internal/prompt"
	"github.com/fred01/quick-translate/internal/translate"
)

// toneOptions is the left-to-right order, labels, and hit-test IDs of the tone
// selector. It is the single source of truth shared by the renderer, the
// keyboard cycler, and the mouse handler.
var toneOptions = []struct {
	tone  prompt.Tone
	id    buttonID
	label string
}{
	{prompt.ToneLiteral, btnToneLiteral, "Literal"},
	{prompt.ToneNeutral, btnToneNeutral, "Neutral"},
	{prompt.ToneDiplomatic, btnToneDiplomatic, "Diplomatic"},
}

// View implements tea.Model. It renders the current screen and records the
// rendered hit-boxes into the shared holder so the mouse handler can resolve
// clicks on the next update.
func (m model) View() tea.View {
	var content string
	var zones map[buttonID]rect
	var rows []rect

	switch {
	case m.screen == screenSetup && m.setupMode == setupEdit:
		content, zones = m.buildSetupEditView()
	case m.screen == screenSetup:
		content, zones, rows = m.buildSetupListView()
	default:
		content, zones = m.buildTranslateView()
	}

	if m.holder != nil {
		m.holder.zones = zones
		m.holder.rows = rows
	}

	v := tea.NewView(content)
	v.MouseMode = tea.MouseModeCellMotion
	// Own the whole screen so the header (with the Setup button) is always at
	// the top and never scrolls off, and so mouse coordinates line up with
	// the rendered layout for hit-testing.
	v.AltScreen = true
	return v
}

func appendLines(lines []string, block string) []string {
	return append(lines, strings.Split(block, "\n")...)
}

func (m model) box(content string, focused bool) string {
	if focused {
		return styleBoxFocused.Render(content)
	}
	return styleBoxBlurred.Render(content)
}

func (m model) rowWidth() int {
	if m.width > 0 {
		return m.width
	}
	return 80
}

// --- Translate screen ---

func (m model) buildTranslateView() (string, map[buttonID]rect) {
	zones := map[buttonID]rect{}
	var lines []string

	header, headerZones := m.renderHeader()
	for id, r := range headerZones {
		zones[id] = r
	}
	lines = appendLines(lines, header)
	lines = appendLines(lines, "")

	lines = appendLines(lines, " "+styleLabel.Render("Context (optional)"))
	lines = appendLines(lines, m.box(m.context.View(), m.focus == focusContext))
	lines = appendLines(lines, "")

	toneRow, toneZones := m.renderToneSelector(len(lines))
	for id, r := range toneZones {
		zones[id] = r
	}
	lines = appendLines(lines, toneRow)
	lines = appendLines(lines, "")

	lines = appendLines(lines, " "+styleLabel.Render("Source"))
	lines = appendLines(lines, m.box(m.source.View(), m.focus == focusSource))
	lines = appendLines(lines, "")

	translateBtn := renderButton("Translate", m.focus == focusTranslateBtn, false)
	btnY := len(lines)
	zones[btnTranslate] = rect{x0: 1, y0: btnY, x1: lipgloss.Width(translateBtn), y1: btnY}
	lines = appendLines(lines, " "+translateBtn)
	lines = appendLines(lines, "")

	lines = appendLines(lines, " "+styleLabel.Render("Translation"))
	lines = appendLines(lines, m.box(m.result.View(), false))
	lines = appendLines(lines, "")

	copyBtn := renderButton("Copy", m.focus == focusCopyBtn, m.resultText == "")
	statusY := len(lines)
	zones[btnCopy] = rect{x0: 1, y0: statusY, x1: lipgloss.Width(copyBtn), y1: statusY}
	status := m.statusText()
	if m.busy {
		status = m.spin.View() + " " + status
	}
	lines = appendLines(lines, " "+copyBtn+"  "+status)

	lines = appendLines(lines, m.helpLine())

	return strings.Join(lines, "\n"), zones
}

// renderToneSelector renders the "Tone" label and the three tone chips on a
// single line at row y, returning that line and the chip hit-boxes.
func (m model) renderToneSelector(y int) (string, map[buttonID]rect) {
	zones := map[buttonID]rect{}
	focused := m.focus == focusTone

	label := " " + styleLabel.Render("Tone")
	var b strings.Builder
	b.WriteString(label)
	b.WriteString("  ")
	x := lipgloss.Width(label) + 2
	for i, opt := range toneOptions {
		chip := renderToneChip(opt.label, m.tone == opt.tone, focused)
		w := lipgloss.Width(chip)
		zones[opt.id] = rect{x0: x, y0: y, x1: x + w - 1, y1: y}
		b.WriteString(chip)
		x += w
		if i < len(toneOptions)-1 {
			b.WriteString(" ")
			x++
		}
	}
	return b.String(), zones
}

func (m model) renderHeader() (string, map[buttonID]rect) {
	left := styleHeaderLeft.Render(" qt · quick translate")
	setupBtn := renderButton("Setup", false, false)
	quitBtn := renderButton("Quit", false, false)
	width := m.rowWidth()

	// The Setup and Quit buttons (mouse-only) are anchored to the right edge
	// and always drawn; the endpoint label fills the middle and is truncated
	// so the whole line never exceeds the terminal width, which would clip
	// the buttons.
	leftW := lipgloss.Width(left)
	setupW := lipgloss.Width(setupBtn)
	quitW := lipgloss.Width(quitBtn)
	buttonsW := setupW + 1 + quitW // one space between the two buttons
	const sep = 2                  // spaces between the endpoint label and the buttons
	avail := width - leftW - buttonsW - sep - 1
	endpoint := ""
	if avail > 0 {
		endpoint = styleHeaderRight.Render(ansi.Truncate(m.endpointLabel(), avail, "…"))
	}

	pad := width - leftW - 1 - lipgloss.Width(endpoint) - buttonsW
	if pad < 1 {
		pad = 1
	}
	header := left + " " + endpoint + strings.Repeat(" ", pad) + setupBtn + " " + quitBtn

	setupX0 := leftW + 1 + lipgloss.Width(endpoint) + pad
	quitX0 := setupX0 + setupW + 1
	zones := map[buttonID]rect{
		btnSetup: {x0: setupX0, y0: 0, x1: setupX0 + setupW - 1, y1: 0},
		btnQuit:  {x0: quitX0, y0: 0, x1: quitX0 + quitW - 1, y1: 0},
	}
	return header, zones
}

// endpointLabel shows the active profile (if named) with its model and host,
// e.g. "LiteLLM [translategemma @ litellm.svc.example.com]".
func (m model) endpointLabel() string {
	if m.activeName != "" {
		return fmt.Sprintf("%s [%s @ %s]", m.activeName, m.modelName, m.host)
	}
	return fmt.Sprintf("%s @ %s", m.modelName, m.host)
}

func (m model) statusText() string {
	switch {
	case m.busy:
		switch m.stage {
		case translate.StagePreparing:
			return "Preparing request…"
		case translate.StageSending:
			return fmt.Sprintf("Sending to %s · %s", m.host, m.modelName)
		default:
			return fmt.Sprintf("Waiting for response… %s", formatElapsed(time.Since(m.requestStarted).Seconds()))
		}
	case m.cancelled:
		return "Cancelled"
	case m.err != nil:
		return "Error: " + m.err.Error()
	case m.everSubmitted:
		s := fmt.Sprintf("Received %d chars · %s", m.lastOutputChars, formatElapsed(m.lastElapsed))
		if m.copied {
			s += " · copied to clipboard"
		}
		return s
	case m.notice != "":
		return m.notice
	default:
		return "Ready"
	}
}

func (m model) helpLine() string {
	newlineHelp := "Alt+Enter newline"
	if m.keyDisambiguation {
		newlineHelp = "Shift+Enter newline"
	}
	return styleHelp.Render(fmt.Sprintf(" Enter translate · %s · Tab field · ←→ tone · Ctrl+C quit", newlineHelp))
}

// --- Profile manager: list mode ---

func (m model) buildSetupListView() (string, map[buttonID]rect, []rect) {
	zones := map[buttonID]rect{}
	var lines []string

	lines = appendLines(lines, styleHeaderLeft.Render(" qt · profiles"))
	lines = appendLines(lines, "")

	names := m.store.Names()
	rows := make([]rect, len(names))
	if len(names) == 0 {
		lines = appendLines(lines, styleHelp.Render("   (no profiles yet — press \"a\" to add one)"))
	}
	for i, name := range names {
		cursor := "  "
		if i == m.listCursor {
			cursor = "› "
		}
		mark := "  "
		if name == m.store.Active {
			mark = styleActiveMark.Render("● ")
		}
		p := m.store.Profiles[name]
		label := fmt.Sprintf("%-16s %s @ %s", name, p.Model, translate.SafeHost(p.BaseURL))
		if i == m.listCursor {
			label = styleCursorRow.Render(label)
		}
		y := len(lines)
		rows[i] = rect{x0: 0, y0: y, x1: m.rowWidth() - 1, y1: y}
		lines = appendLines(lines, " "+cursor+mark+label)
	}
	lines = appendLines(lines, "")

	// Action button row.
	btnY := len(lines) + 0
	specs := []struct {
		id    buttonID
		label string
	}{
		{btnUse, "Use"},
		{btnAdd, "Add"},
		{btnEdit, "Edit"},
		{btnDelete, "Delete"},
		{btnBack, "Back"},
	}
	x := 1
	var b strings.Builder
	b.WriteString(" ")
	for i, s := range specs {
		btn := renderButton(s.label, false, false)
		zones[s.id] = rect{x0: x, y0: btnY, x1: x + lipgloss.Width(btn) - 1, y1: btnY}
		b.WriteString(btn)
		x += lipgloss.Width(btn)
		if i < len(specs)-1 {
			b.WriteString("  ")
			x += 2
		}
	}
	lines = appendLines(lines, b.String())
	lines = appendLines(lines, "")

	lines = appendLines(lines, m.setupListStatusLine())

	return strings.Join(lines, "\n"), zones, rows
}

func (m model) setupListStatusLine() string {
	switch {
	case m.setupErr != nil:
		return " " + styleError.Render("Error: "+m.setupErr.Error())
	case m.setupNotice != "":
		return " " + m.setupNotice
	default:
		return styleHelp.Render(" ↑↓ move · Enter use · a add · e edit · d delete · Esc back")
	}
}

// --- Profile manager: editor mode ---

func (m model) buildSetupEditView() (string, map[buttonID]rect) {
	zones := map[buttonID]rect{}
	var lines []string

	title := "add profile"
	if m.editName != "" {
		title = "edit profile " + m.editName
	}
	lines = appendLines(lines, styleHeaderLeft.Render(" qt · "+title))
	lines = appendLines(lines, "")

	lines = appendLines(lines, " "+styleLabel.Render("Profile name"))
	lines = appendLines(lines, m.box(m.setupInputs[editName].View(), m.setupFocus == editName))
	lines = appendLines(lines, "")

	lines = appendLines(lines, " "+styleLabel.Render("Base URL")+styleHelp.Render("  (OpenAI-compatible API root, must include /v1)"))
	lines = appendLines(lines, m.box(m.setupInputs[editBaseURL].View(), m.setupFocus == editBaseURL))
	lines = appendLines(lines, "")

	lines = appendLines(lines, " "+styleLabel.Render("Model"))
	lines = appendLines(lines, m.box(m.setupInputs[editModel].View(), m.setupFocus == editModel))
	lines = appendLines(lines, "")

	apiKeyHelp := "  (required)"
	if m.existingKey() != "" {
		apiKeyHelp = "  (blank keeps the current key)"
	}
	lines = appendLines(lines, " "+styleLabel.Render("API Key")+styleHelp.Render(apiKeyHelp))
	lines = appendLines(lines, m.box(m.setupInputs[editAPIKey].View(), m.setupFocus == editAPIKey))
	lines = appendLines(lines, "")

	saveBtn := renderButton("Save", m.setupFocus == editSaveBtn, false)
	cancelBtn := renderButton("Cancel", m.setupFocus == editCancelBtn, false)
	btnY := len(lines)
	saveX0 := 1
	cancelX0 := saveX0 + lipgloss.Width(saveBtn) + 3
	zones[btnSave] = rect{x0: saveX0, y0: btnY, x1: saveX0 + lipgloss.Width(saveBtn) - 1, y1: btnY}
	zones[btnCancel] = rect{x0: cancelX0, y0: btnY, x1: cancelX0 + lipgloss.Width(cancelBtn) - 1, y1: btnY}
	lines = appendLines(lines, " "+saveBtn+"   "+cancelBtn)
	lines = appendLines(lines, "")

	lines = appendLines(lines, m.setupEditStatusLine())

	return strings.Join(lines, "\n"), zones
}

func (m model) setupEditStatusLine() string {
	switch {
	case m.setupBusy:
		return " " + m.spin.View() + " Testing endpoint…"
	case m.setupErr != nil:
		return " " + styleError.Render("Error: "+m.setupErr.Error())
	default:
		return styleHelp.Render(" Tab field · Enter save · Esc cancel")
	}
}

func formatElapsed(seconds float64) string {
	return fmt.Sprintf("%.2fs", seconds)
}
