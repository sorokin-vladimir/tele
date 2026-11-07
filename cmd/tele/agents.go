package main

import (
	_ "embed"
	"strings"

	"github.com/spf13/cobra"
)

// agentsGuide is how an AI agent should drive tele's commands on a person's
// behalf. It ships in the binary so that it always matches the tele
// installed, and any agent allowed to run tele can read it (#323).
//
//go:embed agents.md
var agentsGuide string

// newAgentsHelp is `tele help agents`: a help topic rather than a command,
// since it is reference and does nothing.
func newAgentsHelp() *cobra.Command {
	return &cobra.Command{
		Use:   "agents",
		Short: "How an AI agent should use tele's commands",
		Long:  strings.TrimSpace(agentsGuide),
	}
}
