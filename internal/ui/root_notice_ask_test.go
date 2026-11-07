package ui

import (
	"context"
	"errors"
	"testing"

	tea "charm.land/bubbletea/v2"
	xansi "github.com/charmbracelet/x/ansi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sorokin-vladimir/tele/internal/config"
	"github.com/sorokin-vladimir/tele/internal/notices"
	"github.com/sorokin-vladimir/tele/internal/ui/screens"
)

// askingNotice is a notice that asks, and counts how often it was told yes.
func askingNotice(yes *int) notices.Notice {
	return notices.Notice{
		ID: "asks", Title: "Install it?", Body: "body",
		Yes: func(context.Context) (string, error) {
			*yes++
			return "done", nil
		},
	}
}

func pressKey(t *testing.T, m RootModel, k tea.KeyPressMsg) (RootModel, tea.Cmd) {
	t.Helper()
	next, cmd := m.Update(k)
	return next.(RootModel), cmd
}

// runCmd executes cmd and every command it batches, as the program would, and
// returns the messages they produced.
func runCmd(cmd tea.Cmd) []tea.Msg {
	if cmd == nil {
		return nil
	}
	msg := cmd()
	batch, ok := msg.(tea.BatchMsg)
	if !ok {
		return []tea.Msg{msg}
	}
	var out []tea.Msg
	for _, c := range batch {
		out = append(out, runCmd(c)...)
	}
	return out
}

func TestNoticeAsk_YesRunsTheAnswerAndIsSeen(t *testing.T) {
	yes := 0
	seen := notices.NewMemorySeen()
	m := NewRootModel(false).WithNotices([]notices.Notice{askingNotice(&yes)}, seen)
	m.width, m.height = 100, 30

	m, cmd := pressKey(t, m, tea.KeyPressMsg{Code: 'y', Text: "y"})
	runCmd(cmd)

	assert.Equal(t, 1, yes, "y answers yes")
	assert.True(t, seen.IsSeen("asks"))
	assert.NotContains(t, xansi.Strip(m.View().Content), "Install it?")
}

func TestNoticeAsk_NoAndEscAnswerNo(t *testing.T) {
	for _, k := range []tea.KeyPressMsg{{Code: 'n', Text: "n"}, {Code: tea.KeyEsc}} {
		yes := 0
		seen := notices.NewMemorySeen()
		m := NewRootModel(false).WithNotices([]notices.Notice{askingNotice(&yes)}, seen)
		m.width, m.height = 100, 30

		m, cmd := pressKey(t, m, k)
		runCmd(cmd)

		assert.Zero(t, yes, "%s answers no", k)
		assert.True(t, seen.IsSeen("asks"), "a no is an answer too")
		assert.NotContains(t, xansi.Strip(m.View().Content), "Install it?")
	}
}

// A key typed out of habit, Enter after the login above all, must not answer
// a question that is asked only once.
func TestNoticeAsk_OtherKeysDoNotAnswer(t *testing.T) {
	yes := 0
	seen := notices.NewMemorySeen()
	m := NewRootModel(false).WithNotices([]notices.Notice{askingNotice(&yes)}, seen)
	m.width, m.height = 100, 30

	for _, k := range []tea.KeyPressMsg{{Code: tea.KeyEnter}, {Code: 'x', Text: "x"}, {Code: tea.KeySpace, Text: " "}} {
		var cmd tea.Cmd
		m, cmd = pressKey(t, m, k)
		runCmd(cmd)
	}

	assert.Zero(t, yes)
	assert.False(t, seen.IsSeen("asks"))
	out := xansi.Strip(m.View().Content)
	require.Contains(t, out, "Install it?", "the question stays up")
	assert.Contains(t, out, "y / n", "and says how to answer it")
}

// answeredYes is m with a notice whose yes answers with text or err, after y
// was pressed and what the answer said was handed back, as the program would.
func answeredYes(t *testing.T, text string, err error) RootModel {
	t.Helper()
	cfg := &config.Config{}
	cfg.UI.Toasts.MaxVisible = 3
	n := notices.Notice{ID: "asks", Title: "Install it?", Body: "body",
		Yes: func(context.Context) (string, error) { return text, err }}
	m := NewRootModel(false).WithConfig(cfg).WithNotices([]notices.Notice{n}, notices.NewMemorySeen())
	m.width, m.height = 100, 30

	m, cmd := pressKey(t, m, tea.KeyPressMsg{Code: 'y', Text: "y"})
	for _, msg := range runCmd(cmd) {
		next, _ := m.Update(msg)
		m = next.(RootModel)
	}
	return m
}

func TestNoticeAsk_WhatAYesCameToIsSaid(t *testing.T) {
	m := answeredYes(t, "Installed the service tele.", nil)

	assert.Contains(t, xansi.Strip(onScreen(m)), "Installed the service")
}

func TestNoticeAsk_AYesThatFailedSaysWhy(t *testing.T) {
	m := answeredYes(t, "", errors.New("launchctl bootstrap: exit status 5"))

	assert.Contains(t, xansi.Strip(onScreen(m)), "exit status 5")
}

func TestNoticeAfterLogin_WaitsForTheMainScreen(t *testing.T) {
	yes := 0
	n := askingNotice(&yes)
	n.AfterLogin = true
	seen := notices.NewMemorySeen()
	m := NewRootModel(false).WithNotices([]notices.Notice{n}, seen)
	m.width, m.height = 100, 30

	assert.NotContains(t, xansi.Strip(m.View().Content), "Install it?", "not over the login screen")
	m, cmd := pressKey(t, m, tea.KeyPressMsg{Code: 'y', Text: "y"})
	runCmd(cmd)
	assert.Zero(t, yes, "nor answered from there")
	assert.False(t, seen.IsSeen("asks"))

	next, _ := m.Update(screens.TransitionToMainMsg{})
	m = next.(RootModel)

	assert.Contains(t, xansi.Strip(m.View().Content), "Install it?", "it comes up once logged in")
}

// A notice that waits for the login holds back none that do not.
func TestNoticeAfterLogin_OthersShowOnTheLoginScreenMeanwhile(t *testing.T) {
	yes := 0
	asks := askingNotice(&yes)
	asks.AfterLogin = true
	tells := notices.Notice{ID: "tells", Title: "Your data moved", Body: "body"}
	m := NewRootModel(false).WithNotices([]notices.Notice{asks, tells}, notices.NewMemorySeen())
	m.width, m.height = 100, 30

	assert.Contains(t, xansi.Strip(m.View().Content), "Your data moved")
}
