package theme

import "charm.land/lipgloss/v2"

// QRStyle paints a drawn QR code dark on light whatever the theme. It is not a
// token: a scanner reads dark modules on a light ground, and a code inverted
// to suit a dark canvas is one some of them refuse, so no theme may change it
// and no audit judges it (#330).
func QRStyle() lipgloss.Style {
	return NewStyle().
		Foreground(lipgloss.Color("#000000")).
		Background(lipgloss.Color("#ffffff"))
}
