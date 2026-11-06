package media_test

import (
	"bytes"
	"encoding/base64"
	"image"
	"image/color"
	"image/png"
	"math/rand/v2"
	"regexp"
	"strings"
	"testing"
	"unicode"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi/kitty"
	"github.com/sorokin-vladimir/tele/internal/ui/media"
	"github.com/stretchr/testify/require"
)

func TestPlaceholderLines_Shape(t *testing.T) {
	lines := media.PlaceholderLines(7, 5, 3)
	if len(lines) != 3 {
		t.Fatalf("want 3 rows, got %d", len(lines))
	}
}

func TestKittyStore_IDForStableAndMonotonic(t *testing.T) {
	s := media.NewKittyStore()
	a := s.IDFor(100)
	b := s.IDFor(200)
	require.Equal(t, a, s.IDFor(100), "same photo keeps its id")
	require.NotEqual(t, a, b, "different photos get different ids")
	require.Greater(t, a, uint32(0), "ids are positive")
}

// TestKittyStore_IDsStartSomewhereRandom guards against two runs handing the
// same terminal the same image ids. A terminal keeps the placements of a
// process that has exited, and iTerm2 resolves a placeholder to the first
// placement carrying that id, so a fresh run numbering from 1 draws the
// previous run's photos (#259).
func TestKittyStore_IDsStartSomewhereRandom(t *testing.T) {
	const stores = 8

	first := make(map[uint32]int, stores)
	for range stores {
		id := media.NewKittyStore().IDFor(100)
		require.LessOrEqual(t, id, uint32(0xFFFFFF), "ids fit the placeholder's 24-bit encoding")
		require.Greater(t, id, uint32(0), "ids are positive")
		first[id]++
	}
	require.Greater(t, len(first), 1, "stores do not all start at the same id")
}

// An animated image takes its next frame under a second id, not the one on
// screen: a terminal such as Ghostty drops the image under an id the moment new
// data for it arrives, and draws nothing there until the new data is decoded
// (#331). Swapping puts the second id on screen and frees the first for the
// frame after.
func TestKittyStore_ABackIDTakesTheNextFrame(t *testing.T) {
	s := media.NewKittyStore()
	shown := s.IDFor(100)
	back := s.BackIDFor(100)
	require.NotEqual(t, shown, back, "the next frame never goes to the id on screen")
	require.Equal(t, back, s.BackIDFor(100), "the back id is stable until a swap")
	require.NotEqual(t, back, s.IDFor(200), "no other photo is given it")
	s.MarkBackTransmitted(100, 8)

	s.Swap(100)

	require.Equal(t, back, s.IDFor(100), "the swap puts the back id on screen")
	require.Equal(t, shown, s.BackIDFor(100), "and frees the old one for the next frame")
}

// A frame written under the back id is what a swap puts on screen, at the
// width it was written at; the back id then waits for the next frame (#331).
func TestKittyStore_ASwapShowsTheFrameWrittenBehind(t *testing.T) {
	s := media.NewKittyStore()
	s.MarkTransmitted(100, 30)
	require.False(t, s.BackReady(100), "nothing written behind yet")

	s.MarkBackTransmitted(100, 40)
	require.True(t, s.BackReady(100))
	require.True(t, s.Ready(100, 30), "the frame on screen stays until the swap")

	s.Swap(100)

	require.True(t, s.Ready(100, 40), "the swap shows the frame written behind")
	require.False(t, s.Ready(100, 30))
	require.False(t, s.BackReady(100), "the back id waits for the next frame")
}

// With nothing written behind there is nothing to show: a swap then would put
// an empty id on screen (#331).
func TestKittyStore_ASwapWithNothingBehindChangesNothing(t *testing.T) {
	s := media.NewKittyStore()
	shown := s.IDFor(100)
	s.MarkTransmitted(100, 30)

	s.Swap(100)

	require.Equal(t, shown, s.IDFor(100))
	require.True(t, s.Ready(100, 30))
}

// What is drawn after a swap names the id swapped in, though the photo and
// its width are the same as before (#331).
func TestKittyRenderer_DrawsTheIDSwappedIn(t *testing.T) {
	s := media.NewKittyStore()
	r := media.NewKittyRenderer(s)
	img := sampleImage(16, 16)
	s.MarkTransmitted(100, 8)
	before := strings.Join(r.Render(100, img, 8), "\n")
	s.MarkBackTransmitted(100, 8)

	s.Swap(100)

	after := strings.Join(r.Render(100, img, 8), "\n")
	require.Equal(t, strings.Join(media.PlaceholderLines(s.IDFor(100), 8, len(r.Render(100, img, 8))), "\n"), after)
	require.NotEqual(t, before, after)
}

// Deleting a photo deletes both of its ids, and clearing forgets a frame
// written behind as it forgets the one on screen (#331).
func TestKittyStore_BothIDsGoTogether(t *testing.T) {
	s := media.NewKittyStore()
	shown, back := s.IDFor(100), s.BackIDFor(100)
	seq := s.DeleteLiveSeq([]int64{100})
	require.Contains(t, seq, media.DeleteSeq(shown))
	require.Contains(t, seq, media.DeleteSeq(back))

	s.MarkBackTransmitted(100, 8)
	s.Clear()
	require.False(t, s.BackReady(100))

	s.MarkBackTransmitted(100, 8)
	s.Untransmit(100)
	require.False(t, s.BackReady(100))
}

func TestKittyStore_ReadyTracksTransmission(t *testing.T) {
	s := media.NewKittyStore()
	require.False(t, s.Ready(100, 30))
	s.MarkTransmitted(100, 30)
	require.True(t, s.Ready(100, 30))
	require.False(t, s.Ready(100, 40), "different cols is not ready")
}

func TestKittyStore_ClearResetsTransmissionButKeepsIDs(t *testing.T) {
	s := media.NewKittyStore()
	id := s.IDFor(100)
	s.MarkTransmitted(100, 30)
	s.Clear()
	require.False(t, s.Ready(100, 30), "clear marks images untransmitted")
	require.Equal(t, id, s.IDFor(100), "ids remain stable across clear")
}

func TestKittyStore_DeleteSeqIsKittyAPC(t *testing.T) {
	seq := media.DeleteSeq(7)
	require.True(t, strings.HasPrefix(seq, "\x1b_G"), "starts with Kitty APC")
	require.Contains(t, seq, "a=d")
	require.Contains(t, seq, "d=I") // delete by id and free data (unambiguous for virtual placements)
	require.True(t, strings.HasSuffix(seq, "\x1b\\"), "ends with ST")
}

func TestKittyStore_TransmitSeqContainsIDAndPlacement(t *testing.T) {
	s := media.NewKittyStore()
	img := sampleImage(16, 16)
	id := s.IDFor(100)
	seq, err := media.TransmitSeq(id, img, 8, 4)
	require.NoError(t, err)
	require.True(t, strings.HasPrefix(seq, "\x1b_G"))
	require.Contains(t, seq, "a=T") // transmit and put
	require.Contains(t, seq, "U=1") // virtual placement
	require.Contains(t, seq, "c=8") // columns
	require.Contains(t, seq, "r=4") // rows
}

// TestKittyStore_TransmitSeqIsDecodable guards the requirement that the
// transmitted image carries its own dimensions: either a self-describing
// format (PNG, f=100) or explicit s= and v= keys. Raw RGBA without s/v is
// undecodable by the terminal and renders nothing.
func TestKittyStore_TransmitSeqIsDecodable(t *testing.T) {
	s := media.NewKittyStore()
	img := sampleImage(16, 16)
	seq, err := media.TransmitSeq(s.IDFor(100), img, 8, 4)
	require.NoError(t, err)
	selfDescribing := strings.Contains(seq, "f=100")
	hasDims := strings.Contains(seq, "s=") && strings.Contains(seq, "v=")
	require.True(t, selfDescribing || hasDims,
		"transmit must be PNG (f=100) or carry s=/v= dimensions")
}

// A still photo goes out exactly as the kitty package writes it: the faster
// encoding is for animation frames only (#331).
func TestKittyStore_TransmitSeqIsWhatTheKittyPackageWrites(t *testing.T) {
	img := sampleImage(200, 150) // several chunks of data, sent unscaled at 40 columns
	seq, err := media.TransmitSeq(7, img, 40, 20)
	require.NoError(t, err)

	var want strings.Builder
	require.NoError(t, kitty.EncodeGraphics(&want, img, &kitty.Options{
		Action: kitty.TransmitAndPut, ID: 7, Format: kitty.PNG, Transmission: kitty.Direct,
		Chunk: true, VirtualPlacement: true, Columns: 40, Rows: 20, Quite: 2,
	}))
	require.Equal(t, want.String(), seq)
}

// An animation frame is encoded for speed rather than size: at the default
// level a large frame took longer to encode than a tick lasts (#331). It is
// still the same picture, transmitted and placed like any other.
func TestKittyStore_AFrameSeqCarriesThePicture(t *testing.T) {
	// Noise, so the picture does not compress into a single chunk.
	img := image.NewRGBA(image.Rect(0, 0, 200, 150))
	r := rand.New(rand.NewPCG(1, 2))
	for i := range img.Pix {
		img.Pix[i] = uint8(r.UintN(256))
		if i%4 == 3 {
			img.Pix[i] = 255
		}
	}
	seq, err := media.TransmitFrameSeq(7, img, 40, 20)
	require.NoError(t, err)

	chunks := regexp.MustCompile("\x1b_G([^;]*);([^\x1b]*)\x1b\\\\").FindAllStringSubmatch(seq, -1)
	require.Greater(t, len(chunks), 1, "the frame goes in chunks")
	first := chunks[0][1]
	for _, key := range []string{"a=T", "i=7", "f=100", "U=1", "c=40", "r=20", "q=2", "m=1"} {
		require.Contains(t, strings.Split(first, ","), key)
	}
	var payload strings.Builder
	for i, c := range chunks {
		payload.WriteString(c[2])
		if i > 0 {
			want := "q=2,m=1"
			if i == len(chunks)-1 {
				want = "q=2,m=0"
			}
			require.Equal(t, want, c[1], "chunk %d", i)
		}
	}
	data, err := base64.StdEncoding.DecodeString(payload.String())
	require.NoError(t, err)
	got, err := png.Decode(bytes.NewReader(data))
	require.NoError(t, err)
	require.Equal(t, img.Bounds(), got.Bounds())
	require.Equal(t, img.At(120, 90), color.RGBAModel.Convert(got.At(120, 90)))
}

func TestKittyRenderer_NilUntilTransmitted(t *testing.T) {
	s := media.NewKittyStore()
	r := media.NewKittyRenderer(s)
	img := sampleImage(16, 16)
	require.Nil(t, r.Render(100, img, 8), "no output before transmission")
	s.MarkTransmitted(100, 8)
	require.NotNil(t, r.Render(100, img, 8), "output once transmitted")
}

func TestKittyRenderer_FootprintMatchesPhotoRows(t *testing.T) {
	s := media.NewKittyStore()
	r := media.NewKittyRenderer(s)
	img := sampleImage(20, 30)
	cols := 8
	s.MarkTransmitted(100, cols)
	lines := r.Render(100, img, cols)
	b := img.Bounds()
	require.Len(t, lines, media.PhotoRows(b.Dx(), b.Dy(), cols, media.CellAspect()))
}

func TestKittyRenderer_LineWidthEqualsCols(t *testing.T) {
	s := media.NewKittyStore()
	r := media.NewKittyRenderer(s)
	img := sampleImage(20, 20)
	cols := 10
	s.MarkTransmitted(100, cols)
	for _, l := range r.Render(100, img, cols) {
		require.Equal(t, cols, lipgloss.Width(l), "each line measures cols cells")
	}
}

func TestKittyRenderer_CellsCarryPlaceholderAndDiacritics(t *testing.T) {
	s := media.NewKittyStore()
	r := media.NewKittyRenderer(s)
	img := sampleImage(8, 8)
	cols := 4
	s.MarkTransmitted(100, cols)
	first := r.Render(100, img, cols)[0]
	require.Contains(t, first, string(kitty.Placeholder))
	require.Contains(t, first, string(kitty.Diacritic(0)))      // row 0
	require.Contains(t, first, string(kitty.Diacritic(cols-1))) // last column
	require.True(t, strings.HasSuffix(first, "\x1b[0m"), "line resets SGR")
}

// TestPlaceholderWindow_CellsCarryImageIDMSB pins the third diacritic. The
// Kitty spec makes it optional while the image id fits in 24 bits, but iTerm2
// reads an absent one as -1 and shifts it into the id as 0xff000000, so the
// placement is never found and the image does not draw (#259).
func TestPlaceholderWindow_CellsCarryImageIDMSB(t *testing.T) {
	lines := media.PlaceholderWindow(7, 0, 0, 1, 1)
	require.Len(t, lines, 1)

	var marks []rune
	for _, r := range lines[0] {
		if unicode.Is(unicode.Mn, r) {
			marks = append(marks, r)
		}
	}
	require.Equal(t,
		[]rune{kitty.Diacritic(0), kitty.Diacritic(0), kitty.Diacritic(0)},
		marks, "cell carries row, column and image-id-msb diacritics")
}

// TestPlaceholderWindow_MSBDiacriticFollowsID guards the encoding for ids that
// do not fit in 24 bits, so a wrapped id still names its own placement.
func TestPlaceholderWindow_MSBDiacriticFollowsID(t *testing.T) {
	const id = 0x03_00_00_05

	lines := media.PlaceholderWindow(id, 0, 0, 1, 1)
	require.Len(t, lines, 1)
	require.Contains(t, lines[0], string(kitty.Diacritic(3)), "third diacritic spells the id's top byte")
}

func TestKittyStore_DeleteLiveSeq_PerID(t *testing.T) {
	s := media.NewKittyStore()
	id1 := s.IDFor(11)
	id2 := s.IDFor(22)

	seq := s.DeleteLiveSeq([]int64{11, 22})

	require.Equal(t, media.DeleteSeq(id1)+media.DeleteSeq(id2), seq)
	require.NotEmpty(t, seq)
}

func TestKittyStore_DeleteLiveSeq_SkipsUnassigned(t *testing.T) {
	s := media.NewKittyStore()
	// 99 was never assigned an image id, so it contributes nothing.
	require.Empty(t, s.DeleteLiveSeq([]int64{99}))
}

func TestPlaceholderWindowDimensions(t *testing.T) {
	lines := media.PlaceholderWindow(7, 3, 2, 5, 4) // hOff=3 vOff=2 winCols=5 winRows=4
	if len(lines) != 4 {
		t.Fatalf("lines = %d, want winRows 4", len(lines))
	}
	for i, ln := range lines {
		n := 0
		for _, r := range ln {
			if r == kitty.Placeholder {
				n++
			}
		}
		if n != 5 {
			t.Fatalf("line %d has %d placeholder cells, want winCols 5", i, n)
		}
	}
}
