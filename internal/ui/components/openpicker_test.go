package components_test

import (
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/sorokin-vladimir/tele/internal/ui/components"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func openTargets() []components.OpenTarget {
	return []components.OpenTarget{
		{Kind: components.OpenTargetPhoto, Label: "Photo"},
		{Kind: components.OpenTargetLink, Label: "https://a.com", URL: "https://a.com"},
		{Kind: components.OpenTargetLink, Label: "b@c.com", URL: "mailto:b@c.com"},
	}
}

func TestOpenPicker_View_ListsNumberedTargets(t *testing.T) {
	p := components.NewOpenPicker(openTargets(), 60)
	view := strip(p.View())
	assert.Contains(t, view, "Photo")
	assert.Contains(t, view, "https://a.com")
	assert.Contains(t, view, "b@c.com")
}

func TestOpenPicker_Enter_ChoosesCursorTarget(t *testing.T) {
	p := components.NewOpenPicker(openTargets(), 60)
	newP, cmd := p.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	assert.Nil(t, newP)
	require.NotNil(t, cmd)
	msg, ok := cmd().(components.OpenTargetChosenMsg)
	require.True(t, ok)
	assert.Equal(t, components.OpenTargetPhoto, msg.Target.Kind)
}

func TestOpenPicker_J_ThenEnter_ChoosesSecond(t *testing.T) {
	p := components.NewOpenPicker(openTargets(), 60)
	p, _ = p.Update(tea.KeyPressMsg{Code: 'j', Text: "j"})
	require.NotNil(t, p)
	newP, cmd := p.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	assert.Nil(t, newP)
	require.NotNil(t, cmd)
	msg := cmd().(components.OpenTargetChosenMsg)
	assert.Equal(t, "https://a.com", msg.Target.URL)
}

func TestOpenPicker_CyrillicLayout_Navigates(t *testing.T) {
	p := components.NewOpenPicker(openTargets(), 60)
	// Russian ЙЦУКЕН: the physical "j" key produces "о"; navigation must still work.
	p, _ = p.Update(tea.KeyPressMsg{Code: 'о', Text: "о"})
	require.NotNil(t, p)
	assert.Equal(t, 1, p.Cursor())
	// Physical "k" produces "л"; moves back up.
	p, _ = p.Update(tea.KeyPressMsg{Code: 'л', Text: "л"})
	require.NotNil(t, p)
	assert.Equal(t, 0, p.Cursor())
}

func TestOpenPicker_Digit_ChoosesThatTarget(t *testing.T) {
	p := components.NewOpenPicker(openTargets(), 60)
	newP, cmd := p.Update(tea.KeyPressMsg{Code: '3', Text: "3"})
	assert.Nil(t, newP)
	require.NotNil(t, cmd)
	msg := cmd().(components.OpenTargetChosenMsg)
	assert.Equal(t, "mailto:b@c.com", msg.Target.URL)
}

func TestOpenPicker_AccentsDigits(t *testing.T) {
	p := components.NewOpenPicker(openTargets(), 60)
	// Row 0 is selected (plain); a non-selected row's number is accent-colored.
	raw := p.View()
	assert.Regexp(t, `38;5;39[^m]*m2`, raw, "the picker number must be accent-colored")
}

func TestOpenPicker_Esc_Closes(t *testing.T) {
	p := components.NewOpenPicker(openTargets(), 60)
	newP, cmd := p.Update(tea.KeyPressMsg{Code: tea.KeyEsc})
	assert.Nil(t, newP)
	require.NotNil(t, cmd)
	assert.IsType(t, components.CloseOpenPickerMsg{}, cmd())
}

func TestOpenPicker_LongLabel_TruncatedWithTail(t *testing.T) {
	long := "https://example.com/very/long/path/to/resource.html"
	p := components.NewOpenPicker([]components.OpenTarget{
		{Kind: components.OpenTargetLink, Label: long, URL: long},
	}, 24)
	view := strip(p.View())
	assert.NotContains(t, view, "very/long/path/to", "middle must be elided")
	assert.Contains(t, view, "…")
	assert.Contains(t, view, ".html", "the tail must be preserved")
}
