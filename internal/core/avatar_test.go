package core

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sorokin-vladimir/tele/internal/domain"
	"github.com/sorokin-vladimir/tele/internal/mediacache"
	"github.com/sorokin-vladimir/tele/internal/telerr"
	internaltg "github.com/sorokin-vladimir/tele/internal/tg"
)

// avatarStub counts avatar downloads and records the address each one was made
// with, which is how a test can tell that addressing stayed with the owner.
type avatarStub struct {
	stubClient

	mu      sync.Mutex
	calls   int
	lastID  int64
	addrs   []internaltg.UserAddress
	payload string
	delay   time.Duration
}

func (s *avatarStub) DownloadUserAvatarToFile(_ context.Context, addr internaltg.UserAddress, avatarID int64, dst io.Writer) error {
	s.mu.Lock()
	s.calls++
	s.lastID = avatarID
	s.addrs = append(s.addrs, addr)
	payload, delay := s.payload, s.delay
	s.mu.Unlock()
	if delay > 0 {
		time.Sleep(delay)
	}
	_, err := dst.Write([]byte(payload))
	return err
}

func (s *avatarStub) downloads() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.calls
}

// newAvatarOwner builds an owner with a real avatar cache in a temp dir and one
// dialog, so the person has an access hash to be addressed by.
func newAvatarOwner(t *testing.T, c *avatarStub) (*Owner, string) {
	t.Helper()
	o, s := newOwnerWithClient(t, c)
	dir := t.TempDir()
	cache, err := mediacache.New(dir, 1<<20)
	require.NoError(t, err)
	o.SetAvatarCache(cache)
	s.Store().SetChat(domain.Chat{
		ID: 1, Title: "Ada", Peer: domain.Peer{ID: 1, Type: domain.PeerUser, AccessHash: 77},
	})
	return o, dir
}

func TestFetchAvatar_StreamsIntoItsOwnCache(t *testing.T) {
	c := &avatarStub{payload: "face"}
	o, dir := newAvatarOwner(t, c)

	path, err := o.FetchAvatar(context.Background(), 1, 42)
	require.NoError(t, err)

	body, rerr := os.ReadFile(path)
	require.NoError(t, rerr)
	assert.Equal(t, "face", string(body))
	assert.Equal(t, filepath.Join(dir, "avatar_1_42"), path,
		"the key names both the person and the picture")
	assert.Equal(t, int64(42), c.lastID)
	require.Len(t, c.addrs, 1)
	assert.Equal(t, int64(77), c.addrs[0].AccessHash,
		"the owner resolves the address; the client passed an id alone")
}

func TestFetchAvatar_SecondFetchIsServedFromTheCache(t *testing.T) {
	c := &avatarStub{payload: "face"}
	o, _ := newAvatarOwner(t, c)

	first, err := o.FetchAvatar(context.Background(), 1, 42)
	require.NoError(t, err)
	second, err := o.FetchAvatar(context.Background(), 1, 42)
	require.NoError(t, err)

	assert.Equal(t, first, second)
	assert.Equal(t, 1, c.downloads(), "a hit must not go to the network")
}

// A changed picture is a changed id, and that alone is what keeps a stale
// avatar from being served forever: nothing subscribes to anything.
func TestFetchAvatar_ANewPictureIsANewEntry(t *testing.T) {
	c := &avatarStub{payload: "face"}
	o, dir := newAvatarOwner(t, c)

	old, err := o.FetchAvatar(context.Background(), 1, 42)
	require.NoError(t, err)
	fresh, err := o.FetchAvatar(context.Background(), 1, 43)
	require.NoError(t, err)

	assert.NotEqual(t, old, fresh)
	assert.Equal(t, 2, c.downloads())
	assert.Equal(t, filepath.Join(dir, "avatar_1_43"), fresh)
	assert.FileExists(t, old, "the old file is left to the bound rather than deleted")
}

func TestFetchAvatar_ConcurrentFetchesDownloadOnce(t *testing.T) {
	c := &avatarStub{payload: "face", delay: 20 * time.Millisecond}
	o, _ := newAvatarOwner(t, c)

	var wg sync.WaitGroup
	paths := make([]string, 8)
	errs := make([]error, 8)
	for i := range paths {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			paths[i], errs[i] = o.FetchAvatar(context.Background(), 1, 42)
		}(i)
	}
	wg.Wait()

	for i := range paths {
		require.NoError(t, errs[i])
		assert.Equal(t, paths[0], paths[i])
	}
	assert.Equal(t, 1, c.downloads(), "every repaint must not start its own download")
}

// A person with no avatar is a whole answer rather than a failed download: the
// client draws a monogram and nothing is asked of Telegram.
func TestFetchAvatar_NoAvatarIsNotADownload(t *testing.T) {
	c := &avatarStub{payload: "face"}
	o, _ := newAvatarOwner(t, c)

	_, err := o.FetchAvatar(context.Background(), 1, 0)

	require.Error(t, err)
	assert.Equal(t, telerr.NotFound, telerr.Of(err))
	assert.Equal(t, 0, c.downloads())
}

func TestFetchAvatar_WithoutACacheItFailsRatherThanInventingALocation(t *testing.T) {
	c := &avatarStub{payload: "face"}
	o, s := newOwnerWithClient(t, c)
	s.Store().SetChat(domain.Chat{
		ID: 1, Title: "Ada", Peer: domain.Peer{ID: 1, Type: domain.PeerUser, AccessHash: 77},
	})

	_, err := o.FetchAvatar(context.Background(), 1, 42)

	require.Error(t, err)
	assert.Equal(t, telerr.Internal, telerr.Of(err))
	assert.Equal(t, 0, c.downloads())
}

func TestInvalidateAvatar_DropsTheEntrySoTheNextFetchDownloadsAgain(t *testing.T) {
	c := &avatarStub{payload: "face"}
	o, _ := newAvatarOwner(t, c)

	path, err := o.FetchAvatar(context.Background(), 1, 42)
	require.NoError(t, err)
	o.InvalidateAvatar(1, 42)
	assert.NoFileExists(t, path)

	_, err = o.FetchAvatar(context.Background(), 1, 42)
	require.NoError(t, err)
	assert.Equal(t, 2, c.downloads())
}

// A person met only in a group has no access hash anywhere on the account, and
// is addressed through a message they wrote — the same fallback GetUser uses.
func TestFetchAvatar_AddressesSomeoneWithNoDialogThroughAMessage(t *testing.T) {
	c := &avatarStub{payload: "face"}
	o, s := newOwnerWithClient(t, c)
	dir := t.TempDir()
	cache, err := mediacache.New(dir, 1<<20)
	require.NoError(t, err)
	o.SetAvatarCache(cache)
	st := s.Store()
	st.SetChat(domain.Chat{ID: 50, Title: "Group", Peer: domain.Peer{ID: 50, Type: domain.PeerGroup}})
	st.SetMessages(50, []domain.Message{{ID: 7, ChatID: 50, SenderID: 9, Date: time.Unix(1, 0)}})

	_, err = o.FetchAvatar(context.Background(), 9, 42)

	require.NoError(t, err)
	require.Len(t, c.addrs, 1)
	assert.Zero(t, c.addrs[0].AccessHash)
	assert.Equal(t, 7, c.addrs[0].FromMsgID)
	assert.Equal(t, int64(50), c.addrs[0].FromChat.ID)
}
