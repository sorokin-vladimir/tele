package ui

import (
	"image"
	"reflect"
	"regexp"
	"strconv"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/stretchr/testify/require"

	"github.com/sorokin-vladimir/tele/internal/domain"
	"github.com/sorokin-vladimir/tele/internal/ui/media"
)

// screenPlay runs a model as the program would, one message at a time, and
// says what the screen showed whenever a Kitty transmit was written. It
// follows the way a frame takes to the terminal and nothing else: the test
// drives the animation ticks itself, and every other tick re-arms forever.
type screenPlay struct {
	m RootModel
	// wrote is told of every transmit written: the image id it went to, and
	// what the screen drew at that moment.
	wrote func(id uint32, screen string)
	// slow makes encoding take that many ticks, as a large frame does: a
	// frame encoded at one tick arrives only after the slow-th next (#331).
	slow int
	held []heldFrame
}

// heldFrame is an encoded frame still on its way, and the ticks it has left.
type heldFrame struct {
	msg   tea.Msg
	ticks int
}

func (p *screenPlay) update(msg tea.Msg) {
	next, cmd := p.m.Update(msg)
	p.m = next.(RootModel)
	p.run(cmd)
}

// deliver hands over the frames whose encoding has taken its ticks, as they
// finish between two ticks.
func (p *screenPlay) deliver() {
	var due []tea.Msg
	keep := p.held[:0]
	for _, h := range p.held {
		if h.ticks--; h.ticks <= 0 {
			due = append(due, h.msg)
		} else {
			keep = append(keep, h)
		}
	}
	p.held = keep
	for _, m := range due {
		p.update(m)
	}
}

func (p *screenPlay) run(cmd tea.Cmd) {
	if cmd == nil {
		return
	}
	msg := cmd()
	// The root model stamps what its commands produce with its epoch.
	inner := msg
	if e, ok := msg.(epochMsg); ok {
		inner = e.msg
	}
	switch inner.(type) {
	case videoFrameEncodedMsg, kittyEncodedMsg:
		if p.slow > 0 {
			p.held = append(p.held, heldFrame{msg: msg, ticks: p.slow})
			return
		}
		p.update(msg)
		return
	case videoFrameWrittenMsg, kittyTransmittedMsg, gifFrameFailedMsg:
		p.update(msg)
		return
	}
	switch msg := msg.(type) {
	case tea.BatchMsg:
		for _, c := range msg {
			p.run(c)
		}
		return
	case tea.RawMsg:
		seq, _ := msg.Msg.(string)
		for _, id := range transmittedIDs(seq) {
			p.wrote(id, p.m.View().Content)
		}
		return
	}
	// tea.Sequence: its commands run in order.
	if v := reflect.ValueOf(msg); msg != nil && v.Kind() == reflect.Slice && v.Type().Elem() == cmdType {
		for i := range v.Len() {
			p.run(v.Index(i).Interface().(tea.Cmd))
		}
	}
}

var (
	kittyControl = regexp.MustCompile(`\x1b_G([^;\x1b]*)`)
	kittyIDKey   = regexp.MustCompile(`(?:^|,)i=(\d+)`)
	// A placeholder cell carries its image id in the 24-bit foreground.
	placeholderID = regexp.MustCompile("\x1b\\[38;2;(\\d+);(\\d+);(\\d+)m\U0010EEEE")
)

// transmittedIDs are the image ids seq transmits to.
func transmittedIDs(seq string) []uint32 {
	var ids []uint32
	for _, m := range kittyControl.FindAllStringSubmatch(seq, -1) {
		ctl := m[1]
		if !regexp.MustCompile(`(?:^|,)a=T(?:,|$)`).MatchString(ctl) {
			continue
		}
		if s := kittyIDKey.FindStringSubmatch(ctl); s != nil {
			id, _ := strconv.ParseUint(s[1], 10, 32)
			ids = append(ids, uint32(id))
		}
	}
	return ids
}

// idsDrawn are the image ids the placeholder cells on screen name.
func idsDrawn(screen string) []uint32 {
	var ids []uint32
	seen := map[uint32]bool{}
	for _, m := range placeholderID.FindAllStringSubmatch(screen, -1) {
		r, _ := strconv.Atoi(m[1])
		g, _ := strconv.Atoi(m[2])
		b, _ := strconv.Atoi(m[3])
		id := uint32(r)<<16 | uint32(g)<<8 | uint32(b)
		if !seen[id] {
			seen[id] = true
			ids = append(ids, id)
		}
	}
	return ids
}

// A frame of an inline GIF is never written under the image id the chat draws
// while it is written, for the same reason as the modal's (#331).
func TestInlineGIF_AFrameIsNeverWrittenUnderTheIDOnScreen(t *testing.T) {
	m := NewRootModel(false)
	m.imageMode = media.ModeKitty
	m.screen = ScreenMain
	m.chat.SetRenderer(media.NewKittyRenderer(m.kittyStore))
	m.chat.SetSize(80, 24)
	const gif = 77
	m.imageCache.Add(gif, image.NewNRGBA(image.Rect(0, 0, 320, 240)))
	m.chat.SetMessages([]domain.Message{{
		ID: 1, ChatID: 1, Date: time.Now(),
		Media:    &domain.MediaRef{Kind: domain.MediaGIF},
		Document: &domain.DocumentRef{ID: gif, MimeType: "video/mp4", ThumbSize: "m"},
	}})
	m.chat.SetKnownImages(m.imageCache)
	writes := 0
	var check func(id uint32, screen string)
	p := &screenPlay{m: m, wrote: func(id uint32, screen string) { check(id, screen) }}
	// The still thumbnail goes out first, as when the chat opens.
	check = func(uint32, string) {}
	p.update(tea.WindowSizeMsg{Width: 80, Height: 24})
	require.NotEmpty(t, idsDrawn(p.m.View().Content), "the chat draws the GIF")

	p.m.gifFrames[gif] = []image.Image{
		image.NewNRGBA(image.Rect(0, 0, 320, 240)),
		image.NewNRGBA(image.Rect(0, 0, 320, 240)),
		image.NewNRGBA(image.Rect(0, 0, 320, 240)),
	}
	p.m.gifActiveID = gif
	p.m.gifGen++
	at := time.Unix(0, 0)
	p.m.now = func() time.Time { return at }
	check = func(id uint32, screen string) {
		writes++
		require.NotContains(t, idsDrawn(screen), id, "frame %d went under the id on screen", writes)
	}
	for range 5 {
		at = at.Add(gifFrameInterval)
		p.update(gifTickMsg{gen: p.m.gifGen})
	}

	require.GreaterOrEqual(t, writes, 4, "every tick writes a frame")
	require.NotEmpty(t, idsDrawn(p.m.View().Content), "the chat still draws the GIF")
}

// An inline GIF whose frames take longer to encode than a tick lasts neither
// blanks nor goes back, as the modal's video does not (#331).
func TestInlineGIF_ASlowEncoderNeitherBlanksNorGoesBack(t *testing.T) {
	m := NewRootModel(false)
	m.imageMode = media.ModeKitty
	m.screen = ScreenMain
	m.chat.SetRenderer(media.NewKittyRenderer(m.kittyStore))
	m.chat.SetSize(80, 24)
	at := time.Unix(0, 0)
	m.now = func() time.Time { return at }
	const gif = 77
	m.imageCache.Add(gif, image.NewNRGBA(image.Rect(0, 0, 320, 240)))
	m.chat.SetMessages([]domain.Message{{
		ID: 1, ChatID: 1, Date: time.Now(),
		Media:    &domain.MediaRef{Kind: domain.MediaGIF},
		Document: &domain.DocumentRef{ID: gif, MimeType: "video/mp4", ThumbSize: "m"},
	}})
	m.chat.SetKnownImages(m.imageCache)
	writes := 0
	var check func(id uint32, screen string)
	p := &screenPlay{m: m, wrote: func(id uint32, screen string) { check(id, screen) }}
	check = func(uint32, string) {}
	p.update(tea.WindowSizeMsg{Width: 80, Height: 24})
	require.NotEmpty(t, idsDrawn(p.m.View().Content), "the chat draws the GIF")

	frames := make([]image.Image, 40)
	for i := range frames {
		frames[i] = image.NewNRGBA(image.Rect(0, 0, 320, 240))
	}
	p.m.gifFrames[gif] = frames
	p.m.gifActiveID = gif
	p.m.gifGen++
	p.slow = 3
	check = func(id uint32, screen string) {
		writes++
		require.NotContains(t, idsDrawn(screen), id, "frame %d went under the id on screen", writes)
	}
	last := -1
	for range 20 {
		at = at.Add(gifFrameInterval - 5*time.Millisecond)
		p.deliver()
		at = at.Add(5 * time.Millisecond)
		p.update(gifTickMsg{gen: p.m.gifGen})
		require.LessOrEqual(t, len(p.held), 1, "one frame is encoded at a time")
		shown, _ := p.m.imageCache.Get(gif)
		for i, f := range frames {
			if f == shown {
				require.GreaterOrEqual(t, i, last, "the GIF went back")
				last = i
			}
		}
	}

	require.GreaterOrEqual(t, writes, 4, "frames still get written")
	require.Greater(t, last, 10, "the GIF moves on")
}

// stillFrames is a frame source of n small frames; made is every frame it
// gave, in order.
type stillFrames struct {
	n    int
	made []image.Image
}

func (s *stillFrames) Next() (image.Image, bool) {
	if s.n == 0 {
		return nil, false
	}
	s.n--
	f := image.NewNRGBA(image.Rect(0, 0, 16, 9))
	s.made = append(s.made, f)
	return f, true
}

// index is where frame stands in the stream; -1 for none.
func (s *stillFrames) index(frame image.Image) int {
	for i, f := range s.made {
		if f == frame {
			return i
		}
	}
	return -1
}

// When a frame takes longer to encode than a tick lasts, as a large one does,
// frames encoded at different ticks are in flight together and arrive late.
// None may then land under the id on screen, what is shown never goes back in
// the stream, and only one frame is encoded at a time: before, a late frame
// overwrote the one on screen, the video blanked and jumped back and forth,
// and encodes piled up on every core (#331).
func TestVideoModal_ASlowEncoderNeitherBlanksNorGoesBack(t *testing.T) {
	m := newRootInternal(nil, 50)
	m.width, m.height = 120, 40
	m.screen = ScreenMain
	at := time.Unix(0, 0)
	m.now = func() time.Time { return at }
	src := &stillFrames{n: 30}
	m.videoPlayer = &videoPlayer{docID: 1, durSecs: 4, cols: 20, rows: 6,
		source: src, playing: true, gen: 1}
	writes := 0
	writtenAt := map[uint32]time.Time{}
	p := &screenPlay{m: m, slow: 3, wrote: func(id uint32, screen string) {
		writes++
		writtenAt[id] = at
		require.NotContains(t, idsDrawn(screen), id, "frame %d went under the id on screen", writes)
	}}

	last := -1
	var drawn uint32
	for range 20 {
		// A frame finishes just before the tick: too late to have been decoded.
		at = at.Add(videoFrameInterval - 5*time.Millisecond)
		p.deliver()
		at = at.Add(5 * time.Millisecond)
		p.update(videoTickMsg{gen: 1})
		require.LessOrEqual(t, len(p.held), 1, "one frame is encoded at a time")
		if ids := idsDrawn(p.m.View().Content); len(ids) == 1 && ids[0] != drawn {
			drawn = ids[0]
			require.GreaterOrEqual(t, at.Sub(writtenAt[drawn]), videoFrameInterval,
				"a frame is shown a frame's time after it is written, for the terminal to decode it")
		}
		if f := p.m.videoPlayer.frame; f != nil {
			shown := src.index(f)
			require.GreaterOrEqual(t, shown, last, "the video went back")
			last = shown
		}
	}

	require.GreaterOrEqual(t, writes, 4, "frames still get written")
	require.Greater(t, last, 10, "the video moves on")
}

func (s *stillFrames) Close() error { return nil }

// brokenFirst is a frame source whose first frame cannot be encoded.
type brokenFirst struct{ stillFrames }

func (s *brokenFirst) Next() (image.Image, bool) {
	if len(s.made) == 0 {
		f := image.NewNRGBA(image.Rect(0, 0, 0, 0))
		s.made = append(s.made, f)
		return f, true
	}
	return s.stillFrames.Next()
}

// A frame that fails to encode does not stop the ones after it: the modal
// waits for one frame at a time, and would otherwise wait forever (#331).
func TestVideoModal_AFrameThatFailsToEncodeDoesNotStopTheVideo(t *testing.T) {
	m := newRootInternal(nil, 50)
	m.width, m.height = 120, 40
	m.screen = ScreenMain
	at := time.Unix(0, 0)
	m.now = func() time.Time { return at }
	m.videoPlayer = &videoPlayer{docID: 1, durSecs: 4, cols: 20, rows: 6,
		source: &brokenFirst{stillFrames{n: 10}}, playing: true, gen: 1}
	writes := 0
	p := &screenPlay{m: m, wrote: func(uint32, string) { writes++ }}

	for range 5 {
		at = at.Add(videoFrameInterval)
		p.update(videoTickMsg{gen: 1})
	}

	require.GreaterOrEqual(t, writes, 2, "the frames after the broken one are written")
}

// The same holds for an inline GIF (#331).
func TestInlineGIF_AFrameThatFailsToEncodeDoesNotStopTheGIF(t *testing.T) {
	m := NewRootModel(false)
	m.imageMode = media.ModeKitty
	m.screen = ScreenMain
	m.chat.SetRenderer(media.NewKittyRenderer(m.kittyStore))
	m.chat.SetSize(80, 24)
	at := time.Unix(0, 0)
	m.now = func() time.Time { return at }
	const gif = 77
	m.imageCache.Add(gif, image.NewNRGBA(image.Rect(0, 0, 320, 240)))
	m.chat.SetMessages([]domain.Message{{
		ID: 1, ChatID: 1, Date: time.Now(),
		Media:    &domain.MediaRef{Kind: domain.MediaGIF},
		Document: &domain.DocumentRef{ID: gif, MimeType: "video/mp4", ThumbSize: "m"},
	}})
	m.chat.SetKnownImages(m.imageCache)
	writes := 0
	p := &screenPlay{m: m, wrote: func(uint32, string) { writes++ }}
	p.update(tea.WindowSizeMsg{Width: 80, Height: 24})
	writes = 0
	p.m.gifFrames[gif] = []image.Image{
		image.NewNRGBA(image.Rect(0, 0, 320, 240)),
		image.NewNRGBA(image.Rect(0, 0, 0, 0)), // the first one ticked to
		image.NewNRGBA(image.Rect(0, 0, 320, 240)),
	}
	p.m.gifActiveID = gif
	p.m.gifGen++

	for range 5 {
		at = at.Add(gifFrameInterval)
		p.update(gifTickMsg{gen: p.m.gifGen})
	}

	require.GreaterOrEqual(t, writes, 2, "the frames after the broken one are written")
}

// A frame of the video modal is never written under the image id the modal
// draws while it is written. Ghostty drops the image under an id the moment
// new data for it arrives, and draws nothing there until that data is
// decoded, so the modal went blank for a moment at every frame (#331).
func TestVideoModal_AFrameIsNeverWrittenUnderTheIDOnScreen(t *testing.T) {
	m := newRootInternal(nil, 50)
	m.width, m.height = 120, 40
	m.screen = ScreenMain
	at := time.Unix(0, 0)
	m.now = func() time.Time { return at }
	m.videoPlayer = &videoPlayer{docID: 1, durSecs: 4, cols: 20, rows: 6,
		source: &stillFrames{n: 10}, playing: true, gen: 1}
	writes := 0
	p := &screenPlay{m: m, wrote: func(id uint32, screen string) {
		writes++
		require.NotContains(t, idsDrawn(screen), id, "frame %d went under the id on screen", writes)
	}}

	for range 5 {
		at = at.Add(videoFrameInterval)
		p.update(videoTickMsg{gen: 1})
	}

	require.GreaterOrEqual(t, writes, 4, "every tick writes a frame")
	require.NotEmpty(t, idsDrawn(p.m.View().Content), "the modal draws a frame")
}
