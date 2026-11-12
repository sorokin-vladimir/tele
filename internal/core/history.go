package core

import (
	"context"

	"go.uber.org/zap"

	"github.com/sorokin-vladimir/tele/internal/core/project"
	"github.com/sorokin-vladimir/tele/internal/domain"
	"github.com/sorokin-vladimir/tele/internal/store"
	"github.com/sorokin-vladimir/tele/internal/telerr"
)

// fill brings a chat window up to date from Telegram, in the order the two
// reasons to fetch deserve. A gap is a defect and is closed first; a window the
// store came up short on is ordinary and is filled after. They share one guard
// rather than racing for it: run separately, whichever started first would
// silently deny the other, and the loser would wait for a scroll that may never
// come.
//
// One fetch per subscription is in flight at a time (issue #120).
func (o *Owner) fill(ctx context.Context, a *seat, id project.SubID, w project.HistoryWindow) {
	key := subKey{client: a.id, sub: id}
	if !o.beginFetch(key) {
		return
	}
	defer o.endFetch(key)

	o.forwardFill(ctx, w.ChatID)
	// Re-read the window: the repair may have filled it, and asking again costs
	// a projection build against messages already in memory.
	sf := shortfallOf(project.BuildHistory(o.reader(), w), w)
	if !sf.any() {
		return
	}
	chat, ok := o.state.Store().GetChat(w.ChatID)
	if !ok {
		return
	}
	switch {
	case sf.around != 0:
		o.jump(ctx, a, chat, w, sf.around)
	case sf.newest:
		o.backfill(ctx, a, chat, w, 0)
	default:
		if sf.olderThan != 0 {
			o.backfill(ctx, a, chat, w, sf.olderThan)
		}
		if sf.newerThan != 0 {
			o.backfillNewer(ctx, a, chat, w, sf.newerThan)
		}
	}
}

// shortfall is what a window's contents say the store lacks to fill it. Each
// id is zero when nothing is lacking there: a message id is never zero.
type shortfall struct {
	// around names the message the window is anchored on and the store does
	// not hold: the page around it is fetched.
	around int
	// newest says the window is at the tail of a history holding no tail run.
	newest bool
	// olderThan asks for history older than the oldest message of the
	// window's run, newerThan for history newer than its newest.
	olderThan int
	newerThan int
}

func (s shortfall) any() bool {
	return s.around != 0 || s.newest || s.olderThan != 0 || s.newerThan != 0
}

// shortfallOf reads what a window lacks off what the store built it with. A
// window short above its anchor at the start of its run lacks older history
// unless the run reaches the beginning; one short below its anchor at the end
// of its run lacks newer history unless the run ends at the tail.
func shortfallOf(c project.HistoryContents, w project.HistoryWindow) shortfall {
	if len(c.Messages) == 0 {
		if w.Anchor.Kind == project.AnchorMessage {
			return shortfall{around: w.Anchor.MsgID}
		}
		return shortfall{newest: true}
	}
	idx := 0
	for i, m := range c.Messages {
		if m.ID == c.AnchorMsgID {
			idx = i
		}
	}
	var sf shortfall
	if !c.HasOlder && !c.AtBeginning && idx < w.Before {
		sf.olderThan = c.Messages[0].ID
	}
	if !c.HasNewer && !c.AtTail && len(c.Messages)-1-idx < w.After {
		sf.newerThan = c.Messages[len(c.Messages)-1].ID
	}
	return sf
}

// backfill fetches history older than below for a window, the newest page
// when below is zero, and applies it to state; the registry then emits the
// resulting delta through the same path as any other change. Caller holds the
// fetch guard. A failure is told to a, the client whose window it was: nobody
// else asked.
func (o *Owner) backfill(ctx context.Context, a *seat, chat domain.Chat, w project.HistoryWindow, below int) {
	// Read here rather than kept in a field: a changed history limit applies to
	// the next backfill, which is this one.
	limit := o.Config().UI.HistoryLimit
	var fetched []domain.Message
	var err error
	if w.TopicID != 0 {
		// A topic's scrollback is its own thread. The forum's history would
		// bring every other topic's messages in its place.
		fetched, err = o.client.GetReplies(ctx, chat.Peer, w.TopicID, below, limit)
	} else {
		fetched, err = o.client.GetHistory(ctx, chat.Peer, below, limit)
	}
	if err != nil {
		o.failedToFill(a, w, err)
		return
	}
	// The store merges under its own lock, so a message that arrives mid-fetch
	// is not written back out of existence. A page with nothing older marks the
	// run as reaching the beginning, which is what stops the next backfill.
	_, added := o.state.MergeOlder(w.History(), below, fetched)
	o.log.Debug("history backfill",
		zap.Int64("chat", w.ChatID), zap.Int("below", below),
		zap.Int("fetched", len(fetched)), zap.Bool("added", added))
}

// backfillNewer fetches history newer than above for a window whose run does
// not end at the tail: a person scrolling down a run far back. Caller holds
// the fetch guard.
func (o *Owner) backfillNewer(ctx context.Context, a *seat, chat domain.Chat, w project.HistoryWindow, above int) {
	fetched, err := o.client.GetHistoryAfter(ctx, chat.Peer, w.TopicID, above, o.Config().UI.HistoryLimit)
	if err != nil {
		o.failedToFill(a, w, err)
		return
	}
	o.state.MergeNewer(w.History(), above, fetched)
	o.log.Debug("history backfill newer",
		zap.Int64("chat", w.ChatID), zap.Int("above", above), zap.Int("fetched", len(fetched)))
}

// jump fetches the page around a message the store does not hold and stores
// it as a run of its own: fetching everything between it and the tail could
// take thousands of pages, and storing it as if it joined would hide the
// history nobody fetched. A page without the message says it is gone, and the
// client is told which message of the page is nearest, to open there instead.
// Caller holds the fetch guard.
func (o *Owner) jump(ctx context.Context, a *seat, chat domain.Chat, w project.HistoryWindow, id int) {
	page, err := o.client.GetHistoryAround(ctx, chat.Peer, w.TopicID, id, o.Config().UI.HistoryLimit)
	if err != nil {
		o.failedToFill(a, w, err)
		return
	}
	o.state.MergeAround(w.History(), page)
	if !holds(page, id) {
		a.tell(MessageGone{ChatID: w.ChatID, TopicID: w.TopicID, MsgID: id, Nearest: nearest(page, id)})
	}
	o.log.Debug("history around a message",
		zap.Int64("chat", w.ChatID), zap.Int("id", id), zap.Int("fetched", len(page)))
}

// failedToFill tells the client its window could not be filled: otherwise the
// pane waits on a load that will never arrive.
func (o *Owner) failedToFill(a *seat, w project.HistoryWindow, err error) {
	o.log.Warn("history backfill failed", zap.Int64("chat", w.ChatID), zap.Error(err))
	a.tell(Failure{ChatID: w.ChatID, Op: "load history", Err: telerr.From(err)})
}

func holds(msgs []domain.Message, id int) bool {
	for _, m := range msgs {
		if m.ID == id {
			return true
		}
	}
	return false
}

// nearest is the message of a page closest to id: the newest one older than
// it, else the oldest one newer, zero for an empty page.
func nearest(msgs []domain.Message, id int) int {
	older, newer := 0, 0
	for _, m := range msgs {
		switch {
		case m.ID < id && m.ID > older:
			older = m.ID
		case m.ID > id && (newer == 0 || m.ID < newer):
			newer = m.ID
		}
	}
	if older != 0 {
		return older
	}
	return newer
}

// recordGap writes down that a chat stopped being able to receive what it
// missed, at the only moment the position can still be known: right now, before
// the messages that follow the hole land on the tail and hide it.
//
// A chat holding nothing records nothing. There is no hole after no message,
// and history nobody has fetched is what backfill is for.
func (o *Owner) recordGap(chatID int64) {
	st := o.state.Store()
	tail := st.TailMessageID(chatID)
	if tail == 0 {
		return
	}
	st.MarkGap(chatID, tail)
	o.log.Info("gap recorded", zap.Int64("chat", chatID), zap.Int("after", tail))
	if o.focus.focused(chatID) {
		// Somebody is reading this chat, and the stream that fills it has just
		// stopped being able to. Fetching the history directly is the only way
		// they see anything until it recovers, so it happens now rather than at
		// the next open.
		o.spawn(func() { o.forwardFill(o.ctx, chatID) })
	}
}

// scanForGaps answers the account-wide version of the same news, which names no
// chat. Reloading the dialog list is what makes the question answerable: it
// says where every chat now ends on Telegram's side, and a chat whose tail
// falls short of that missed the difference.
//
// The comparison is only honest here, before anything is appended to a tail
// this session. Once the stream resumes, an arriving message carries the tail
// past the hole and the two numbers agree again with the hole still in place.
func (o *Owner) scanForGaps(ctx context.Context) {
	chats, err := o.client.GetDialogs(ctx)
	if err != nil {
		o.log.Warn("gap scan could not reload the dialog list", zap.Error(err))
		return
	}
	o.state.SetDialogs(chats)

	st := o.state.Store()
	marked := 0
	for _, chat := range chats {
		tail := st.TailMessageID(chat.ID)
		if tail == 0 || chat.TopMessageID <= tail {
			continue
		}
		st.MarkGap(chat.ID, tail)
		marked++
		if o.focus.focused(chat.ID) {
			o.spawn(func() { o.forwardFill(ctx, chat.ID) })
		}
	}
	o.log.Info("gap scan done", zap.Int("dialogs", len(chats)), zap.Int("marked", marked))
}

// forwardFill closes a chat's recorded gap, fetching from the position it opens
// after towards the tail one page at a time. Each page joins the one before it,
// so the history is contiguous at every step and never holds a hole nothing
// knows about; see docs/adr/0014.
//
// The repair records its progress as it goes, so an interruption costs the page
// in flight rather than the whole run, and the next trigger continues from
// where this one stopped. It gives up on a gap wider than the chat could hold
// anyway and reloads the tail instead: filling that much would only push out
// what it just fetched.
//
// Caller holds the fetch guard.
func (o *Owner) forwardFill(ctx context.Context, chatID int64) {
	st := o.state.Store()
	if _, open := st.Gap(chatID); !open {
		return
	}
	chat, ok := st.GetChat(chatID)
	if !ok {
		return
	}
	if !o.beginRepair(chatID) {
		return
	}
	defer o.endRepair(chatID)

	limit := o.Config().UI.HistoryLimit
	// The chat cannot hold more than the cap, so a gap wider than it is one the
	// repair cannot win: every page past that point evicts one it already
	// fetched.
	budget := store.MaxMessagesPerChat
	for {
		// Read the position each time round rather than carrying it: a hole
		// opening underneath the repair records an earlier one, and that is
		// where the work has to continue from.
		after, open := st.Gap(chatID)
		if !open {
			return
		}
		page, err := o.client.GetHistoryAfter(ctx, chat.Peer, 0, after, limit)
		if err != nil {
			// Nobody asked for this fetch, so nobody is waiting to hear that it
			// failed: reporting it would interrupt a person over work they did
			// not start. The gap keeps its position and the next open,
			// reconnect or too-long carries on from there.
			o.log.Warn("gap repair failed",
				zap.Int64("chat", chatID), zap.Int("after", after), zap.Error(err))
			return
		}
		newest := 0
		if len(page) > 0 {
			newest = page[len(page)-1].ID
		}
		if newest <= after {
			// Telegram answers a request for messages newer than the newest by
			// sliding the window back over ones we have. Nothing moved forward,
			// so there is nothing left to close.
			o.log.Debug("gap closed", zap.Int64("chat", chatID), zap.Int("at", after))
			st.CloseGap(chatID, after)
			return
		}
		o.state.RepairHistory(chatID, page)
		o.log.Debug("gap repair page",
			zap.Int64("chat", chatID), zap.Int("fetched", len(page)), zap.Int("now_at", newest))
		if !st.AdvanceGap(chatID, after, newest) {
			// The record moved while the page was in flight, so it no longer
			// describes this repair's progress. Whatever it says now is the
			// truth, and the next turn of the loop reads it.
			continue
		}

		if chat.TopMessageID > 0 && newest >= chat.TopMessageID {
			// Caught up with where the server was when the dialog list was
			// read. Anything newer than that arrived through the update stream.
			st.CloseGap(chatID, newest)
			return
		}
		if budget -= len(page); budget <= 0 {
			o.tailReload(ctx, chat)
			return
		}
	}
}

// tailReload throws a chat's stored history away and takes a fresh page. It is
// the admission that the gap cannot be closed: what is held and what the server
// has cannot be joined, and joining them anyway would draw two ranges as one
// conversation.
//
// It is silent. The scrollback it drops comes back by scrolling, which is
// backfill's ordinary job, and a person who did nothing to cause this has
// nothing to do about it either.
func (o *Owner) tailReload(ctx context.Context, chat domain.Chat) {
	page, err := o.client.GetHistory(ctx, chat.Peer, 0, o.Config().UI.HistoryLimit)
	if err != nil {
		o.log.Warn("tail reload failed", zap.Int64("chat", chat.ID), zap.Error(err))
		return
	}
	if len(page) == 0 {
		return
	}
	o.state.ApplyHistory(chat.ID, page)
	o.state.Store().ClearGap(chat.ID)
	o.log.Info("tail reloaded: the gap was wider than the chat can hold",
		zap.Int64("chat", chat.ID), zap.Int("kept", len(page)))
	o.refillWindows(chat.ID)
}

// refillWindows checks again what every window on a chat lacks, for every
// client: a tail reload discards the runs those windows were on, the one on
// screen included, since it admits nothing stored can be trusted, and a window
// anchored far back fetches the page around its anchor again.
func (o *Owner) refillWindows(chatID int64) {
	for _, a := range o.attached() {
		for id, w := range a.registry.HistoryWindows() {
			if w.ChatID == chatID {
				o.maybeFetch(a, id, w)
			}
		}
	}
}

func (o *Owner) beginFetch(key subKey) bool {
	o.fetchMu.Lock()
	defer o.fetchMu.Unlock()
	if o.fetching[key] {
		return false
	}
	o.fetching[key] = true
	return true
}

func (o *Owner) endFetch(key subKey) {
	o.fetchMu.Lock()
	defer o.fetchMu.Unlock()
	delete(o.fetching, key)
}

func (o *Owner) beginRepair(chatID int64) bool {
	o.fetchMu.Lock()
	defer o.fetchMu.Unlock()
	if o.repairing[chatID] {
		return false
	}
	o.repairing[chatID] = true
	return true
}

func (o *Owner) endRepair(chatID int64) {
	o.fetchMu.Lock()
	defer o.fetchMu.Unlock()
	delete(o.repairing, chatID)
}
