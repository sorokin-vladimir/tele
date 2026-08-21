package components

import (
	"image"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/sorokin-vladimir/tele/internal/domain"
	"github.com/sorokin-vladimir/tele/internal/ui/keys"
	"github.com/sorokin-vladimir/tele/internal/ui/media"
	"github.com/sorokin-vladimir/tele/internal/ui/theme"
)

// Profile overlay requests. The root model turns each into an owner call.
// Every one of them names the person by id: a client holds no access hash for
// someone met in a group, so addressing is the owner's (ADR 0006).
type (
	// OpenProfileRequest asks for the overlay. It is what every entry point
	// emits, which is why the overlay itself has no idea where it came from.
	OpenProfileRequest struct{ UserID int64 }
	// ProfileOpenChatRequest opens the private chat with the person. It works
	// whether or not a dialog exists: with none, the chat opens empty and ready
	// to take a first message.
	ProfileOpenChatRequest struct{ UserID int64 }
	// ProfileMuteRequest mutes or unmutes the dialog with the person. Only ever
	// emitted when a dialog exists.
	ProfileMuteRequest struct {
		UserID int64
		Muted  bool
	}
	// ProfileCopyUsernameRequest copies the handle, with its leading '@'.
	ProfileCopyUsernameRequest struct{ Username string }
	CloseProfileMsg            struct{}
)

// ProfileMinWidth is the narrowest terminal the overlay will open on. Below it
// the frame is already too tight (#216) and a box that cannot hold a name is
// worse than a message saying so.
const ProfileMinWidth = 30

// profileHMargin and profileVMargin are how many cells and rows the overlay
// leaves free around it, so it reads as a surface over the app rather than as a
// replacement for it.
const (
	profileHMargin = 4
	profileVMargin = 2
)

// profilePadV and profilePadH hold the content off the frame. A surface pads
// what it carries; a box with text against its border reads as a box someone
// poured text into. These are the startup notice's numbers, which is the convention
// the app already has (#236).
const (
	profilePadV = 1
	profilePadH = 2
)

// avatarLargeCols and avatarSmallCols are the two widths the avatar is drawn
// at, and avatarGap the air between it and the name. The profile is the one
// place in the app whose subject is the person, so the large size is the
// default and the small one is what a viewport that cannot afford it falls back
// to (#236). One cell of gap read as a single run with the name; two separate
// the picture from the text, monogram included.
const (
	avatarLargeCols = 16
	avatarSmallCols = 8
	avatarGap       = 2
)

// avatarLadder is what the overlay walks when it decides how big a picture to
// draw: the large size, then the small one, then none at all. The last rung is
// no avatar rather than no overlay — refusing to open a profile that opens
// today would be a strange way to add a feature (#223).
var avatarLadder = []int{avatarLargeCols, avatarSmallCols, 0}

// ProfileAvatarImageKey is the image id the profile's avatar is transmitted
// under, in the same sentinel space as the photo and video modals' keys (-1001
// and -1000). One stable key rather than one per person: only ever one profile
// is open, and the placement is deleted when it closes, so the next person
// cannot inherit this one's geometry (#175).
const ProfileAvatarImageKey int64 = -1002

// Profile is the user profile overlay: a person and what can be done about
// them. It is a snapshot — what is known when it opens, completed once by the
// full-user answer — and never subscribes to anything, because a profile is
// opened, read and closed (#222).
type Profile struct {
	user domain.User
	// full is true once the complete answer has arrived. Until then an empty
	// Bio means "not known yet" rather than "not set", which is the whole
	// difference between a partial profile and a finished one.
	full bool
	// hasDialog reports whether the client holds a chat with this person.
	// Muting is a fact about a dialog, so with no dialog there is nothing to
	// mute and the item is absent rather than guessed at.
	hasDialog bool
	muted     bool

	// avatar is the person's picture, nil until (and unless) one arrives. The
	// overlay draws a monogram meanwhile, and the two occupy the same cells, so
	// a picture landing replaces the letters without moving anything (#223).
	avatar image.Image
	// renderer is set only where the terminal can draw an image, which is what
	// makes it the test for "picture or monogram here". Block art is
	// deliberately not an option: at eight cells it is a smear, and letters say
	// more.
	renderer media.Renderer

	// actions is what can be done about this person, in the order it is
	// offered. It is a list of actions rather than of rows: the overlay names
	// them on its bottom border, and the words come from the same place every
	// other hint's words do (#236).
	actions []keys.Action
	keyMap  keys.KeyMap
	width   int
	height  int
}

// NewProfile builds the overlay around what is known now. hasDialog and muted
// describe the client's dialog with the person, which the overlay reads but
// does not own.
func NewProfile(user domain.User, hasDialog, muted bool, km keys.KeyMap, width, height int) *Profile {
	p := &Profile{
		user:      user,
		hasDialog: hasDialog,
		muted:     muted,
		keyMap:    km,
		width:     width,
		height:    height,
	}
	p.rebuild()
	return p
}

// UserID is the person the overlay is about.
func (p *Profile) UserID() int64 { return p.user.ID }

// SetUser replaces the person with the completed answer.
func (p *Profile) SetUser(u domain.User) {
	p.user = u
	p.full = true
	p.rebuild()
}

// SetSize records the terminal size, so a resize while the overlay is open
// re-wraps the bio instead of stamping a stale width.
func (p *Profile) SetSize(w, h int) { p.width, p.height = w, h }

// SetRenderer gives the overlay the image renderer, which is the same seam the
// message list is handed. A nil renderer (a terminal without Kitty graphics) is
// the ordinary case, not a degraded one: the monogram is a whole answer.
func (p *Profile) SetRenderer(r media.Renderer) { p.renderer = r }

// SetAvatar hands over the person's decoded picture.
func (p *Profile) SetAvatar(img image.Image) { p.avatar = img }

// Avatar is the picture the overlay is showing, nil when it is drawing a
// monogram. The caller needs it to re-transmit the image when the terminal's
// placements are reset under it.
func (p *Profile) Avatar() image.Image { return p.avatar }

// HasAvatar reports whether the overlay holds a picture, for tests.
func (p *Profile) HasAvatar() bool { return p.avatar != nil }

// AvatarBox is the cell box the avatar occupies in the viewport the overlay was
// last sized to, and 0, 0 when none of the ladder's sizes fit. The caller
// transmits at exactly this size.
//
// The box is square on screen, which is why the rows come from the terminal's
// real cell aspect rather than from a constant.
func (p *Profile) AvatarBox() (cols, rows int) {
	return avatarBoxFor(p.avatarCols())
}

func avatarBoxFor(cols int) (int, int) {
	if cols <= 0 {
		return 0, 0
	}
	return cols, media.PhotoRows(1, 1, cols, media.CellAspect())
}

// avatarCols is the width the avatar is drawn at here and now: the first rung
// of the ladder this viewport can hold.
//
// A rung is tried by building the overlay at it and measuring, rather than by
// comparing against a number written down beside the layout. A taller picture
// makes a taller overlay, and how much taller depends on the person — their bio
// length, whether they have a phone, how many actions apply — so the only
// honest answer to "does this fit" is the one the layout itself gives.
func (p *Profile) avatarCols() int {
	for _, cols := range avatarLadder {
		if p.avatarFits(cols) {
			return cols
		}
	}
	return 0
}

// avatarFits reports whether the overlay can carry a picture this wide, in both
// directions. Both answers come from the layout built at this rung rather than
// from a rule written down beside it: the width the content asks for with this
// picture in it, and the rows the overlay takes with it.
//
// Width is a question about squeezing rather than about the terminal. The
// overlay grows to hold the picture beside the name, so the picture only costs
// something once that growth runs into the terminal — and then it costs the
// name the room it is read in, which is the wrong trade at any size.
func (p *Profile) avatarFits(cols int) bool {
	if cols <= 0 {
		return true // no block always fits: the overlay outlives the avatar
	}
	if p.contentWidth(cols) > p.maxInnerWidth() {
		return false
	}
	return p.overlayHeight(cols) <= p.height-profileVMargin
}

// overlayHeight is how many rows the whole overlay takes with an avatar this
// wide: its content, the padding above and below it, and the frame.
func (p *Profile) overlayHeight(cols int) int {
	return len(p.contentLines(p.innerWidth(cols), cols)) + 2*profilePadV + 2
}

// avatarBlock renders the cols×rows block standing at the head of the overlay:
// the picture where one can be drawn, and the monogram in every other case —
// no renderer, no picture yet, or no picture at all.
//
// A renderer that answers with the wrong number of lines is treated as no
// answer: the block's height is what the layout is built on, and a monogram of
// the right size beats an image of the wrong one.
func (p *Profile) avatarBlock(cols, rows int) []string {
	if p.avatar != nil && p.renderer != nil {
		if lines := p.renderer.Render(ProfileAvatarImageKey, p.avatar, cols); len(lines) == rows {
			return lines
		}
	}
	return monogramLines(p.user, cols, rows)
}

// rebuild recomputes the action list from the current user. An action that
// cannot apply is absent rather than disabled: an item that refuses to do
// anything is a worse answer than no item.
func (p *Profile) rebuild() {
	actions := []keys.Action{keys.ActionOpenChat}
	if p.hasDialog {
		if p.muted {
			actions = append(actions, keys.ActionUnmute)
		} else {
			actions = append(actions, keys.ActionMute)
		}
	}
	if p.user.Username != "" {
		actions = append(actions, keys.ActionCopyUsername)
	}
	p.actions = append(actions, keys.ActionCancel)
}

func (p *Profile) Update(msg tea.Msg) (*Profile, tea.Cmd) {
	kp, ok := msg.(tea.KeyPressMsg)
	if !ok {
		return p, nil
	}
	action := p.keyMap.Resolve(keys.ContextProfile, kp.String())
	if action == keys.ActionCancel {
		return nil, func() tea.Msg { return CloseProfileMsg{} }
	}
	// A letter bound to an action the profile is currently offering fires it.
	// There is no cursor to move first: the overlay names its actions on its
	// bottom border and each one is its own key (#236). Mute and unmute share a
	// key, so the offer decides which of the two the press means.
	if action != keys.ActionNone {
		for _, a := range p.actions {
			if a == action || (action == keys.ActionMute && a == keys.ActionUnmute) {
				return p.execute(a)
			}
		}
	}
	return p, nil
}

// execute runs one action. Everything but muting closes the overlay: muting is
// a toggle you may want to see land, while the rest take you elsewhere.
func (p *Profile) execute(action keys.Action) (*Profile, tea.Cmd) {
	switch action {
	case keys.ActionOpenChat:
		userID := p.user.ID
		return nil, func() tea.Msg { return ProfileOpenChatRequest{UserID: userID} }
	case keys.ActionMute, keys.ActionUnmute:
		muted := action == keys.ActionMute
		userID := p.user.ID
		// The overlay is a snapshot, so it flips its own row rather than
		// waiting for a delta it does not listen to.
		p.muted = muted
		p.rebuild()
		return p, func() tea.Msg { return ProfileMuteRequest{UserID: userID, Muted: muted} }
	case keys.ActionCopyUsername:
		handle := "@" + p.user.Username
		return nil, func() tea.Msg { return ProfileCopyUsernameRequest{Username: handle} }
	case keys.ActionCancel:
		return nil, func() tea.Msg { return CloseProfileMsg{} }
	}
	return p, nil
}

// infoLines renders the identity block: the name, the handle, presence, the bio
// and the phone, in that order, with blank lines only between groups that are
// actually there.
func (p *Profile) infoLines(innerW, avatarW int) []string {
	bg := theme.NewStyle().Background(theme.T().SurfaceOverlay)
	name := bg.Foreground(theme.T().TextOnSurface).Bold(true)
	dim := bg.Foreground(theme.T().TextDim)
	online := bg.Foreground(theme.T().StatusOnline)

	// The identity block sits beside the avatar, so it is written at the width
	// left over rather than at the overlay's own.
	textW := innerW
	if avatarW > 0 {
		textW = innerW - avatarW - avatarGap
	}

	var identity []string
	identity = append(identity, name.Render(truncate(p.user.DisplayName(), textW)))
	if p.user.Username != "" {
		identity = append(identity, dim.Render(truncate("@"+p.user.Username, textW)))
	}
	if p.user.Online {
		identity = append(identity, online.Render("online"))
	}

	lines := identity
	if avatarW > 0 {
		lines = p.headerLines(identity, bg, avatarW)
	}

	var detail []string
	if p.user.Bio != "" {
		for _, l := range wrapPlain(p.user.Bio, innerW) {
			detail = append(detail, bg.Foreground(theme.T().TextOnSurface).Render(l))
		}
	}
	if p.user.Phone != "" {
		detail = append(detail, dim.Render(truncate(formatPhone(p.user.Phone), innerW)))
	}
	if !p.full && p.user.Bio == "" && p.user.Phone == "" {
		// A partial profile says so rather than showing a gap that looks like
		// a person with nothing to say.
		detail = append(detail, dim.Render("…"))
	}
	if len(detail) > 0 {
		lines = append(lines, bg.Render(""))
		lines = append(lines, detail...)
	}
	return lines
}

// headerLines lays the avatar block beside the identity lines. Its height is
// whichever of the two is taller, so a person with a long identity block is not
// cut off and a short one does not cut the picture off.
func (p *Profile) headerLines(identity []string, bg lipgloss.Style, avatarW int) []string {
	cols, rows := avatarBoxFor(avatarW)
	block := p.avatarBlock(cols, rows)
	// Both are painted by the style rather than assembled from spaces: an
	// unpainted cell inside the surface reads as a hole (#227).
	blank := bg.Width(cols).Render("")
	gap := bg.Width(avatarGap).Render("")

	height := len(block)
	if len(identity) > height {
		height = len(identity)
	}
	lines := make([]string, 0, height)
	for i := 0; i < height; i++ {
		left := blank
		if i < len(block) {
			left = block[i]
		}
		right := ""
		if i < len(identity) {
			right = identity[i]
		}
		lines = append(lines, left+gap+right)
	}
	return lines
}

// formatPhone puts back the leading '+' Telegram strips, so the number reads as
// a number rather than as a long integer.
func formatPhone(phone string) string {
	if phone == "" || strings.HasPrefix(phone, "+") {
		return phone
	}
	return "+" + phone
}

// wrapPlain hard-wraps text to width on word boundaries, keeping any line
// breaks the text already had.
func wrapPlain(s string, width int) []string {
	if width <= 0 {
		return nil
	}
	var out []string
	for _, para := range strings.Split(s, "\n") {
		words := strings.Fields(para)
		if len(words) == 0 {
			out = append(out, "")
			continue
		}
		line := words[0]
		for _, w := range words[1:] {
			if lipgloss.Width(line)+1+lipgloss.Width(w) > width {
				out = append(out, line)
				line = w
				continue
			}
			line += " " + w
		}
		out = append(out, line)
	}
	return out
}

// truncate cuts s to width cells, marking the cut with an ellipsis.
func truncate(s string, width int) string {
	if width <= 0 || lipgloss.Width(s) <= width {
		return s
	}
	if width == 1 {
		return "…"
	}
	runes := []rune(s)
	for len(runes) > 0 && lipgloss.Width(string(runes))+1 > width {
		runes = runes[:len(runes)-1]
	}
	return string(runes) + "…"
}

// innerWidth is the content width the overlay renders at: as wide as its
// content wants, capped by the terminal. Zero means the terminal is too narrow
// to open on at all.
func (p *Profile) innerWidth(avatarW int) int {
	maxW := p.maxInnerWidth()
	if maxW == 0 {
		return 0
	}
	if want := p.contentWidth(avatarW); want < maxW {
		return want
	}
	return maxW
}

// maxInnerWidth is all the content width this terminal can give, and 0 when it
// cannot give enough to open on at all.
func (p *Profile) maxInnerWidth() int {
	maxW := p.width - profileHMargin - 2*profilePadH - 2
	if p.width < ProfileMinWidth || maxW < 1 {
		return 0
	}
	return maxW
}

// contentWidth is the width the overlay's content asks for with an avatar this
// wide, before the terminal has its say. The difference between this and what
// the terminal allows is what the ladder reads: a rung that asks for more than
// there is would be drawn squeezed, and a smaller picture is a better answer
// than a squeezed one.
func (p *Profile) contentWidth(avatarW int) int {
	// The identity block is measured with the avatar in front of it: those are
	// the lines the picture shares a row with, and the phone and the actions
	// below it start at the overlay's left edge either way.
	want := lipgloss.Width(p.user.DisplayName())
	if p.user.Username != "" {
		want = maxInt(want, lipgloss.Width("@"+p.user.Username))
	}
	if avatarW > 0 {
		want += avatarW + avatarGap
	}
	if p.user.Phone != "" {
		want = maxInt(want, lipgloss.Width(formatPhone(p.user.Phone)))
	}
	// The bottom border names the actions, and a border cannot be narrower than
	// what is written on it.
	want = maxInt(want, p.hintWidth())
	// A bio is wrapped rather than measured: it would otherwise decide the
	// width of the whole overlay on its own.
	if p.user.Bio != "" {
		want = maxInt(want, 32)
	}
	return want
}

// actionRow is the unstyled text of one action row: its key, then its label.
// It carries no indent of its own: the overlay's padding is the left margin, and
// one box with two margins is what made the actions look inset under a flush
// identity block (#236).
func (p *Profile) hint() string {
	pairs := make([][2]string, 0, len(p.actions))
	for _, a := range p.actions {
		pairs = append(pairs, [2]string{
			p.keyMap.KeyFor(keys.ContextProfile, a),
			DescribeShort(keys.ContextProfile, a),
		})
	}
	return OverlayHint(pairs, OverlayMenuBg())
}

// hintWidth is how much content width the bottom border needs to name every
// action. RenderBox draws the hint or drops it whole, and an overlay that opens
// without naming a single action is a broken screen rather than an answer, so
// this is a floor on the overlay's width rather than something to hope for.
func (p *Profile) hintWidth() int { return lipgloss.Width(p.hint()) }

// TooSmall reports that the viewport cannot hold the overlay. The caller says
// so rather than drawing a broken box.
//
// It asks about this profile rather than about the worst one imaginable: what
// the overlay needs depends on the person, and an answer of "not here" is only
// honest when it is about the screen in front of you (#216).
//
// Height counts as well as width. The ladder drops the avatar first, so this is
// the case where even the pictureless overlay is taller than the terminal —
// where the alternative is a box whose top border and bottom hint are stamped
// off the screen and never drawn.
func (p *Profile) TooSmall() bool {
	maxW := p.maxInnerWidth()
	if maxW == 0 || maxW < p.hintWidth() {
		return true
	}
	return p.overlayHeight(0) > p.height
}

// contentLines is everything inside the padding: the identity block, and
// nothing else — the actions are named on the bottom border. Taking the avatar
// width as an argument is what lets the ladder ask what the overlay would look
// like at a size before committing to it.
func (p *Profile) contentLines(innerW, avatarW int) []string {
	if innerW == 0 {
		return nil
	}
	return p.infoLines(innerW, avatarW)
}

func (p *Profile) View() string {
	if p.TooSmall() {
		return ""
	}
	avatarW := p.avatarCols()
	innerW := p.innerWidth(avatarW)

	lines := p.contentLines(innerW, avatarW)
	hint := p.hint()

	// Every line is padded to the content width so the overlay is one solid
	// surface rather than a ragged one: an unpainted cell inside a box reads as
	// a hole (#227).
	for i, l := range lines {
		if w := lipgloss.Width(l); w < innerW {
			lines[i] = l + theme.PadTo(w, innerW)
		}
	}

	// The padding is part of the surface, so it is painted by the surface's own
	// style rather than by bare spaces — a plain Padding would leave a ring of
	// holes around the content.
	padded := theme.S().MenuBg.Padding(profilePadV, profilePadH).Render(strings.Join(lines, "\n"))

	box := RenderBox(padded, "", "", hint, "",
		lipgloss.RoundedBorder(), nil, innerW+2*profilePadH+2, len(lines)+2*profilePadV+2)

	boxLines := strings.Split(box, "\n")
	for i, l := range boxLines {
		boxLines[i] = theme.S().MenuBg.Render(l)
	}
	return strings.Join(boxLines, "\n")
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
