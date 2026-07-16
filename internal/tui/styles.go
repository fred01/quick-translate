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

	// Tone selector chips: the selected chip carries a background; unselected
	// chips are plain text. The row focused/blurred distinction brightens the
	// selected chip (and slightly brightens unselected ones) when the tone row
	// holds focus, so keyboard users can see it is active.
	styleToneSelected        = lipgloss.NewStyle().Padding(0, 1).Foreground(lipgloss.Color("15")).Background(lipgloss.Color("238"))
	styleToneSelectedFocused = lipgloss.NewStyle().Padding(0, 1).Foreground(lipgloss.Color("0")).Background(lipgloss.Color("6")).Bold(true)
	styleToneChip            = lipgloss.NewStyle().Padding(0, 1).Foreground(lipgloss.Color("245"))
	styleToneChipFocused     = lipgloss.NewStyle().Padding(0, 1).Foreground(lipgloss.Color("252"))
)

// renderToneChip renders one tone option. selected marks the active tone;
// rowFocused marks whether the tone selector currently holds keyboard focus.
func renderToneChip(label string, selected, rowFocused bool) string {
	switch {
	case selected && rowFocused:
		return styleToneSelectedFocused.Render(label)
	case selected:
		return styleToneSelected.Render(label)
	case rowFocused:
		return styleToneChipFocused.Render(label)
	default:
		return styleToneChip.Render(label)
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
