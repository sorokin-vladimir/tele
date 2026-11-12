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
	if needsBackfill(project.BuildHistory(o.reader(), w), w) {
		o.backfill(ctx, a, w)
	}
}

// backfill fetches older history for a chat subscription whose window the store
// could not fill and applies it to state; the registry then emits the resulting
// delta through the same path as any other change. Caller holds the fetch guard.
// A failure is told to a, the client whose window it was: nobody else asked.
func (o *Owner) backfill(ctx context.Context, a *seat, w project.HistoryWindow) {
	chat, ok := o.state.Store().GetChat(w.ChatID)
	if !ok {
		return
	}
	held := o.state.Store().Messages(w.History())
	// Page backwards from the oldest message held; zero means "from the newest",
	// which is what an empty window needs.
	offsetID := 0
	if len(held) > 0 {
		offsetID = held[0].ID
	}
	// Read here rather than kept in a field: a changed history limit applies to
	// the next backfill, which is this one.
	limit := o.Config().UI.HistoryLimit
	var fetched []domain.Message
	var err error
	if w.TopicID != 0 {
		// A topic's scrollback is its own thread. The forum's history would
		// bring every other topic's messages in its place.
		fetched, err = o.client.GetReplies(ctx, chat.Peer, w.TopicID, offsetID, limit)
	} else {
		fetched, err = o.client.GetHistory(ctx, chat.Peer, offsetID, limit)
	}
	if err != nil {
		o.log.Warn("history backfill failed", zap.Int64("chat", w.ChatID), zap.Error(err))
		// The client asked for a window it cannot fill itself, so it has to be
		// told: otherwise the pane waits on a load that will never arrive.
		a.tell(Failure{ChatID: w.ChatID, Op: "load history", Err: telerr.From(err)})
		return
	}
	// The store merges under its own lock, so a message that arrives mid-fetch
	// is not written back out of existence. Nothing added means the page was
	// entirely history the store already had: the chat has no more to give, and
	// there is no window to rebuild.
	_, added := o.state.MergeOlder(w.History(), offsetID, fetched)
	o.log.Debug("history backfill",
		zap.Int64("chat", w.ChatID),
		zap.Int("offset_id", offsetID),
		zap.Int("held", len(held)),
		zap.Int("fetched", len(fetched)),
		zap.Bool("added", added),
		zap.Int("want", w.Before+w.After+1))
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
		page, err := o.client.GetHistoryAfter(ctx, chat.Peer, after, limit)
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
