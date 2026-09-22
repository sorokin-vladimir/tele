package tg

import (
	"context"
	"errors"
	"net"
	"strings"
	"time"

	"github.com/gotd/td/bin"
	"github.com/gotd/td/rpc"
	"github.com/gotd/td/telegram"
	gotdtg "github.com/gotd/td/tg"
	"github.com/gotd/td/tgerr"
	"go.uber.org/zap"

	"github.com/sorokin-vladimir/tele/internal/telerr"
)

// mapError converts any error leaving this package into a domain error. It is
// called once per RPC by the error middleware, so no method has to remember to
// wrap its own result.
//
// Context errors are control flow rather than domain failures and pass through
// untouched, which lets the UI stay silent when the user cancels.
func (c *GotdClient) mapError(op string, err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return err
	}
	if _, ok := telerr.As(err); ok {
		return err
	}

	var te *tgerr.Error
	if errors.As(err, &te) {
		kind, reason, retryAfter := classifyTgErr(te)
		if kind == telerr.Internal && c.log != nil {
			// The kind set is closed, the mapping table is not. Unmapped types
			// are logged so the table is extended on evidence rather than by
			// guessing, and so Internal does not become a silent dumping ground.
			c.log.Warn("unmapped telegram error",
				zap.String("op", op), zap.String("type", te.Type), zap.Int("code", te.Code))
		}
		return &telerr.Error{
			Kind:       kind,
			Op:         op,
			Detail:     te.Type,
			Reason:     reason,
			RetryAfter: retryAfter,
			Transient:  kind == telerr.Network,
			Cause:      err,
		}
	}

	var ne net.Error
	if errors.As(err, &ne) {
		return &telerr.Error{Kind: telerr.Network, Op: op, Detail: ne.Error(), Transient: true, Cause: err}
	}

	// gotd's own transport failures. Neither is a tgerr and neither satisfies
	// net.Error, so both used to land in Internal — which is terminal, so an
	// offline send was given up on instead of waited out (#193).
	//
	// RetryLimitReachedErr is gotd saying the server never acknowledged the
	// request. The request may well have arrived: repeating it is safe only
	// because the caller keeps its random_id, which is what the outbox is for.
	if errors.Is(err, &rpc.RetryLimitReachedErr{}) || errors.Is(err, rpc.ErrEngineClosed) {
		return &telerr.Error{Kind: telerr.Network, Op: op, Detail: err.Error(), Transient: true, Cause: err}
	}

	return &telerr.Error{Kind: telerr.Internal, Op: op, Detail: err.Error(), Cause: err}
}

// errorMiddleware maps the error of every RPC exactly once. gotd routes all
// calls through this invoker, including the downloader and the uploader, so no
// call site has to remember the conversion and a method added later cannot
// forget it.
func (c *GotdClient) errorMiddleware() telegram.Middleware {
	return telegram.MiddlewareFunc(func(next gotdtg.Invoker) telegram.InvokeFunc {
		return func(ctx context.Context, input bin.Encoder, output bin.Decoder) error {
			err := next.Invoke(ctx, input, output)
			if err == nil {
				// A reply came back, so the clock check let it through: whatever
				// skew there was is over (#277).
				c.skew.passed()
			}
			return c.mapError(opName(input), err)
		}
	})
}

// opName reads the TL method name off the request, e.g. "messages.sendMessage".
func opName(in bin.Encoder) string {
	if named, ok := in.(interface{ TypeName() string }); ok {
		return named.TypeName()
	}
	return "telegram"
}

// classifyTgErr maps one gotd error onto a kind, the reason behind a Rejected
// one, and a wait for RateLimited. Codes decide first because they are stable;
// types refine the 400s.
func classifyTgErr(e *tgerr.Error) (telerr.Kind, telerr.Reason, time.Duration) {
	// Ahead of the codes: a refused app key arrives as a 406, which the code
	// table would otherwise read as an expired session and send the person into
	// a login that offers the same key again.
	switch e.Type {
	case "API_ID_PUBLISHED_FLOOD", "API_ID_INVALID":
		return telerr.AppKeyBlocked, "", 0
	}

	switch {
	case e.Code == 420:
		// FLOOD_WAIT, FLOOD_PREMIUM_WAIT and SLOWMODE_WAIT all carry the wait
		// in Argument.
		return telerr.RateLimited, "", time.Duration(e.Argument) * time.Second
	case e.Code == 401 || e.Code == 406:
		return telerr.Unauthorized, "", 0
	case e.Code == 403:
		return telerr.Forbidden, "", 0
	case e.Code >= 500:
		return telerr.Network, "", 0
	}

	if reason, ok := rejectionReasons[e.Type]; ok {
		return telerr.Rejected, reason, 0
	}

	switch e.Type {
	case "PEER_ID_INVALID", "CHANNEL_INVALID", "CHAT_ID_INVALID", "USER_ID_INVALID",
		"USERNAME_NOT_OCCUPIED", "PEER_ID_NOT_SUPPORTED":
		return telerr.PeerNotFound, "", 0
	case "CHAT_WRITE_FORBIDDEN", "CHAT_FORWARDS_RESTRICTED", "USER_BANNED_IN_CHANNEL",
		"CHAT_ADMIN_REQUIRED", "MESSAGE_DELETE_FORBIDDEN":
		return telerr.Forbidden, "", 0
	case "FILE_REFERENCE_EXPIRED", "FILE_REFERENCE_INVALID", "FILE_REFERENCE_EMPTY":
		return telerr.StaleReference, "", 0
	case "MESSAGE_ID_INVALID", "MSG_ID_INVALID", "RANDOM_ID_INVALID":
		return telerr.NotFound, "", 0
	}

	// Net for the families too large to list: CHAT_SEND_*_FORBIDDEN alone is
	// eight distinct types.
	if strings.HasSuffix(e.Type, "_FORBIDDEN") || strings.HasSuffix(e.Type, "_RESTRICTED") {
		return telerr.Forbidden, "", 0
	}

	return telerr.Internal, "", 0
}

// rejectionReasons is the closed list of refusals we can explain. It is a list
// and not a rule — an unrecognised 400 stays Internal, is logged as unmapped and
// joins this table on evidence.
//
// Sorting every unknown 400 in here would look tidier and would be worse: the
// difference between "Telegram refused your photo" and "we do not know what
// happened" is exactly what a person needs, and Internal is where the second one
// is admitted. Several types share a reason wherever the remedy is the same.
var rejectionReasons = map[string]telerr.Reason{
	"PHOTO_EXT_INVALID":        telerr.ReasonPhotoType,
	"PHOTO_INVALID_DIMENSIONS": telerr.ReasonPhotoDimensions,
	"PHOTO_SAVE_FILE_INVALID":  telerr.ReasonMediaUnreadable,
	"IMAGE_PROCESS_FAILED":     telerr.ReasonMediaUnreadable,
	"VIDEO_FILE_INVALID":       telerr.ReasonMediaUnreadable,
	"MEDIA_INVALID":            telerr.ReasonMediaUnsupported,
	"MEDIA_EMPTY":              telerr.ReasonMediaUnsupported,
	"MESSAGE_EMPTY":            telerr.ReasonTextEmpty,
	"MESSAGE_TOO_LONG":         telerr.ReasonTextTooLong,
	"ENTITIES_TOO_LONG":        telerr.ReasonMarkupTooLong,
}
