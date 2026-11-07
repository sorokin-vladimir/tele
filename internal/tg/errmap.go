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
		if te.Type == "AUTH_KEY_DUPLICATED" {
			// Whichever request it came back on, the key is dead for all of
			// them: the connection ends here rather than one failure at a time.
			c.loggedOut.report(telerr.LogOutKeyDropped)
		}
		if kind == telerr.Internal && c.log != nil {
			// The kind set is closed, the mapping table is not. Unmapped types
			// are logged so the table is extended on evidence rather than by
			// guessing, and so Internal does not become a silent dumping ground.
			c.log.Warn("unmapped telegram error",
				zap.String("op", op), zap.String("type", te.Type), zap.Int("code", te.Code))
		}
		mapped := &telerr.Error{
			Kind:       kind,
			Op:         op,
			Detail:     te.Type,
			Reason:     reason,
			RetryAfter: retryAfter,
			Transient:  kind == telerr.Network,
			Cause:      err,
		}
		if kind == telerr.Unauthorized {
			mapped.LogOut = logOutOf(te)
			// Once logged in, a refusal is checked against the main data
			// centre before anything ends (#297). A dropped key needs no
			// check: Telegram has said it is dead everywhere.
			if c.authorized.Load() && mapped.LogOut != telerr.LogOutKeyDropped {
				c.recheck.start(c.checkSession)
			}
		}
		return mapped
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
//
// 406 has no row of its own. Telegram uses it for refusals of every sort, most
// of them raised while logging in (a mistyped number, too many attempts, a
// client too old to log in), so as a family it says nothing about the session.
// The 406s that are mapped are mapped by type; the rest stay Internal.
func classifyTgErr(e *tgerr.Error) (telerr.Kind, telerr.Reason, time.Duration) {
	// Ahead of the codes: a refused app key is not about the session, and it
	// arrives as a 400 or a 406 that the codes alone cannot tell from any other
	// refusal.
	switch e.Type {
	case "API_ID_PUBLISHED_FLOOD", "API_ID_INVALID":
		return telerr.AppKeyBlocked, "", 0
	// Ahead of the codes too: a ban on the session arrives as a 401, which the
	// code table would read as a log out and answer with a login that meets
	// the same ban. The number's ban is a 400 raised while logging in (#254).
	case "USER_DEACTIVATED_BAN", "PHONE_NUMBER_BANNED":
		return telerr.AccountBanned, "", 0
	// The one 406 that is about the session: Telegram has already dropped it,
	// and the key must be replaced before logging in again (#254).
	case "AUTH_KEY_DUPLICATED":
		return telerr.Unauthorized, "", 0
	}

	switch {
	case e.Code == 420:
		// FLOOD_WAIT, FLOOD_PREMIUM_WAIT and SLOWMODE_WAIT all carry the wait
		// in Argument.
		return telerr.RateLimited, "", time.Duration(e.Argument) * time.Second
	case e.Code == 401:
		// Every other 401 is a log out, a deleted Telegram account included:
		// logging in with the same number makes a new account.
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
		"USERNAME_NOT_OCCUPIED", "USERNAME_INVALID", "PEER_ID_NOT_SUPPORTED":
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

// logOutOf says how an Unauthorized error logged the account out. Every 401 is
// a log out, so anything not named here was ended elsewhere (#297).
func logOutOf(e *tgerr.Error) telerr.LogOut {
	switch e.Type {
	case "USER_DEACTIVATED":
		return telerr.LogOutDeleted
	case "AUTH_KEY_DUPLICATED":
		return telerr.LogOutKeyDropped
	default:
		return telerr.LogOutElsewhere
	}
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
