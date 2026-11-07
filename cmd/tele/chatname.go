package main

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/sorokin-vladimir/tele/internal/core"
	"github.com/sorokin-vladimir/tele/internal/core/project"
	"github.com/sorokin-vladimir/tele/internal/telerr"
)

// parseChatName reads how a command line names a chat: its id, @username or
// me. Anything else is a mistake in the command line, a title above all: it
// changes, and two chats can share one (#181).
func parseChatName(arg string) (core.ChatName, error) {
	switch {
	case arg == "me":
		return core.ChatName{Me: true}, nil
	case strings.HasPrefix(arg, "@") && len(arg) > 1:
		return core.ChatName{Username: arg[1:]}, nil
	}
	if id, err := strconv.ParseInt(arg, 10, 64); err == nil && id > 0 {
		return core.ChatName{ID: id}, nil
	}
	return core.ChatName{}, &exitCode{code: usageCode, msg: fmt.Sprintf(
		"%q names no chat: name one by its id, its @username, or me.", arg)}
}

// resolveChat answers with the chat arg names. One nobody holds exits 5.
func resolveChat(ctx context.Context, acc accountSurface, arg string) (project.ChatRow, error) {
	name, err := parseChatName(arg)
	if err != nil {
		return project.ChatRow{}, err
	}
	row, err := acc.ResolveChat(ctx, name)
	if telerr.Of(err) == telerr.PeerNotFound {
		return project.ChatRow{}, &exitCode{code: notFoundCode, msg: fmt.Sprintf("No chat is %s.", arg)}
	}
	return row, err
}
