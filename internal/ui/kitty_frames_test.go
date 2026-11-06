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
}

func (p *screenPlay) update(msg tea.Msg) {
	next, cmd := p.m.Update(msg)
	p.m = next.(RootModel)
	p.run(cmd)
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
	case videoFrameEncodedMsg, videoFrameWrittenMsg, kittyEncodedMsg, kittyTransmittedMsg:
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
	check = func(id uint32, screen string) {
		writes++
		require.NotContains(t, idsDrawn(screen), id, "frame %d went under the id on screen", writes)
	}
	for range 5 {
		p.update(gifTickMsg{gen: p.m.gifGen})
	}

	require.GreaterOrEqual(t, writes, 4, "every tick writes a frame")
	require.NotEmpty(t, idsDrawn(p.m.View().Content), "the chat still draws the GIF")
}

// stillFrames is a frame source of n small frames.
type stillFrames struct{ n int }

func (s *stillFrames) Next() (image.Image, bool) {
	if s.n == 0 {
		return nil, false
	}
	s.n--
	return image.NewNRGBA(image.Rect(0, 0, 16, 9)), true
}

func (s *stillFrames) Close() error { return nil }

// A frame of the video modal is never written under the image id the modal
// draws while it is written. Ghostty drops the image under an id the moment
// new data for it arrives, and draws nothing there until that data is
// decoded, so the modal went blank for a moment at every frame (#331).
func TestVideoModal_AFrameIsNeverWrittenUnderTheIDOnScreen(t *testing.T) {
	m := newRootInternal(nil, 50)
	m.width, m.height = 120, 40
	m.screen = ScreenMain
	m.videoPlayer = &videoPlayer{docID: 1, durSecs: 4, cols: 20, rows: 6,
		source: &stillFrames{n: 10}, playing: true, gen: 1}
	writes := 0
	p := &screenPlay{m: m, wrote: func(id uint32, screen string) {
		writes++
		require.NotContains(t, idsDrawn(screen), id, "frame %d went under the id on screen", writes)
	}}

	for range 5 {
		p.update(videoTickMsg{gen: 1})
	}

	require.GreaterOrEqual(t, writes, 4, "every tick writes a frame")
	require.NotEmpty(t, idsDrawn(p.m.View().Content), "the modal draws a frame")
}
