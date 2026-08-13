package core

import (
	"context"
	"os"
	"strconv"

	"go.uber.org/zap"
	"golang.org/x/sync/singleflight"

	"github.com/sorokin-vladimir/tele/internal/mediacache"
	"github.com/sorokin-vladimir/tele/internal/telerr"
	internaltg "github.com/sorokin-vladimir/tele/internal/tg"
)

// avatarFetcher downloads avatars on behalf of clients. It sits beside
// mediaFetcher rather than inside it, because the two share a shape and nothing
// else (#223):
//
//   - An avatar is addressed by peer and photo id, neither of which expires, so
//     none of mediaFetcher's file-reference refresh machinery applies here.
//   - It hangs off a person rather than off a message, so there is no message to
//     resolve and no state to consult.
//   - It has its own cache with its own bound, so a busy chat cannot evict the
//     avatars of everyone in the chat list.
type avatarFetcher struct {
	client internaltg.Client
	log    *zap.Logger

	// cache is nil until the app supplies one, like mediaFetcher's: a fetch
	// with nowhere to put the file fails rather than inventing a location.
	cache *mediacache.Cache

	// inflight collapses concurrent fetches of one avatar into a single
	// download. A profile opened twice in quick succession asks twice.
	inflight singleflight.Group
}

func newAvatarFetcher(client internaltg.Client, log *zap.Logger) *avatarFetcher {
	return &avatarFetcher{client: client, log: log}
}

// avatarCacheKey names the file in the cache. The avatar id alone would do —
// Telegram photo ids are unique — but that uniqueness is Telegram's promise
// rather than ours, and the owner costs nothing here: the name is still digits
// only, and a cache directory that says whose picture each file is can be read
// by a person chasing a bug about the wrong face.
//
// The id is what makes a changed avatar a different file, which is the whole of
// the staleness story: nothing has to notice the change.
func avatarCacheKey(userID, avatarID int64) string {
	return "avatar_" + strconv.FormatInt(userID, 10) + "_" + strconv.FormatInt(avatarID, 10)
}

// Fetch returns a path to the person's avatar, downloading it into the cache on
// a miss. addr says how the person is reachable; the caller resolves it.
func (f *avatarFetcher) Fetch(ctx context.Context, addr internaltg.UserAddress, avatarID int64) (string, error) {
	if f.cache == nil {
		return "", &telerr.Error{Kind: telerr.Internal, Op: "fetch avatar", Detail: "no avatar cache"}
	}
	if avatarID == 0 {
		// The client should not have asked: a person with no avatar is drawn as
		// a monogram without a round trip. Say so rather than downloading
		// nothing slowly.
		return "", &telerr.Error{Kind: telerr.NotFound, Op: "fetch avatar", Detail: "user has no avatar"}
	}
	key := avatarCacheKey(addr.UserID, avatarID)
	if p, ok := f.cache.Path(key); ok {
		return p, nil
	}
	path, err, _ := f.inflight.Do(key, func() (any, error) {
		// Another caller may have finished while this one waited.
		if p, ok := f.cache.Path(key); ok {
			return p, nil
		}
		f.log.Debug("avatar: downloading", zap.String("key", key))
		return f.cache.Put(key, func(file *os.File) error {
			return f.client.DownloadUserAvatarToFile(ctx, addr, avatarID, file)
		})
	})
	if err != nil {
		// Nothing about this is visible on screen: the profile keeps its
		// monogram either way, so the log is the only place that can say the
		// picture was asked for and did not arrive.
		f.log.Debug("avatar: fetch failed", zap.String("key", key), zap.Error(err))
		return "", err
	}
	return path.(string), nil
}

// Invalidate drops a cached avatar. A client calls it when the bytes turn out
// to be undecodable, so the next fetch downloads the file again instead of
// returning the same broken entry forever. It is the only place an avatar is
// removed deliberately: a person who changes their picture simply stops asking
// for the old key, and the bound takes care of the file.
func (f *avatarFetcher) Invalidate(userID, avatarID int64) {
	if f.cache == nil {
		return
	}
	key := avatarCacheKey(userID, avatarID)
	f.log.Debug("avatar: dropped an entry the client could not decode", zap.String("key", key))
	f.cache.Remove(key)
}
