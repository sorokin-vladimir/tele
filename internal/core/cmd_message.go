package core

import (
	"context"
	"time"

	"go.uber.org/zap"

	"github.com/sorokin-vladimir/tele/internal/core/state"
	"github.com/sorokin-vladimir/tele/internal/domain"
	"github.com/sorokin-vladimir/tele/internal/telerr"
)

// messageByID returns a copy of one stored message, so a caller can keep the
// pre-change value for a rollback or resolve media on it. It is a function
// rather than a method because the media fetcher needs it without an Owner.
func messageByID(s *state.State, chatID int64, msgID int) (domain.Message, error) {
	for _, m := range s.Store().Messages(chatID) {
		if m.ID == msgID {
			return m, nil
		}
	}
	return domain.Message{}, &telerr.Error{Kind: telerr.NotFound}
}

func (o *Owner) messageByID(chatID int64, msgID int) (domain.Message, error) {
	return messageByID(o.state, chatID, msgID)
}

// Forward copies messages into another chat, optionally preceded by a comment.
//
// Both ends are chat IDs like every other command. A target the account has no
// dialog with, such as a search hit, resolves through the address search left
// behind (#278).
func (o *Owner) Forward(ctx context.Context, fromChatID, toChatID int64, msgIDs []int, comment string) error {
	// Forwarding crosses four layers (client -> owner -> tg -> update stream)
	// and shows nothing until the last one delivers, so each step says what it
	// did: "forward:" in the log is the whole path (#198).
	o.log.Debug("forward: requested",
		zap.Int64("from_chat", fromChatID),
		zap.Int64("to_chat", toChatID),
		zap.Ints("msg_ids", msgIDs),
		zap.Bool("with_comment", comment != ""))
	from, err := o.peer(fromChatID)
	if err != nil {
		o.log.Debug("forward: source peer not resolved", zap.Int64("from_chat", fromChatID))
		return err
	}
	// Resolved before the comment is queued: a target nobody can reach would
	// leave the comment behind with nothing to follow it.
	to, err := o.peer(toChatID)
	if err != nil {
		o.log.Debug("forward: target peer not resolved", zap.Int64("to_chat", toChatID))
		return err
	}
	if comment != "" {
		// The comment goes through the durable queue like any other message. The
		// ApplyIncoming that used to sit here existed only because echo
		// suppression hid the comment and no optimistic bubble would ever show
		// it; with suppression gone for text it arrives the ordinary way (#193).
		if err := o.Send(ctx, SendRequest{Ref: NewRef(), ChatID: toChatID, Text: comment}); err != nil {
			o.log.Debug("forward: comment could not be queued", zap.Error(err))
			return err
		}
		o.log.Debug("forward: comment queued", zap.Int64("to_chat", toChatID))
	}
	if err := o.client.ForwardMessages(ctx, from, to, msgIDs); err != nil {
		o.log.Debug("forward: telegram refused", zap.Error(err))
		return err
	}
	o.bumpForwardTarget(fromChatID, toChatID, msgIDs)
	o.log.Debug("forward: done, target bumped",
		zap.Int64("to_chat", toChatID),
		zap.Int("held_in_store", len(o.state.Store().Messages(toChatID))))
	return nil
}

// bumpForwardTarget gives the target chat a last-message preview built from the
// forwarded source message, so it surfaces in the list at once. A target the
// owner does not hold is a no-op inside BumpChatLastMessage, which is correct:
// there is no row to bump yet.
func (o *Owner) bumpForwardTarget(fromChatID, toChatID int64, msgIDs []int) {
	preview := domain.Message{ChatID: toChatID, IsOut: true, Date: time.Now()}
	if len(msgIDs) > 0 {
		if src, err := o.messageByID(fromChatID, msgIDs[0]); err == nil {
			preview.Text = src.Text
			preview.Forward = src.Forward
		}
	}
	o.state.Store().BumpChatLastMessage(toChatID, preview)
	// BumpChatLastMessage is a store write with no state entry point of its own,
	// so the projections are rebuilt explicitly.
	o.Refresh()
}

// SendReaction sets or retracts our reaction on a message.
func (o *Owner) SendReaction(ctx context.Context, chatID int64, msgID int, emoji string) error {
	peer, err := o.peer(chatID)
	if err != nil {
		return err
	}
	msg, err := o.messageByID(chatID, msgID)
	if err != nil {
		return err
	}
	prev := make([]domain.Reaction, len(msg.Reactions))
	copy(prev, msg.Reactions)
	next := optimisticReactions(prev, emoji)
	// Reaction trace (#248): the store logs every set it writes; these lines say
	// which of those writes were ours and what was picked.
	o.log.Debug("reaction: optimistic",
		zap.Int64("chat_id", chatID), zap.Int("msg_id", msgID), zap.String("picked", emoji),
		zap.String("was", domain.FormatReactions(prev)), zap.String("now", domain.FormatReactions(next)))
	o.state.ApplyReactions(chatID, msgID, next, false)
	if err := o.client.SendReaction(ctx, peer, msgID, reactionToSend(prev, emoji)); err != nil {
		o.log.Debug("reaction: rollback",
			zap.Int64("chat_id", chatID), zap.Int("msg_id", msgID),
			zap.String("to", domain.FormatReactions(prev)), zap.Error(err))
		o.state.ApplyReactions(chatID, msgID, prev, false)
		return err
	}
	o.log.Debug("reaction: sent", zap.Int64("chat_id", chatID), zap.Int("msg_id", msgID))
	return nil
}

// reactionToSend is the emoji sent to Telegram: empty retracts, which is what
// picking the already-chosen reaction means.
func reactionToSend(current []domain.Reaction, emoji string) string {
	for _, r := range current {
		if r.Emoji == emoji && r.IsChosen {
			return ""
		}
	}
	return emoji
}

// optimisticReactions returns what a message's reactions look like right after
// the user picks an emoji, so the choice shows before the server answers.
// Picking the already-chosen emoji retracts it. This lives here rather than in
// the UI because what a reaction looks like is state, not rendering (#198).
func optimisticReactions(current []domain.Reaction, emoji string) []domain.Reaction {
	alreadyChosen := false
	for _, r := range current {
		if r.Emoji == emoji && r.IsChosen {
			alreadyChosen = true
			break
		}
	}
	out := make([]domain.Reaction, 0, len(current)+1)
	emojiFound := false
	for _, r := range current {
		nr := r
		if r.Emoji == emoji {
			emojiFound = true
			if alreadyChosen {
				nr.IsChosen = false
				nr.Count--
				if nr.Count <= 0 {
					continue
				}
			} else {
				nr.IsChosen = true
				nr.Count++
			}
		} else if r.IsChosen {
			nr.IsChosen = false
			nr.Count--
			if nr.Count <= 0 {
				continue
			}
		}
		out = append(out, nr)
	}
	if !alreadyChosen && !emojiFound && emoji != "" {
		out = append(out, domain.Reaction{Emoji: emoji, Count: 1, IsChosen: true})
	}
	return out
}

// DeleteMessages deletes messages, for everyone when revoke is set. They leave
// the window at once and come back if Telegram refuses.
func (o *Owner) DeleteMessages(ctx context.Context, chatID int64, msgIDs []int, revoke bool) error {
	peer, err := o.peer(chatID)
	if err != nil {
		return err
	}
	removed := make([]domain.Message, 0, len(msgIDs))
	for _, id := range msgIDs {
		m, err := o.messageByID(chatID, id)
		if err != nil {
			continue // already gone: nothing to remove, nothing to restore
		}
		removed = append(removed, m)
	}
	o.state.ApplyDelete(chatID, msgIDs)
	if err := o.client.DeleteMessages(ctx, peer, msgIDs, revoke); err != nil {
		for _, m := range removed {
			o.state.ApplyRestore(m)
		}
		return err
	}
	return nil
}

// EditMessage rewrites one of our messages. The new text is shown before the
// request so the chat does not stutter, and the previous version is restored if
// Telegram refuses.
func (o *Owner) EditMessage(ctx context.Context, chatID int64, msgID int, text string, entities []domain.MessageEntity) error {
	peer, err := o.peer(chatID)
	if err != nil {
		return err
	}
	prev, err := o.messageByID(chatID, msgID)
	if err != nil {
		return err
	}
	edited := prev
	edited.Text = text
	edited.Entities = entities
	now := time.Now()
	edited.EditDate = &now
	o.state.ApplyEdit(edited)
	if err := o.client.EditMessage(ctx, peer, msgID, text, entities); err != nil {
		o.state.ApplyEditRestore(prev)
		return err
	}
	return nil
}
