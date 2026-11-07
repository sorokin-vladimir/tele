package ui_test

import (
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/stretchr/testify/assert"

	"github.com/sorokin-vladimir/tele/internal/core/project"
	"github.com/sorokin-vladimir/tele/internal/domain"
	"github.com/sorokin-vladimir/tele/internal/telerr"
	"github.com/sorokin-vladimir/tele/internal/ui"
	"github.com/sorokin-vladimir/tele/internal/ui/components"
)

// A client that attaches while a step waits is asked it at once: the step is
// in the account it subscribed to, not in an event it was not there for
// (#309).
func TestLogin_AStepWaitingWhenTheClientAttachesIsAskedAtOnce(t *testing.T) {
	m := sizedLoginRoot(t)

	m = withLoginStep(t, m, accountIs(m, project.AccountContents{
		Phase: project.AccountLoggingIn,
		Login: project.LoginStep{Kind: project.LoginCode, Hint: "Enter the SMS code:", Seq: 4},
	}))

	assert.Contains(t, m.View().Content, "Enter the SMS code:")
}

// The same step asked again with a reason is a new asking, and the reason is
// said under the field.
func TestLogin_AStepAskedAgainSaysWhy(t *testing.T) {
	m := sizedLoginRoot(t)
	step := project.AccountContents{
		Phase: project.AccountLoggingIn,
		Login: project.LoginStep{Kind: project.LoginCode, Seq: 1},
	}
	m = withLoginStep(t, m, accountIs(m, step))

	step.Login = project.LoginStep{Kind: project.LoginCode, Err: "That code is not right. Try again.", Seq: 2}
	m = withLoginStep(t, m, accountIs(m, step))

	assert.Contains(t, m.View().Content, "That code is not right. Try again.")
}

// What the connection ended with is the account's, and the login screen shows
// it the way it showed the host's report before (#283).
func TestLogin_TheConnectionsEndComesWithTheAccount(t *testing.T) {
	err := &telerr.Error{Kind: telerr.Network, Op: "dial", Detail: "connection refused"}
	m := sizedLoginRoot(t)

	view := withLoginStep(t, m, accountIs(m, project.AccountContents{Ended: err})).View().Content

	assert.Contains(t, view, "Could not connect to Telegram: no connection")
	assert.Contains(t, view, err.Error())
}

func TestLogin_ALoginThatCannotGoOnIsSaid(t *testing.T) {
	m := sizedLoginRoot(t)

	view := withLoginStep(t, m, accountIs(m, project.AccountContents{
		LoginFailed: "Login requires email verification.",
	})).View().Content

	assert.Contains(t, view, "Login requires email verification.")
}

// Mentions of the logged-in user are drawn as such from what the account says,
// not from anything the host sets beside the endpoint (#208).
func TestAccount_MentionsOfTheLoggedInUserStandOut(t *testing.T) {
	defer components.SetSelfIdentity(0, "")
	mention := []domain.MessageEntity{{Type: "mention", Offset: 0, Length: 4}}
	m := sizedLoginRoot(t)

	m = withLoginStep(t, m, accountIs(m, project.AccountContents{LoggedInUser: project.LoggedInUser{ID: 7, Username: "bob"}}))
	other := components.RenderEntities("@ada hi", mention)
	withLoginStep(t, m, accountIs(m, project.AccountContents{LoggedInUser: project.LoggedInUser{ID: 5, Username: "ada"}}))
	self := components.RenderEntities("@ada hi", mention)

	assert.NotEqual(t, other, self)
}

// A client attaching to an account already loaded goes straight to the main
// screen.
func TestLogin_AReadyAccountOpensTheMainScreen(t *testing.T) {
	m := sizedLoginRoot(t)

	m = withLoginStep(t, m, accountIs(m, project.AccountContents{Phase: project.AccountReady}))

	assert.Equal(t, ui.ScreenMain, m.CurrentScreen())
}

// A window attaching to a service that is already logged in can hear the
// account is ready before the terminal has said how big it is. The main screen
// cannot be laid out without a size, so nothing is drawn until one arrives.
func TestLogin_AReadyAccountBeforeTheSizeDrawsNothing(t *testing.T) {
	m := newLoginRoot().WithLogPath(testLogPath)

	m = withLoginStep(t, m, accountIs(m, project.AccountContents{Phase: project.AccountReady}))

	assert.Empty(t, m.View().Content)
}

// The size arriving after the account is ready brings up the main screen the
// account was already on.
func TestLogin_TheSizeAfterAReadyAccountDrawsTheMainScreen(t *testing.T) {
	m := newLoginRoot().WithLogPath(testLogPath)
	m = withLoginStep(t, m, accountIs(m, project.AccountContents{Phase: project.AccountReady}))

	m = withLoginStep(t, m, tea.WindowSizeMsg{Width: 120, Height: 40})

	assert.Equal(t, ui.ScreenMain, m.CurrentScreen())
	assert.Contains(t, m.View().Content, "[1] Chats")
}
