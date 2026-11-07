package app

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sorokin-vladimir/tele/internal/notices"
)

func serviceNotice(all []notices.Notice) (notices.Notice, bool) {
	for _, n := range all {
		if n.ID == "service-prompt-v2.0" {
			return n, true
		}
	}
	return notices.Notice{}, false
}

func TestStartupNotices_AServiceIsOfferedOnceLoggedIn(t *testing.T) {
	installs := 0
	offer := &ServiceOffer{App: "tele-beta", Install: func(context.Context) (string, error) {
		installs++
		return "installed", nil
	}}

	n, ok := serviceNotice(StartupNotices("/state", false, false, offer))

	require.True(t, ok, "a tele that can install its service offers to")
	assert.True(t, n.Asks(), "it is a question")
	assert.True(t, n.AfterLogin, "asked only once logged in")
	assert.Contains(t, n.Body, "tele-beta daemon install", "naming the command for whoever changes their mind")
	text, err := n.Yes(context.Background())
	require.NoError(t, err)
	assert.Equal(t, "installed", text)
	assert.Equal(t, 1, installs, "a yes installs it")
}

func TestStartupNotices_NoOfferNoQuestion(t *testing.T) {
	_, ok := serviceNotice(StartupNotices("/state", false, false, nil))

	assert.False(t, ok)
}
