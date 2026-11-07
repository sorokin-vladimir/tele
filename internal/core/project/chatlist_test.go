package project_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sorokin-vladimir/tele/internal/core/project"
	"github.com/sorokin-vladimir/tele/internal/domain"
)

// fakeReader serves a fixed list in a fixed order, standing in for the store.
// Order is the store's job (pinned first, then last-message time), so the
// builder must preserve it rather than sort.
type fakeReader struct {
	chats   []domain.Chat
	msgs    map[int64][]domain.Message
	filters []domain.FolderFilter
	outbox  map[int64][]domain.OutboxEntry
	// topicMsgs holds a forum's histories, one per topic; topics its topics in
	// the order the store would list them.
	topicMsgs map[domain.HistoryKey][]domain.Message
	topics    map[int64][]domain.Topic
	account   project.AccountContents
}

func (f *fakeReader) Topics(chatID int64) []domain.Topic { return f.topics[chatID] }

func (f *fakeReader) Topic(chatID int64, topicID int) (domain.Topic, bool) {
	for _, t := range f.topics[chatID] {
		if t.ID == topicID {
			return t, true
		}
	}
	return domain.Topic{}, false
}

func (f *fakeReader) Chats() []domain.Chat { return f.chats }

func (f *fakeReader) GetChat(id int64) (domain.Chat, bool) {
	for _, c := range f.chats {
		if c.ID == id {
			return c, true
		}
	}
	return domain.Chat{}, false
}

func (f *fakeReader) Messages(h domain.HistoryKey) []domain.Message {
	if h.TopicID != 0 {
		return f.topicMsgs[h]
	}
	return f.msgs[h.ChatID]
}

func (f *fakeReader) FolderFilters() []domain.FolderFilter { return f.filters }

func (f *fakeReader) Outbox(h domain.HistoryKey) []domain.OutboxEntry { return f.outbox[h.ChatID] }

func (f *fakeReader) Account() project.AccountContents { return f.account }

func chats(n int) []domain.Chat {
	out := make([]domain.Chat, 0, n)
	for i := 1; i <= n; i++ {
		out = append(out, domain.Chat{ID: int64(i), Title: "chat", Peer: domain.Peer{ID: int64(i)}})
	}
	return out
}

func TestBuildChatList_WindowsAndReportsTotal(t *testing.T) {
	r := &fakeReader{chats: chats(10)}

	got := project.BuildChatList(r, project.ChatListWindow{Offset: 3, Limit: 4})

	require.Len(t, got.Rows, 4)
	assert.Equal(t, int64(4), got.Rows[0].ID, "window starts at the offset")
	assert.Equal(t, 3, got.Offset)
	assert.Equal(t, 10, got.Total, "total is the whole filtered list, for the scrollbar")
}

func TestBuildChatList_ClampsWindowPastTheEnd(t *testing.T) {
	r := &fakeReader{chats: chats(3)}

	got := project.BuildChatList(r, project.ChatListWindow{Offset: 2, Limit: 10})

	require.Len(t, got.Rows, 1)
	assert.Equal(t, int64(3), got.Rows[0].ID)
	assert.Equal(t, 3, got.Total)
}

func TestBuildChatList_OffsetBeyondEndYieldsNoRows(t *testing.T) {
	r := &fakeReader{chats: chats(3)}

	got := project.BuildChatList(r, project.ChatListWindow{Offset: 9, Limit: 5})

	assert.Empty(t, got.Rows)
	assert.Equal(t, 3, got.Total)
}

func TestBuildChatList_AllChatsExcludesArchived(t *testing.T) {
	all := chats(3)
	all[1].IsArchived = true
	r := &fakeReader{chats: all}

	got := project.BuildChatList(r, project.ChatListWindow{Folder: 0, Offset: 0, Limit: 10})

	require.Len(t, got.Rows, 2)
	assert.Equal(t, int64(1), got.Rows[0].ID)
	assert.Equal(t, int64(3), got.Rows[1].ID)
}

func TestBuildChatList_ArchiveFolderShowsOnlyArchived(t *testing.T) {
	all := chats(3)
	all[1].IsArchived = true
	r := &fakeReader{chats: all}

	got := project.BuildChatList(r,
		project.ChatListWindow{Folder: domain.ArchiveFolderID, Offset: 0, Limit: 10})

	require.Len(t, got.Rows, 1)
	assert.Equal(t, int64(2), got.Rows[0].ID)
}

func TestBuildChatList_CustomFolderUsesFilterRules(t *testing.T) {
	all := chats(3)
	all[0].UnreadCount = 5
	r := &fakeReader{
		chats:   all,
		filters: []domain.FolderFilter{{ID: 7, Title: "Unread", IncludePeers: []int64{1}}},
	}

	got := project.BuildChatList(r, project.ChatListWindow{Folder: 7, Offset: 0, Limit: 10})

	require.Len(t, got.Rows, 1)
	assert.Equal(t, int64(1), got.Rows[0].ID)
}

func TestBuildChatList_UnknownFolderYieldsNoRows(t *testing.T) {
	r := &fakeReader{chats: chats(3)}

	got := project.BuildChatList(r, project.ChatListWindow{Folder: 99, Limit: 10})

	assert.Empty(t, got.Rows)
	assert.Zero(t, got.Total)
}

func TestBuildChatList_RowCarriesWhatTheRowRenders(t *testing.T) {
	c := domain.Chat{
		ID:                   1,
		Title:                "Ada",
		Peer:                 domain.Peer{ID: 1, Type: domain.PeerUser, AccessHash: 999},
		Online:               true,
		UnreadCount:          3,
		UnreadMentionsCount:  1,
		UnreadReactionsCount: 2,
		UnreadMark:           true,
		IsMuted:              true,
	}
	r := &fakeReader{chats: []domain.Chat{c}}

	got := project.BuildChatList(r, project.ChatListWindow{Limit: 10})

	require.Len(t, got.Rows, 1)
	assert.Equal(t, project.ChatRow{
		ID: 1, Title: "Ada", Kind: project.ChatKindUser, IsUser: true, Online: true,
		Unread: 3, Mentions: 1, Reactions: 2, UnreadMark: true, Muted: true,
	}, got.Rows[0], "the row carries no Peer: a client holds no access hash")
}

// The chat context menu is built from the row under the cursor, and it offers
// Archive or Unarchive by this flag (#278).
func TestRow_CarriesWhetherTheChatIsArchived(t *testing.T) {
	got := project.Row(domain.Chat{ID: 1, Title: "Ada", IsArchived: true}, nil)

	assert.True(t, got.Archived)
}

// A command names a chat's kind and username: a supergroup is a group to the
// person, and a bot is told from a person (#181).
func TestRow_CarriesTheChatsKindAndUsername(t *testing.T) {
	cases := []struct {
		chat domain.Chat
		kind project.ChatKind
	}{
		{domain.Chat{ID: 1, Username: "ada", Peer: domain.Peer{Type: domain.PeerUser}}, project.ChatKindUser},
		{domain.Chat{ID: 2, Username: "helper_bot", IsBot: true, Peer: domain.Peer{Type: domain.PeerUser}}, project.ChatKindBot},
		{domain.Chat{ID: 3, Peer: domain.Peer{Type: domain.PeerGroup}}, project.ChatKindGroup},
		{domain.Chat{ID: 4, Username: "gophers", Peer: domain.Peer{Type: domain.PeerSuperGroup}}, project.ChatKindGroup},
		{domain.Chat{ID: 5, Username: "news", Peer: domain.Peer{Type: domain.PeerChannel}}, project.ChatKindChannel},
	}
	for _, c := range cases {
		got := project.Row(c.chat, nil)

		assert.Equal(t, c.kind, got.Kind, "chat %d", c.chat.ID)
		assert.Equal(t, c.chat.Username, got.Username, "chat %d", c.chat.ID)
	}
}

func TestBuildChatList_FolderCountsCountChatsWithUnreadAcrossTheWholeList(t *testing.T) {
	all := chats(4)
	all[0].UnreadCount = 1
	all[3].UnreadCount = 9 // outside a 2-row window, still counted
	all[2].IsArchived = true
	r := &fakeReader{
		chats:   all,
		filters: []domain.FolderFilter{{ID: 7, IncludePeers: []int64{1, 4}}},
	}

	got := project.BuildChatList(r, project.ChatListWindow{Offset: 0, Limit: 2})

	assert.Equal(t, 2, got.Folders.Unread[7], "counts chats with unread, not messages")
	assert.True(t, got.Folders.ArchivePresent)
}

func TestBuildChatList_FolderCountsSkipAllChatsAndArchive(t *testing.T) {
	all := chats(2)
	all[0].UnreadCount = 1
	r := &fakeReader{
		chats:   all,
		filters: []domain.FolderFilter{{ID: 0}, {ID: domain.ArchiveFolderID}},
	}

	got := project.BuildChatList(r, project.ChatListWindow{Limit: 10})

	assert.NotContains(t, got.Folders.Unread, 0, "All Chats has no badge")
	assert.NotContains(t, got.Folders.Unread, domain.ArchiveFolderID, "Archive shows no count")
	assert.False(t, got.Folders.ArchivePresent)
}

// A custom folder lists archived chats when its rules match them, so its badge
// must count their unread too. Ported from ui.computeFolderUnreads' tests when
// the computation moved here.
func TestBuildChatList_FolderCountsIncludeArchivedOnACategoryMatch(t *testing.T) {
	r := &fakeReader{
		chats: []domain.Chat{
			{ID: 1, Peer: domain.Peer{ID: 1, Type: domain.PeerGroup}, IsArchived: true, UnreadCount: 2},
			{ID: 2, Peer: domain.Peer{ID: 2, Type: domain.PeerGroup}, UnreadCount: 3},
		},
		filters: []domain.FolderFilter{{ID: 7, Title: "Groups", Groups: true}},
	}

	got := project.BuildChatList(r, project.ChatListWindow{Limit: 10})

	assert.Equal(t, 2, got.Folders.Unread[7], "an archived group is in the folder, so it is in the badge")
	assert.NotContains(t, got.Folders.Unread, domain.ArchiveFolderID)
}

func TestBuildChatList_CustomFolderIncludesAnArchivedChatByCategory(t *testing.T) {
	r := &fakeReader{
		chats: []domain.Chat{
			{ID: 1, Title: "Normal", Peer: domain.Peer{ID: 1, Type: domain.PeerUser}},
			{ID: 2, Title: "Archived", Peer: domain.Peer{ID: 2, Type: domain.PeerGroup}, IsArchived: true},
		},
		filters: []domain.FolderFilter{{ID: 7, Title: "Groups", Groups: true}},
	}

	got := project.BuildChatList(r, project.ChatListWindow{Folder: 7, Limit: 10})

	require.Len(t, got.Rows, 1)
	assert.Equal(t, int64(2), got.Rows[0].ID)
}

func TestBuildChatList_CustomFolderIncludesAnArchivedExplicitPeer(t *testing.T) {
	r := &fakeReader{
		chats: []domain.Chat{
			{ID: 1, Title: "Normal", Peer: domain.Peer{ID: 1, Type: domain.PeerUser}},
			{ID: 2, Title: "Archived", Peer: domain.Peer{ID: 2, Type: domain.PeerChannel}, IsArchived: true},
		},
		filters: []domain.FolderFilter{{ID: 7, Title: "News", IncludePeers: []int64{2}}},
	}

	got := project.BuildChatList(r, project.ChatListWindow{Folder: 7, Limit: 10})

	require.Len(t, got.Rows, 1)
	assert.Equal(t, int64(2), got.Rows[0].ID)
}
