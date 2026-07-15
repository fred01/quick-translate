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
)

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
