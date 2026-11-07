package main

import (
	"fmt"
	"strings"
	"testing"
)

// `tele help agents` prints the guide for an AI agent driving tele, from the
// binary itself, so it matches the tele installed (#323).
func TestAgents_HelpPrintsTheGuide(t *testing.T) {
	out, code := runBuilt(t, "help", "agents")

	if code != 0 {
		t.Fatalf("exited %d:\n%s", code, out)
	}
	if strings.TrimSpace(out) != strings.TrimSpace(agentsGuide) {
		t.Errorf("printed something other than the guide:\n%s", out)
	}
}

// The guide is offered where the help lists what else it can say.
func TestAgents_TheHelpOffersIt(t *testing.T) {
	help, _ := runBuilt(t, "--help")

	assertSays(t, help, "agents")
}

// Every exit code a command can end with is in the guide, with what to do
// about it: an agent meets them all, and one left out is one it guesses at.
func TestAgents_TheGuideSaysEveryExitCode(t *testing.T) {
	for _, code := range []int{usageCode, noHostCode, notLoggedInCode, notFoundCode, notSentCode, commandsOffCode, laterCode} {
		if !strings.Contains(agentsGuide, fmt.Sprintf("| %d ", code)) {
			t.Errorf("the guide has no row for exit %d", code)
		}
	}
}
