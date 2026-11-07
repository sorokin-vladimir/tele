package core

import (
	"context"
	"os"

	"github.com/gotd/td/tg"
	"go.uber.org/zap"

	"github.com/sorokin-vladimir/tele/internal/domain"
	"github.com/sorokin-vladimir/tele/internal/telerr"
	internaltg "github.com/sorokin-vladimir/tele/internal/tg"
)

// attemptMedia sends one album group: upload every part, convert them into
// server-side refs when there is more than one, then send. One entry is one
// group, so it lands whole or not at all — a part that will not upload takes the
// group with it rather than producing an album of unknown composition.
func (o *Owner) attemptMedia(ctx context.Context, e domain.OutboxEntry) {
	peer, err := o.peer(e.ChatID)
	if err != nil {
		o.recordFailure(e, err)
		return
	}
	if e.Media == nil || len(e.Media.Parts) == 0 {
		o.recordFailure(e, &telerr.Error{Kind: telerr.Internal, Op: "outbox.media", Detail: "empty entry"})
		return
	}
	if err := checkParts(e.Media.Parts); err != nil {
		o.recordFailure(e, err)
		return
	}

	uctx, cancel := o.beginUpload(ctx, e.Ref)
	defer o.endUpload(e.Ref, cancel)

	e.State = domain.OutboxUploading
	if err := o.outbox.Update(e); err != nil {
		o.log.Error("outbox: could not mark an entry uploading", zap.Error(err))
		return
	}
	o.Refresh()

	o.log.Debug("outbox: uploading",
		zap.String("ref", e.Ref), zap.Int64("chat_id", e.ChatID), zap.Int("parts", len(e.Media.Parts)))

	uploaded, err := o.uploadGroup(uctx, e, peer)
	if o.abandoned(uctx, e.Ref) {
		return
	}
	if err != nil {
		o.recordFailure(e, err)
		return
	}

	e.State = domain.OutboxSending
	if err := o.outbox.Update(e); err != nil {
		o.log.Error("outbox: could not mark an entry in flight", zap.Error(err))
		return
	}
	o.Refresh()

	ids, err := o.sendGroup(uctx, e, peer, uploaded)
	if o.abandoned(uctx, e.Ref) {
		return
	}
	if err != nil {
		o.recordFailure(e, err)
		return
	}
	o.recordSentMedia(e, peer, ids)
}

// checkParts confirms every file is still there before any bytes go up. A file
// deleted between submission and this attempt cannot be recovered by retrying,
// so the group fails now, named, rather than uploading its siblings first and
// then reporting whatever the uploader happened to say about a missing path.
func checkParts(parts []domain.OutboxMediaPart) error {
	for _, p := range parts {
		if _, err := os.Stat(p.Path); err != nil {
			return &telerr.Error{
				Kind:   telerr.NotFound,
				Op:     "outbox.media",
				Detail: "cannot read " + p.Name,
				Cause:  err,
			}
		}
	}
	return nil
}

// uploadGroup uploads every part, reporting progress aggregated over the whole
// entry. Parts of a group of more than one take the uploadMedia hop, because
// sendMultiMedia rejects inputMediaUploaded* constructors; a lone part goes
// straight to sendMedia and is spared the round-trip.
func (o *Owner) uploadGroup(ctx context.Context, e domain.OutboxEntry, peer domain.Peer) ([]tg.InputMediaClass, error) {
	parts := e.Media.Parts
	var total int64
	for _, p := range parts {
		total += p.Size
	}

	out := make([]tg.InputMediaClass, 0, len(parts))
	var done int64
	for i, p := range parts {
		idx, base := i+1, done
		m, err := o.uploadPart(ctx, p, func(sent, _ int64) {
			o.publishProgress(Progress{
				ChatID: e.ChatID, Ref: e.Ref,
				Part: idx, Parts: len(parts),
				Done: base + sent, Total: total,
			})
		})
		if err != nil {
			return nil, err
		}
		done += p.Size
		if len(parts) > 1 {
			m, err = o.client.UploadMedia(ctx, peer, m)
			if err != nil {
				return nil, err
			}
		}
		out = append(out, m)
	}
	return out, nil
}

// sendGroup issues the one request this entry is: a grouped album, or a plain
// media message when the group holds a single part.
func (o *Owner) sendGroup(ctx context.Context, e domain.OutboxEntry, peer domain.Peer, uploaded []tg.InputMediaClass) ([]int, error) {
	if len(uploaded) == 1 {
		id, err := o.client.SendMedia(ctx, internaltg.SendMediaParams{
			Peer: peer, Media: uploaded[0],
			Caption: e.Media.Caption, Entities: o.addressMentions(e.Media.Entities),
			ReplyToMsgID: e.Media.ReplyToMsgID,
			TopicID:      e.TopicID,
			RandomID:     mediaRandomID(e.Ref, 0),
		})
		if err != nil {
			return nil, err
		}
		return []int{id}, nil
	}

	items := make([]internaltg.AlbumItem, 0, len(uploaded))
	randomIDs := make([]int64, 0, len(uploaded))
	for i, m := range uploaded {
		item := internaltg.AlbumItem{Media: m}
		// Telegram shows an album's caption from its first part.
		if i == 0 {
			item.Caption = e.Media.Caption
			item.Entities = o.addressMentions(e.Media.Entities)
		}
		items = append(items, item)
		randomIDs = append(randomIDs, mediaRandomID(e.Ref, i))
	}
	return o.client.SendAlbum(ctx, internaltg.SendAlbumParams{
		Peer: peer, Items: items,
		ReplyToMsgID: e.Media.ReplyToMsgID,
		TopicID:      e.TopicID,
		RandomIDs:    randomIDs,
	})
}

// recordSentMedia puts the sent messages into domain state, which is what makes
// the pending bubble become a real album.
//
// The send answers with ids and nothing else, so the messages are re-fetched:
// that is where the server's media refs and the grouped_id that collapses the
// parts come from. The fetch runs on the owner's context, never the upload's —
// the upload context is cancelled the moment this send finishes or is discarded,
// and tying the fetch to it made it fail every time, leaving the parts with no
// media, no grouping and no caption (the lesson of #130).
func (o *Owner) recordSentMedia(e domain.OutboxEntry, peer domain.Peer, ids []int) {
	if len(ids) == 0 {
		// The send succeeded but named no message. Nothing can be recorded and
		// nothing should be re-sent: drop the entry and let the next history
		// fetch surface the messages.
		o.log.Warn("outbox: media send confirmed no message id", zap.String("ref", e.Ref))
		o.dropSent(e)
		o.Refresh()
		return
	}
	e.SentMsgIDs = ids
	if err := o.outbox.Update(e); err != nil {
		o.log.Error("outbox: could not record a send", zap.Error(err))
	}
	o.log.Debug("outbox: sent media", zap.String("ref", e.Ref), zap.Ints("msg_ids", ids))

	msgs, err := o.client.RefreshMessages(o.ctx, peer, ids)
	if err != nil {
		// The messages exist server-side; only our view of them is missing. The
		// next history fetch surfaces them, and a bubble stuck at "sending"
		// would be the worse lie.
		o.log.Warn("outbox: could not refresh a sent album",
			zap.String("ref", e.Ref), zap.Error(err))
		o.dropSent(e)
		o.Refresh()
		return
	}
	// Applying commits, the commit listener runs clearSentOutbox, and that is
	// where the row goes — so the pending bubble and the album swap inside one
	// delta instead of across two with a frame showing neither.
	for _, m := range msgs {
		o.state.ApplyIncoming(m)
	}

	// The listener normally has the row already. This is the backstop for the
	// case where it did not.
	o.spawn(func() { o.dropIfUndelivered(e.Ref) })
}

// beginUpload derives the cancellable context this entry's upload runs on and
// registers it, so a discard can stop the bytes instead of waiting out a large
// file.
func (o *Owner) beginUpload(ctx context.Context, ref string) (context.Context, context.CancelFunc) {
	uctx, cancel := context.WithCancel(ctx)
	o.uploadMu.Lock()
	o.uploadCancels[ref] = cancel
	o.uploadMu.Unlock()
	return uctx, cancel
}

// endUpload unregisters and releases the entry's upload context.
func (o *Owner) endUpload(ref string, cancel context.CancelFunc) {
	o.uploadMu.Lock()
	delete(o.uploadCancels, ref)
	o.uploadMu.Unlock()
	cancel()
}

// abandoned reports that this attempt should end quietly: the owner is going
// away, or the entry was discarded under it. Neither is a failure to record, and
// recording one would resurrect a row the user asked to be rid of.
func (o *Owner) abandoned(ctx context.Context, ref string) bool {
	if ctx.Err() != nil {
		return true
	}
	_, still := o.outbox.Get(ref)
	return !still
}
