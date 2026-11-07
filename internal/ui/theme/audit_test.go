package theme_test

import (
	"image/color"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sorokin-vladimir/tele/internal/ui/theme"
)

// legible is tele-dark on the canvas it was tuned for, with the body text the
// canvas requires. The audit finds nothing in it, so anything a test finds is
// what that test put there.
func legible(t *testing.T) theme.Theme {
	t.Helper()
	th := theme.TeleDark
	th.Background = mustColor(t, "#000000")
	th.Text = mustColor(t, "#cdd6f4")
	require.Empty(t, theme.Audit(th), "the fixture must start clean")
	return th
}

func mustColor(t *testing.T, s string) color.Color {
	t.Helper()
	c, err := theme.ParseColor(s)
	require.NoError(t, err)
	return c
}

// Without a canvas there is nothing to measure against, and the terminal's
// colour is not knowable here. An empty result is the absence of a question, not
// a clean bill of health.
func TestAudit_SaysNothingAboutAThemeWithNoCanvas(t *testing.T) {
	assert.Empty(t, theme.Audit(theme.TeleDark))
	assert.Empty(t, theme.Audit(theme.TeleLight))
}

// The case the whole thing exists for: a token inherited from a palette tuned
// for the opposite background.
func TestAudit_FindsAForegroundTokenThatCannotBeReadOnTheCanvas(t *testing.T) {
	th := legible(t)
	th.StatusOnline = mustColor(t, "#0b0b0b")

	found := theme.Audit(th)

	require.Len(t, found, 1)
	assert.Equal(t, "status_online", found[0].Token)
	assert.Less(t, found[0].Ratio, 3.0)
	assert.False(t, found[0].Unset)
}

// text is guarded by the background/text dependency, but only for being set. A
// dependency cannot tell #333 on #222 from anything else.
func TestAudit_JudgesTheBodyTextTheDependencyOnlyRequiresToExist(t *testing.T) {
	th := legible(t)
	th.Background = mustColor(t, "#222222")
	th.Text = mustColor(t, "#333333")

	assert.Contains(t, tokensOf(theme.Audit(th)), "text")
}

// A token left at none takes the terminal's own foreground, which a claimed
// canvas has no relation to. There is nothing to measure and nothing to be
// reassured by, so it is reported as its own kind of problem.
func TestAudit_ReportsAnUnsetTokenInsteadOfMeasuringIt(t *testing.T) {
	th := legible(t)
	th.TextDim = mustColor(t, "none")

	found := theme.Audit(th)

	require.Len(t, found, 1)
	assert.Equal(t, "text_dim", found[0].Token)
	assert.True(t, found[0].Unset)
	assert.Zero(t, found[0].Ratio, "nothing was measured, so no ratio may be implied")
}

// One unreadable entry means every Nth person in a group has an invisible name,
// and the author has to be told which entry rather than that the list is wrong.
func TestAudit_NamesAPaletteEntryByItsIndex(t *testing.T) {
	th := legible(t)
	th.SenderPalette = []color.Color{
		mustColor(t, "#cdd6f4"),
		mustColor(t, "#0b0b0b"),
	}

	found := theme.Audit(th)

	require.Len(t, found, 1)
	assert.Equal(t, "sender_palette[1]", found[0].Token)
	assert.True(t, found[0].Palette)
}

// Worst first, because 1.4:1 is invisible and 2.8:1 is merely quiet. The
// unmeasured go last as a group: they have no ratio to be ranked by.
func TestAudit_OrdersWorstFirstWithTheUnmeasuredLast(t *testing.T) {
	th := legible(t)
	th.StatusOnline = mustColor(t, "#2a2a2a") // the darker of the two
	th.StatusInfo = mustColor(t, "#4a4a4a")
	th.TextFaint = mustColor(t, "none")

	assert.Equal(t, []string{"status_online", "status_info", "text_faint"},
		tokensOf(theme.Audit(th)))
}

// A notice per finding would be twenty-six toasts at fifteen seconds each, which
// is a punishment rather than a notification. One line, and the command that
// prints the rest.
func TestAudit_SummarisesInOneWarningAndNamesWhereTheDetailIs(t *testing.T) {
	dir := themesDir(t, map[string]string{
		"mine.yml": "background: \"#ffffff\"\ntext: \"#000000\"\n",
	})
	l := theme.NewLoader(dir)

	l.Resolve("mine", theme.TeleDark)

	warnings := l.Warnings()
	require.Len(t, warnings, 1, "a warning per finding is what this must not be")
	assert.Contains(t, warnings[0], "theme mine:")
	assert.Contains(t, warnings[0], "unreadable on its canvas")
	assert.Contains(t, warnings[0], "tele theme check")
}

// sender_palette is one token holding a list. Three bad entries are one badly
// chosen token, and a count that says "tokens" has to mean what the file means.
func TestAudit_CountsPaletteEntriesApartFromTokens(t *testing.T) {
	dir := themesDir(t, map[string]string{
		"mine.yml": "background: \"#ffffff\"\ntext: \"#000000\"\n" +
			"sender_palette: [\"#fafafa\", \"#f5f5f5\"]\n",
	})
	l := theme.NewLoader(dir)

	l.Resolve("mine", theme.TeleDark)

	require.Len(t, l.Warnings(), 1)
	assert.Contains(t, l.Warnings()[0], "2 sender_palette entries")
	assert.NotContains(t, l.Warnings()[0], "sender_palette[",
		"the summary counts; the report names")
}

// An unmeasured token cannot be folded into the count of measured ones without
// claiming a ratio nobody took.
func TestAudit_SaysSeparatelyWhatItCouldNotMeasure(t *testing.T) {
	dir := themesDir(t, map[string]string{
		"mine.yml": "background: \"#ffffff\"\ntext: \"#000000\"\ntext_dim: none\n",
	})
	l := theme.NewLoader(dir)

	l.Resolve("mine", theme.TeleDark)

	require.Len(t, l.Warnings(), 1)
	assert.Contains(t, l.Warnings()[0], "1 more takes the terminal's foreground")
}

// Putting one theme in both slots is the configuration the documentation
// recommends. It resolves that theme twice, and every problem it has would
// otherwise be reported twice — two identical toasts, fifteen seconds each.
func TestLoader_WarnsOnceWhenOneThemeFillsBothSlots(t *testing.T) {
	dir := themesDir(t, map[string]string{
		"mine.yml": "background: \"#ffffff\"\ntext: \"#000000\"\n",
	})

	loaded := theme.LoadSlots(dir, "mine", "mine")

	assert.Len(t, loaded.Warnings, 1)
}

// The findings belong to the resolution, so whatever prints the provenance can
// print them without resolving the theme a second time and risking a different
// answer.
func TestResolve_CarriesTheFindingsOnTheResolution(t *testing.T) {
	dir := themesDir(t, map[string]string{
		"mine.yml": "background: \"#ffffff\"\ntext: \"#000000\"\n",
	})

	got := theme.NewLoader(dir).Resolve("mine", theme.TeleDark)

	assert.NotEmpty(t, got.Findings)
	assert.Empty(t, theme.NewLoader(dir).Resolve("", theme.TeleDark).Findings,
		"a theme with no canvas was asked nothing")
}

// A canvas the dependency refused is not a canvas anything is drawn on, so
// measuring against it would report a screen nobody will see.
func TestAudit_IgnoresACanvasTheDependencyRefused(t *testing.T) {
	dir := themesDir(t, map[string]string{
		"mine.yml": "background: \"#ffffff\"\n", // no text: the canvas is cleared
	})
	l := theme.NewLoader(dir)

	got := l.Resolve("mine", theme.TeleDark)

	assert.Empty(t, got.Findings)
	require.Len(t, l.Warnings(), 1)
	assert.Contains(t, l.Warnings()[0], "is set but", "only the dependency has anything to say")
}

// The source column is what turns a finding into an edit: a token from the
// built-in has to be added to the file, and one the author wrote has to be
// changed there.
func TestReport_ListsOffendersWorstFirstWithTheThemeThatSetThem(t *testing.T) {
	dir := themesDir(t, map[string]string{
		"mine.yml": "background: \"#ffffff\"\ntext: \"#000000\"\nstatus_online: \"#fdfdfd\"\n",
	})

	got := theme.Report("dark", theme.NewLoader(dir).Resolve("mine", theme.TeleDark))

	assert.Contains(t, got, "canvas #ffffff:")
	assert.Contains(t, got, "status_online")
	assert.Contains(t, got, "from mine", "a token the author wrote is theirs to change")
	assert.Contains(t, got, "from tele-dark", "an inherited one has to be added to the file")

	lines := offenderLines(got)
	require.Greater(t, len(lines), 1)
	assert.Contains(t, lines[0], "status_online",
		"the worst offender leads: it is the one that is invisible rather than quiet")
}

// Nothing was asked of a theme with no canvas, so the report must not imply an
// answer.
func TestReport_SaysNothingAboutLegibilityWithoutACanvas(t *testing.T) {
	got := theme.Report("dark", theme.NewLoader(t.TempDir()).Resolve("", theme.TeleDark))

	assert.NotContains(t, got, "canvas")
}

func tokensOf(fs []theme.Finding) []string {
	out := make([]string, 0, len(fs))
	for _, f := range fs {
		out = append(out, f.Token)
	}
	return out
}

// offenderLines returns the indented per-token lines of a report's findings
// block, which are the only ones indented four spaces.
func offenderLines(report string) []string {
	var out []string
	for _, line := range strings.Split(report, "\n") {
		if strings.HasPrefix(line, "    ") {
			out = append(out, line)
		}
	}
	return out
}
