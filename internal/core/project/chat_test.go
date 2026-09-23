package project_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sorokin-vladimir/tele/internal/core/project"
	"github.com/sorokin-vladimir/tele/internal/domain"
)

// msgs builds n messages with ids 1..n, oldest first — the order Messages()
// returns (store/sqlite_messages.go sorts by date then id).
func msgs(n int) []domain.Message {
	out := make([]domain.Message, 0, n)
	base := time.Unix(1000, 0)
	for i := 1; i <= n; i++ {
		out = append(out, domain.Message{
			ID:     i,
			ChatID: 1,
			Date:   base.Add(time.Duration(i) * time.Minute),
		})
	}
	return out
}

func readerWith(chat domain.Chat, m []domain.Message) *fakeReader {
	return &fakeReader{
		chats: []domain.Chat{chat},
		msgs:  map[int64][]domain.Message{chat.ID: m},
	}
}

func ids(m []domain.Message) []int {
	out := make([]int, 0, len(m))
	for _, x := range m {
		out = append(out, x.ID)
	}
	return out
}

func TestBuildChat_NewestAnchorTakesTheTail(t *testing.T) {
	r := readerWith(domain.Chat{ID: 1}, msgs(10))

	got := project.BuildChat(r, project.ChatWindow{
		ChatID: 1, Anchor: project.Anchor{Kind: project.AnchorNewest}, Before: 3, After: 0,
	})

	assert.Equal(t, []int{7, 8, 9, 10}, ids(got.Messages),
		"Before counts messages above the anchor; the anchor is carried on top of it")
	assert.Equal(t, 10, got.AnchorMsgID)
	assert.True(t, got.HasOlder)
	assert.False(t, got.HasNewer, "the newest message is in the window")
}

func TestBuildChat_FirstUnreadAnchorKeepsContextAbove(t *testing.T) {
	r := readerWith(domain.Chat{ID: 1, UnreadCount: 4, ReadInboxMaxID: 6}, msgs(10))

	got := project.BuildChat(r, project.ChatWindow{
		ChatID: 1, Anchor: project.Anchor{Kind: project.AnchorFirstUnread}, Before: 2, After: 5,
	})

	assert.Equal(t, 7, got.AnchorMsgID, "first unread is the first id above ReadInboxMaxID")
	assert.Equal(t, []int{5, 6, 7, 8, 9, 10}, ids(got.Messages),
		"Before messages above the anchor, the anchor itself, and up to After below")
	assert.True(t, got.HasOlder)
	assert.False(t, got.HasNewer)
}

func TestBuildChat_FirstUnreadWithNoUnreadBehavesAsNewest(t *testing.T) {
	r := readerWith(domain.Chat{ID: 1, UnreadCount: 0, ReadInboxMaxID: 10}, msgs(10))

	got := project.BuildChat(r, project.ChatWindow{
		ChatID: 1, Anchor: project.Anchor{Kind: project.AnchorFirstUnread}, Before: 2, After: 5,
	})

	assert.Equal(t, 10, got.AnchorMsgID)
	assert.Equal(t, []int{8, 9, 10}, ids(got.Messages))
}

func TestBuildChat_FirstUnreadWithReadPointerPastEverythingStoredIsNewest(t *testing.T) {
	// UnreadCount claims unread, but every stored message is at or below the read
	// pointer: the unread messages have not been fetched yet.
	r := readerWith(domain.Chat{ID: 1, UnreadCount: 3, ReadInboxMaxID: 10}, msgs(10))

	got := project.BuildChat(r, project.ChatWindow{
		ChatID: 1, Anchor: project.Anchor{Kind: project.AnchorFirstUnread}, Before: 2,
	})

	assert.Equal(t, 10, got.AnchorMsgID)
}

func TestBuildChat_MessageAnchorIsSymmetric(t *testing.T) {
	r := readerWith(domain.Chat{ID: 1}, msgs(10))

	got := project.BuildChat(r, project.ChatWindow{
		ChatID: 1, Anchor: project.Anchor{Kind: project.AnchorMessage, MsgID: 5}, Before: 2, After: 2,
	})

	assert.Equal(t, 5, got.AnchorMsgID)
	assert.Equal(t, []int{3, 4, 5, 6, 7}, ids(got.Messages))
	assert.True(t, got.HasOlder)
	assert.True(t, got.HasNewer, "messages exist below the window")
}

func TestBuildChat_MessageAnchorNotInStoreYieldsEmptyWindow(t *testing.T) {
	r := readerWith(domain.Chat{ID: 1}, msgs(10))

	got := project.BuildChat(r, project.ChatWindow{
		ChatID: 1, Anchor: project.Anchor{Kind: project.AnchorMessage, MsgID: 99}, Before: 2, After: 2,
	})

	assert.Empty(t, got.Messages, "the core backfills, then rebuilds — the builder does not guess")
	assert.Equal(t, 99, got.AnchorMsgID)
	assert.False(t, got.HasOlder,
		"HasOlder reports what the store holds outside the window, not what Telegram might have")
}

func TestBuildChat_ClampsAtBothEnds(t *testing.T) {
	r := readerWith(domain.Chat{ID: 1}, msgs(3))

	got := project.BuildChat(r, project.ChatWindow{
		ChatID: 1, Anchor: project.Anchor{Kind: project.AnchorNewest}, Before: 50, After: 50,
	})

	assert.Equal(t, []int{1, 2, 3}, ids(got.Messages))
	assert.False(t, got.HasOlder, "clamped at the start")
	assert.False(t, got.HasNewer, "clamped at the end")
}

func TestBuildChat_EmptyChat(t *testing.T) {
	r := readerWith(domain.Chat{ID: 1}, nil)

	got := project.BuildChat(r, project.ChatWindow{
		ChatID: 1, Anchor: project.Anchor{Kind: project.AnchorNewest}, Before: 10,
	})

	assert.Empty(t, got.Messages)
	assert.False(t, got.HasOlder)
	assert.False(t, got.HasNewer)
	assert.Zero(t, got.AnchorMsgID)
}

func TestBuildChat_CarriesTheHeaderTheChatPaneRenders(t *testing.T) {
	c := domain.Chat{
		ID:              1,
		Title:           "Ada",
		Peer:            domain.Peer{ID: 1, Type: domain.PeerUser},
		Online:          true,
		ReadInboxMaxID:  4,
		ReadOutboxMaxID: 2,
		Draft:           "wip",

		UnreadMentionsCount: 3,
	}
	r := readerWith(c, msgs(5))

	got := project.BuildChat(r, project.ChatWindow{
		ChatID: 1, Anchor: project.Anchor{Kind: project.AnchorNewest}, Before: 2,
	})

	assert.Equal(t, "Ada", got.Title)
	assert.True(t, got.IsUser)
	assert.False(t, got.IsGroup)
	assert.True(t, got.Online, "the presence dot in root_view.go is per-chat state")
	assert.Equal(t, 4, got.ReadInboxMaxID)
	assert.Equal(t, 2, got.ReadOutboxMaxID)
	assert.Equal(t, "wip", got.Draft)
	assert.Equal(t, 3, got.UnreadMentions, "opening the chat reads its mentions")
}

func TestBuildChat_UnknownChat(t *testing.T) {
	r := readerWith(domain.Chat{ID: 1}, msgs(3))

	got := project.BuildChat(r, project.ChatWindow{
		ChatID: 77, Anchor: project.Anchor{Kind: project.AnchorNewest}, Before: 10,
	})

	assert.Empty(t, got.Messages)
	assert.Equal(t, int64(77), got.ChatID)
}

func TestBuildChat_GroupAndChannelAreGroups(t *testing.T) {
	for _, tc := range []struct {
		name string
		typ  domain.PeerType
		want bool
	}{
		{"user", domain.PeerUser, false},
		{"group", domain.PeerGroup, true},
		{"supergroup", domain.PeerSuperGroup, true},
		{"channel", domain.PeerChannel, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := readerWith(domain.Chat{ID: 1, Peer: domain.Peer{ID: 1, Type: tc.typ}}, msgs(2))

			got := project.BuildChat(r, project.ChatWindow{
				ChatID: 1, Anchor: project.Anchor{Kind: project.AnchorNewest}, Before: 5,
			})

			assert.Equal(t, tc.want, got.IsGroup, "the message list shows sender names only in groups")
		})
	}
}

// Everything past the first unread is unread by definition, so a window
// anchored there must run to the newest message. Stopping at the anchor hides
// exactly what the chat was opened to show.
func TestBuildChat_FirstUnreadWindowReachesTheNewestMessage(t *testing.T) {
	r := readerWith(domain.Chat{ID: 1, UnreadCount: 4, ReadInboxMaxID: 6}, msgs(10))

	got := project.BuildChat(r, project.ChatWindow{
		ChatID: 1, Anchor: project.Anchor{Kind: project.AnchorFirstUnread}, Before: 2, After: 0,
	})

	assert.Equal(t, 7, got.AnchorMsgID)
	assert.Equal(t, []int{5, 6, 7, 8, 9, 10}, ids(got.Messages),
		"After is 0, yet the unread tail is still in the window")
	assert.False(t, got.HasNewer)
}

func TestBuildChat_CarriesTheChatsOutboxEntries(t *testing.T) {
	r := readerWith(domain.Chat{ID: 1, Title: "Ada"}, msgs(3))
	r.outbox = map[int64][]domain.OutboxEntry{
		1: {{Ref: "r1", ChatID: 1, State: domain.OutboxQueued}},
	}

	got := project.BuildChat(r, project.ChatWindow{ChatID: 1, Before: 10})

	require.Len(t, got.Outbox, 1)
	assert.Equal(t, "r1", got.Outbox[0].Ref)
}

func TestBuildChat_CarriesTheOutboxWhenTheChatHasNoHistory(t *testing.T) {
	// A queued send is the only thing an empty chat has to show, so it must not
	// be lost to the early return that skips window slicing.
	r := readerWith(domain.Chat{ID: 1, Title: "Ada"}, nil)
	r.outbox = map[int64][]domain.OutboxEntry{
		1: {{Ref: "r1", ChatID: 1, State: domain.OutboxQueued}},
	}

	got := project.BuildChat(r, project.ChatWindow{ChatID: 1, Before: 10})

	require.Len(t, got.Outbox, 1)
	assert.Empty(t, got.Messages)
}
