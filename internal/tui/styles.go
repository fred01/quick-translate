package tui

import "charm.land/lipgloss/v2"

var (
	styleHeaderLeft  = lipgloss.NewStyle().Bold(true)
	styleHeaderRight = lipgloss.NewStyle().Faint(true)
	styleLabel       = lipgloss.NewStyle().Bold(true)
	styleBoxFocused  = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("6")).Padding(0, 1)
	styleBoxBlurred  = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("240")).Padding(0, 1)
	styleHelp        = lipgloss.NewStyle().Faint(true)
	styleError       = lipgloss.NewStyle().Foreground(lipgloss.Color("9"))
	styleActiveMark  = lipgloss.NewStyle().Foreground(lipgloss.Color("6"))
	styleCursorRow   = lipgloss.NewStyle().Bold(true)

	styleButtonBlurred  = lipgloss.NewStyle().Padding(0, 1).Foreground(lipgloss.Color("15")).Background(lipgloss.Color("238"))
	styleButtonFocused  = lipgloss.NewStyle().Padding(0, 1).Foreground(lipgloss.Color("0")).Background(lipgloss.Color("6")).Bold(true)
	styleButtonDisabled = lipgloss.NewStyle().Padding(0, 1).Foreground(lipgloss.Color("240")).Background(lipgloss.Color("236"))

	// Tone checkboxes: the "[x]"/"[ ]" glyph carries the checked state, so a
	// checked box is shown in normal text and an unchecked one is faint. The
	// cursor (the box under keyboard focus) gets the focused-button background
	// regardless of its checked state.
	styleToneChecked   = lipgloss.NewStyle().Padding(0, 1).Foreground(lipgloss.Color("15"))
	styleToneUnchecked = lipgloss.NewStyle().Padding(0, 1).Foreground(lipgloss.Color("245"))

	// styleTabActive marks the active result tab when the tab row is not
	// focused (a background without the focused-button's bold cyan).
	styleTabActive = lipgloss.NewStyle().Padding(0, 1).Foreground(lipgloss.Color("15")).Background(lipgloss.Color("238"))
)

// renderToneCheckbox renders one tone option as a checkbox. checked reflects
// whether the tone is selected; cursor marks whether the tone row holds focus
// with this box under the cursor.
func renderToneCheckbox(label string, checked, cursor bool) string {
	mark := "[ ] "
	if checked {
		mark = "[x] "
	}
	s := mark + label
	switch {
	case cursor:
		return styleButtonFocused.Render(s)
	case checked:
		return styleToneChecked.Render(s)
	default:
		return styleToneUnchecked.Render(s)
	}
}

// renderButton renders a single-line button in one of three visual states.
func renderButton(label string, focused, disabled bool) string {
	switch {
	case disabled:
		return styleButtonDisabled.Render(label)
	case focused:
		return styleButtonFocused.Render(label)
	default:
		return styleButtonBlurred.Render(label)
	}
}
