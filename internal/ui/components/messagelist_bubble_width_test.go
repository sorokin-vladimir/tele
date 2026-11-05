package components_test

import (
	"strings"
	"testing"
	"time"

	xansi "github.com/charmbracelet/x/ansi"
	runewidth "github.com/mattn/go-runewidth"
	"github.com/sorokin-vladimir/tele/internal/domain"
	"github.com/sorokin-vladimir/tele/internal/ui/components"
	"github.com/stretchr/testify/require"
)

// A base glyph followed by VS16 (✍️) is one cell in the terminal this app runs
// in, while Unicode counts it as two. Measured by Unicode, the rows carrying it
// end one cell short of their right border. Every body row must end on the same
// cell column as the border. The terminal's rule is measured here directly, not
// through the function under test.
func TestMessageList_BodyRowsEndOnTheBorderColumn(t *testing.T) {
	text := "📖 Reading day.html\n" +
		"✍️ Writing /srv/app/payroll-web/payroll...\n" +
		"📖 Reading day.html L46\n" +
		"✍️ Writing /srv/app/payroll-web/payroll...\n" +
		"🔧 Editing /srv/app/payroll-web/static/...\n" +
		"🔧 Editing /srv/app/payroll-web/payroll..."
	ml := components.NewMessageList(40, 80)
	ml.SetMessages([]domain.Message{{ID: 1, ChatID: 1, Text: text, Date: time.Now()}})

	var rows []string
	for _, ln := range strings.Split(xansi.Strip(ml.View()), "\n") {
		if strings.HasPrefix(ln, "│") || strings.Contains(ln, "│") {
			rows = append(rows, ln)
		}
	}
	require.NotEmpty(t, rows)
	var col = -1
	for _, r := range rows {
		if !strings.Contains(r, "Writing") && !strings.Contains(r, "Reading") && !strings.Contains(r, "Editing") {
			continue
		}
		i := strings.LastIndex(r, "│")
		c := runewidth.StringWidth(r[:i])
		if col == -1 {
			col = c
		}
		require.Equal(t, col, c, "row %q ends its border off the column", r)
	}
	require.NotEqual(t, -1, col, "no body row was found")
}
