package ui

import (
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sorokin-vladimir/tele/internal/core"
	"github.com/sorokin-vladimir/tele/internal/core/project"
	"github.com/sorokin-vladimir/tele/internal/domain"
	"github.com/sorokin-vladimir/tele/internal/store"
	"github.com/sorokin-vladimir/tele/internal/ui/screens"
)

const testForum = 50

// forumRoot is a model with a forum on the account: Releases read, Design with
// unread, the way the store would list them.
func forumRoot(t *testing.T) (*ownerStub, RootModel) {
	t.Helper()
	st := store.NewMemory()
	st.SetChat(domain.Chat{ID: testForum, Title: "Dev", IsForum: true, Peer: domain.Peer{ID: testForum, Type: domain.PeerSuperGroup}})
	st.SetTopicsPage(testForum, []domain.Topic{
		{ChatID: testForum, ID: 12, Title: "Releases", LastMessage: &domain.Message{ID: 40, Date: time.Unix(400, 0)}},
		{ChatID: testForum, ID: 30, Title: "Design", UnreadCount: 2, LastMessage: &domain.Message{ID: 41, Date: time.Unix(300, 0)}},
	})
	o := newOwnerStub(st)
	m := newRootInternal(st, 50).WithOwner(o).WithScreen(ScreenMain)
	nm, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	return o, nm.(RootModel)
}

func step(t *testing.T, o *ownerStub, m RootModel, msg tea.Msg) RootModel {
	t.Helper()
	nm, cmd := m.Update(msg)
	runCmdTree(cmd)
	nm, _ = o.drain(nm.(RootModel))
	return nm.(RootModel)
}

func enterForum(t *testing.T, o *ownerStub, m RootModel) RootModel {
	t.Helper()
	return step(t, o, m, screens.OpenChatMsg{ChatID: testForum, Title: "Dev", IsForum: true})
}

// Opening a forum turns the middle pane into its topic list rather than opening
// a chat: a forum has no history of its own to show.
func TestForum_OpeningAForumShowsItsTopics(t *testing.T) {
	o, m := forumRoot(t)

	m = enterForum(t, o, m)

	require.NotNil(t, m.forum)
	assert.Equal(t, 2, m.forum.Total())
	assert.Zero(t, m.currentChatID, "nothing is open in the chat pane yet")
	row, ok := m.forum.CursorTopic()
	require.True(t, ok)
	assert.Equal(t, 30, row.ID, "the cursor starts on the first topic with unread")
}

func TestForum_OpeningATopicOpensItsHistory(t *testing.T) {
	o, m := forumRoot(t)
	m = enterForum(t, o, m)

	m = step(t, o, m, screens.OpenTopicMsg{ChatID: testForum, TopicID: 30, Title: "Design"})

	assert.Equal(t, int64(testForum), m.currentChatID)
	assert.Equal(t, 30, m.currentTopicID)
	require.NotEmpty(t, o.focusKeys)
	assert.Equal(t, domain.HistoryKey{ChatID: testForum, TopicID: 30}, o.focusKeys[len(o.focusKeys)-1])
	assert.Equal(t, "Dev › Design", m.chat.HeaderTitle())
}

// Leaving the forum gives the chat list back; going in again lands on the
// topic last opened there.
func TestForum_LeavingAndComingBackRemembersTheTopic(t *testing.T) {
	o, m := forumRoot(t)
	m = enterForum(t, o, m)
	m = step(t, o, m, screens.OpenTopicMsg{ChatID: testForum, TopicID: 12, Title: "Releases"})

	m = step(t, o, m, screens.LeaveForumMsg{ChatID: testForum})
	require.Nil(t, m.forum)

	m = enterForum(t, o, m)
	row, ok := m.forum.CursorTopic()
	require.True(t, ok)
	assert.Equal(t, 12, row.ID)
}

// A message sent from a topic goes to that topic.
func TestForum_SendingFromATopicNamesIt(t *testing.T) {
	o, m := forumRoot(t)
	m = enterForum(t, o, m)
	m = step(t, o, m, screens.OpenTopicMsg{ChatID: testForum, TopicID: 30, Title: "Design"})

	nm, cmd := m.Update(screens.SendMsgRequest{ChatID: testForum, TopicID: 30, Text: "hi"})
	runCmdTree(cmd)
	_ = nm

	require.Len(t, o.sent, 1)
	assert.Equal(t, 30, o.sent[0].TopicID)
}

// Somebody typing in another topic of the forum is not typing here.
func TestForum_TypingIsShownOnlyInItsTopic(t *testing.T) {
	o, m := forumRoot(t)
	m = enterForum(t, o, m)
	m = step(t, o, m, screens.OpenTopicMsg{ChatID: testForum, TopicID: 30, Title: "Design"})

	m = step(t, o, m, core.Typing{ChatID: testForum, TopicID: 12, Label: "Ann is typing"})
	assert.False(t, m.chat.IsTyping())

	m = step(t, o, m, core.Typing{ChatID: testForum, TopicID: 30, Label: "Ann is typing"})
	assert.True(t, m.chat.IsTyping())
}

// A message arriving in a topic flashes that topic's row.
func TestForum_AnArrivalFlashesItsTopic(t *testing.T) {
	o, m := forumRoot(t)
	m = enterForum(t, o, m)

	m = step(t, o, m, core.Incoming{ChatID: testForum, TopicID: 12})

	assert.Equal(t, int64(12), m.forum.HighlightedTopicID())
}

// A notification from a topic opens that topic, going into the forum first.
func TestForum_ANotificationOpensItsTopic(t *testing.T) {
	o, m := forumRoot(t)

	nm, cmd := m.Update(notifyOpenMsg{chatID: testForum, topicID: 30, title: "Dev", topicTitle: "Design"})
	require.NotNil(t, cmd)
	m = step(t, o, nm.(RootModel), cmd())

	require.NotNil(t, m.forum)
	assert.Equal(t, int64(testForum), m.currentChatID)
	assert.Equal(t, 30, m.currentTopicID)
}

// Choosing a forum in the forward picker lists its topics; choosing a topic
// forwards into it.
func TestForum_ForwardingIntoAForumGoesThroughItsTopics(t *testing.T) {
	o, m := forumRoot(t)

	nm, cmd := m.Update(screens.ForwardForumChosen{ChatID: testForum, Title: "Dev", MsgID: 55})
	require.NotNil(t, cmd)
	m = step(t, o, nm.(RootModel), cmd())
	require.NotNil(t, m.searchModel, "the picker now lists the forum's topics")

	nm, cmd = m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	require.NotNil(t, cmd)
	req, ok := cmd().(screens.ForwardToChatRequest)
	require.True(t, ok)
	assert.Equal(t, int64(testForum), req.ToChatID)
	assert.NotZero(t, req.ToTopicID)

	m = nm.(RootModel)
	_, cmd = m.Update(req)
	runCmdTree(cmd)
	require.Len(t, o.forwardTargets, 1)
	assert.Equal(t, domain.HistoryKey{ChatID: testForum, TopicID: req.ToTopicID}, o.forwardTargets[0])
}

// The menu over a topic row acts on the topic.
func TestForum_TheTopicMenuActsOnTheTopic(t *testing.T) {
	o, m := forumRoot(t)
	m = enterForum(t, o, m)
	m.focus = FocusChatList

	m = step(t, o, m, tea.KeyPressMsg{Code: tea.KeySpace, Text: " "})
	require.NotNil(t, m.chatMenu)

	nm, cmd := m.Update(tea.KeyPressMsg{Code: 'm', Text: "m"})
	require.NotNil(t, cmd)
	m = step(t, o, nm.(RootModel), cmd())
	_ = m

	require.NotEmpty(t, o.calls)
	last := o.calls[len(o.calls)-1]
	assert.Equal(t, "SetMuted", last.name)
	assert.Equal(t, 30, last.topic, "the topic under the cursor")
}

// The mouse works the topic list where it worked the chat list.
func TestForum_TheMouseWorksTheTopicList(t *testing.T) {
	o, m := forumRoot(t)
	m = enterForum(t, o, m)
	m.forum.SetCursor(0)

	nm, _ := m.Update(tea.MouseWheelMsg(tea.Mouse{X: 3, Y: 2, Button: tea.MouseWheelDown}))
	m = nm.(RootModel)
	// The notch moves more than one row but the fixture has only two topics, so
	// the cursor stops on the last one.
	assert.Equal(t, 1, m.forum.Cursor())

	_, cmd := m.Update(tea.MouseClickMsg(tea.Mouse{X: 3, Y: 1, Button: tea.MouseLeft}))
	require.NotNil(t, cmd)
	var opened *screens.OpenTopicMsg
	for _, msg := range collectMsgs(cmd) {
		if open, ok := msg.(screens.OpenTopicMsg); ok {
			opened = &open
		}
	}
	require.NotNil(t, opened, "a click on a topic row opens it")
	assert.Equal(t, 12, opened.TopicID)
}

// collectMsgs runs a command and returns every message it produced, batches
// flattened.
func collectMsgs(cmd tea.Cmd) []tea.Msg {
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
		out = append(out, collectMsgs(c)...)
	}
	return out
}

// Esc in the topic list is how the pane is left.
func TestForum_EscInTheTopicListLeavesIt(t *testing.T) {
	o, m := forumRoot(t)
	m = enterForum(t, o, m)
	m.focus = FocusChatList

	nm, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	require.NotNil(t, cmd)
	m = step(t, o, nm.(RootModel), cmd())

	assert.Nil(t, m.forum)
}

// #307: selecting a folder is leaving the forum. The middle pane shows that
// folder's chats at once, and the topic open in the chat pane stays open: a
// folder filters the chat list, it does not move to another chat.
func TestForum_SelectingAFolderLeavesIt(t *testing.T) {
	o, m := forumRoot(t)
	m = enterForum(t, o, m)
	m = step(t, o, m, screens.OpenTopicMsg{ChatID: testForum, TopicID: 30, Title: "Design"})

	m = step(t, o, m, screens.FolderSelectedMsg{Filter: &domain.FolderFilter{ID: 1, Title: "Work"}})

	assert.Nil(t, m.forum)
	assert.Equal(t, 1, m.activeFolder)
	assert.Equal(t, domain.HistoryKey{ChatID: testForum, TopicID: 30}, m.currentHistory())
}

// #307: opening a chat that is not this forum is leaving it. Search, a
// notification and a profile's "Send message" all open a chat this way.
func TestForum_OpeningAnotherChatLeavesIt(t *testing.T) {
	o, m := forumRoot(t)
	m = enterForum(t, o, m)

	m = step(t, o, m, screens.OpenChatMsg{ChatID: 60, Title: "Alice"})

	assert.Nil(t, m.forum)
	assert.Equal(t, domain.HistoryKey{ChatID: 60}, m.currentHistory())
}

// #307: picking the folder already shown leaves the forum too; it is how a
// person asks to see that folder's chats.
func TestForum_SelectingTheFolderAlreadyShownLeavesIt(t *testing.T) {
	o, m := forumRoot(t)
	m = enterForum(t, o, m)
	require.Zero(t, m.activeFolder)

	m = step(t, o, m, screens.FolderSelectedMsg{Filter: nil})

	assert.Nil(t, m.forum)
}

// #307: the automatic fallback from an emptied Archive to All Chats is not the
// person's choice, so it leaves the forum they are reading where it is.
func TestForum_ArchiveFallbackKeepsIt(t *testing.T) {
	o, m := forumRoot(t)
	m = enterForum(t, o, m)
	m.activeFolder = domainArchiveFolderID

	m = step(t, o, m, project.Delta{Sub: m.chatListSub, ChatList: &project.ChatListDelta{
		Kind:    project.ChatListFolders,
		Folders: project.FolderCounts{ArchivePresent: false},
	}})

	require.Zero(t, m.activeFolder, "the fallback ran")
	assert.NotNil(t, m.forum)
}
