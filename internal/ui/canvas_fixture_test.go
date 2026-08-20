package ui_test

import (
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/sorokin-vladimir/tele/internal/domain"
	"github.com/sorokin-vladimir/tele/internal/store"
	"github.com/sorokin-vladimir/tele/internal/ui"
)

// The scan of #214 passed while seven sites were unpainted, and the reason was
// content rather than method: newPopulatedRoot builds a one-to-one chat of
// plain-text messages, so no group sender, no entity, no media, no album, no
// edit, no reaction and no separator was ever in the frame it looked at (#227).
//
// One large fixture does not fix that. The message list renders from the bottom
// and the scan runs down to 41x12, so a chat holding every shape at once shows
// only its last few rows at the sizes where the arithmetic is tightest. Each
// shape therefore gets its own short chat, wholly in view at every size, and the
// shapes are walked rather than written out where the domain allows it:
// domain.MediaKindCount bounds the media enum, so a kind added later is scanned
// without anyone remembering to add it here.
//
// Where the domain does not allow it, the list says so. domain.MessageEntity.Type
// is a string whose seventeen values live in a doc comment, so no mechanism can
// notice an eighteenth; entityTypes below is a copy and will drift.

// richChatID is the chat every fixture here opens.
const richChatID = 1

// baseDate is when the fixture's messages are sent. It is a fixed instant rather
// than time.Now so "Today" and a date label are decided by the fixture and not
// by the day the suite runs on.
var baseDate = time.Date(2026, 3, 14, 9, 30, 0, 0, time.UTC)

// richChat builds a main-screen model on a group chat holding exactly msgs.
//
// The chat is a supergroup so that incoming bubbles carry a sender name, which
// is one of the sites: the spaces flanking the name were bare. Its read pointer
// sits below the highest incoming id, so the unread divider is drawn too.
func richChat(t testing.TB, w, h int, msgs []domain.Message) ui.RootModel {
	t.Helper()

	readUpTo := 0
	for _, msg := range msgs {
		if !msg.IsOut && msg.ID > readUpTo {
			readUpTo = msg.ID
		}
	}
	// One below the last incoming message: the divider anchors on the first
	// incoming id above the pointer, so this puts it in front of the last
	// bubble rather than off the top of the window.
	if readUpTo > 0 {
		readUpTo--
	}

	st := store.NewMemory()
	st.SetChat(domain.Chat{
		ID:             richChatID,
		Title:          "Group",
		Peer:           domain.Peer{ID: richChatID, Type: domain.PeerSuperGroup},
		UnreadCount:    1,
		ReadInboxMaxID: readUpTo,
	})
	st.SetMessages(richChatID, msgs)

	m := newRoot(st, len(msgs)+10, false).WithScreen(ui.ScreenMain)
	next, _ := m.Update(tea.WindowSizeMsg{Width: w, Height: h})
	m = next.(ui.RootModel)
	m = toMain(t, m)
	m = openChat(t, m, richChatID, "Group")
	if _, cmd := applyHistory(t, m, st, richChatID); cmd != nil {
		cmd()
	}
	return m
}

// incoming builds a received message from a named sender, so the group sender
// line and the incoming bubble border are both drawn.
func incoming(id int, text string) domain.Message {
	return domain.Message{
		ID:         id,
		ChatID:     richChatID,
		SenderID:   int64(100 + id),
		SenderName: "Alice A",
		Text:       text,
		Date:       baseDate.Add(time.Duration(id) * time.Minute),
	}
}

// outgoing builds a sent message, which takes the other branch of every
// alignment the bubble does.
func outgoing(id int, text string) domain.Message {
	msg := incoming(id, text)
	msg.IsOut = true
	msg.SenderID = 0
	msg.SenderName = ""
	return msg
}

// mediaOf attaches a media reference of the given kind, filled in enough that
// the kinds carrying metadata render the label they carry it for: a duration
// for video and voice, a name and a size for a file, an alt emoji for a sticker.
func mediaOf(msg domain.Message, kind domain.MediaKind) domain.Message {
	ref := &domain.MediaRef{Kind: kind, Duration: 96}
	switch kind {
	case domain.MediaSticker:
		ref.Emoji = "🎈"
	case domain.MediaFile:
		ref.FileName = "quarterly-report.pdf"
		ref.Size = 4 * 1024 * 1024
	case domain.MediaAudio:
		ref.Title, ref.Performer = "Sonata", "Someone"
	}
	msg.Media = ref
	msg.Text = ""
	return msg
}

// entityTypes is a copy of the list in domain.MessageEntity.Type's doc comment.
// It is a copy because the field is a string: there is no value to walk and no
// compile event when an eighteenth type appears, so this list is coverage by
// hand and the only honest thing to do is say so rather than dress it up as a
// guarantee. code and pre matter most — they paint surface_code, which is the
// one legitimate light patch in a painted frame.
var entityTypes = []string{
	"bold", "italic", "code", "pre", "strike", "underline", "text_url", "url",
	"email", "phone", "bank_card", "mention", "mention_name", "hashtag",
	"cashtag", "bot_command",
}

// withEntity marks the whole text with one entity type.
func withEntity(msg domain.Message, kind string) domain.Message {
	e := domain.MessageEntity{Type: kind, Offset: 0, Length: len([]rune(msg.Text))}
	if kind == "text_url" {
		e.URL = "https://example.invalid/target"
	}
	if kind == "pre" {
		e.Language = "go"
	}
	msg.Entities = []domain.MessageEntity{e}
	return msg
}

// album returns count parts sharing a grouped id, which is what the mosaic and
// the part badges are drawn from.
func album(firstID, count int) []domain.Message {
	out := make([]domain.Message, 0, count)
	for i := range count {
		msg := mediaOf(incoming(firstID+i, ""), domain.MediaPhoto)
		msg.GroupedID = 7000
		if i == 1 {
			msg = mediaOf(msg, domain.MediaFile)
			msg.GroupedID = 7000
		}
		out = append(out, msg)
	}
	return out
}

// edited marks a message as having been edited, which is what puts the "edited"
// marker and the separator before the timestamp on the bottom border.
func edited(msg domain.Message) domain.Message {
	at := msg.Date.Add(time.Minute)
	msg.EditDate = &at
	return msg
}

// reacted attaches reactions, which are drawn under the bubble.
func reacted(msg domain.Message) domain.Message {
	msg.Reactions = []domain.Reaction{
		{Emoji: "👍", Count: 3, IsChosen: true},
		{Emoji: "🎉", Count: 1},
	}
	return msg
}

// yesterday moves a message a day back, so that the group after it opens with a
// date separator carrying a real label rather than "Today".
func yesterday(msg domain.Message) domain.Message {
	msg.Date = msg.Date.AddDate(0, 0, -1)
	return msg
}
