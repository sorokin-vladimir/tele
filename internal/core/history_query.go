package core

import (
	"context"
	"slices"

	"github.com/sorokin-vladimir/tele/internal/domain"
	"github.com/sorokin-vladimir/tele/internal/telerr"
)

// History answers with up to limit messages of history h older than before,
// or the newest when before is zero, oldest first. It is a command's way to
// read: the run that reaches up to before answers, the tail run for zero, and
// what it lacks is fetched page by page back from its oldest message, as a
// window's backfill fetches it, and kept (#181).
//
// With no run reaching up to before, the page older than before is fetched and
// kept as a run of its own: the stretch between it and the rest stays
// unclaimed, so nothing reads it as adjacent, and the next command asking for
// it is answered from the store (#347). A chat known by its address alone is
// not kept, since the account has no dialog with it.
func (o *Owner) History(ctx context.Context, h domain.HistoryKey, before, limit int) ([]domain.Message, error) {
	chat, err := o.chatOf(ctx, h.ChatID)
	if err != nil {
		return nil, err
	}
	if chat.IsForum && h.TopicID == 0 {
		return nil, &telerr.Error{Kind: telerr.NotFound, Op: OpNoTopic, Detail: "a forum's messages are its topics'"}
	}
	peer := chat.Peer
	st := o.state.Store()
	_, held := st.GetChat(h.ChatID)
	fetch := func(offsetID int) ([]domain.Message, error) {
		page := max(limit, o.Config().UI.HistoryLimit)
		if h.TopicID != 0 {
			return o.client.GetReplies(ctx, peer, h.TopicID, offsetID, page)
		}
		return o.client.GetHistory(ctx, peer, offsetID, page)
	}
	if !held {
		page, err := fetch(before)
		return newestOf(page, before, limit), err
	}

	st.LoadMessages(h)
	o.forwardFill(ctx, h.ChatID)
	for {
		run, ok := runUpTo(st.Runs(h), before)
		if !ok {
			// Nothing held reaches up to before: the page under it reaches up
			// to before, and joins the run holding before or is a run of its
			// own; for zero it is the newest page of a history holding no tail
			// run.
			page, err := fetch(before)
			if err != nil {
				return nil, err
			}
			o.state.MergeOlder(h, before, page)
			if len(page) == 0 {
				return nil, nil
			}
			continue
		}
		got := newestOf(run.Messages, before, limit)
		if len(got) == limit || run.AtBeginning {
			return got, nil
		}
		oldest := run.Messages[0].ID
		page, err := fetch(oldest)
		if err != nil {
			return nil, err
		}
		if _, added := o.state.MergeOlder(h, oldest, page); !added {
			// Nothing new: the run reaches the beginning now.
			return got, nil
		}
	}
}

// runUpTo is the run whose messages older than before reach up to it with
// nothing missing between: the tail run when before is zero, else the run
// holding a message older than before that reaches up to before, or ends at
// the tail. A run that ends below before is not it, since what lies between
// was never fetched.
func runUpTo(runs []domain.Run, before int) (domain.Run, bool) {
	for _, r := range runs {
		if len(r.Messages) == 0 {
			continue
		}
		if before == 0 {
			if r.AtTail {
				return r, true
			}
			continue
		}
		if r.Messages[0].ID < before && (before <= r.UpTo || r.AtTail) {
			return r, true
		}
	}
	return domain.Run{}, false
}

// newestOf is the newest limit messages of msgs older than before, every one
// when before is zero, oldest first.
func newestOf(msgs []domain.Message, before, limit int) []domain.Message {
	var out []domain.Message
	for _, m := range msgs {
		if before == 0 || m.ID < before {
			out = append(out, m)
		}
	}
	slices.SortFunc(out, func(a, b domain.Message) int { return a.ID - b.ID })
	if len(out) > limit {
		out = out[len(out)-limit:]
	}
	return out
}
