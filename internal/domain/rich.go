// Rich-message content model. Telegram Rich Messages (see Message.RichMessage)
// are structured: instead of one string plus inline entities, a message is a
// list of blocks (paragraph, heading, table, …), each block carrying rich text
// or further blocks. The types here are the subset of that structure tele
// renders, converted from the MTProto PageBlock/RichText classes in
// internal/tg/richparse.go. They are pure data — no gotd, no persistence — so
// both the store and the renderer can hold them.
package domain

import "strings"

// RichTextKind names the style of one rich-text node. The terminal renders
// most of these as a lipgloss attribute on the children's text; a few (URL,
// Email, Phone, Mention, …) also carry link colour or an openable target.
type RichTextKind int

const (
	RichTextPlain RichTextKind = iota
	RichTextBold
	RichTextItalic
	RichTextUnderline
	RichTextStrike
	RichTextFixed // inline fixed-width code
	RichTextURL
	RichTextEmail
	RichTextPhone
	RichTextConcat
	RichTextSubscript
	RichTextSuperscript
	RichTextMarked
	RichTextImage
	RichTextAnchor
	RichTextMath
	RichTextCustomEmoji
	RichTextSpoiler
	RichTextMention
	RichTextMentionName
	RichTextHashtag
	RichTextCashtag
	RichTextBotCommand
	RichTextBankCard
	RichTextAutoURL
	RichTextAutoEmail
	RichTextAutoPhone
	RichTextDate
)

// RichText is one node of a rich-text tree. Text runs carry Text; styled nodes
// carry children and apply their Kind to them; Concat joins several nodes; URL,
// Email, Phone, MentionName, CustomEmoji and Date carry extra data in the fields
// below.
type RichText struct {
	Kind   RichTextKind
	Text   string // plain leaf text; "" for styled/concat nodes
	URL    string // for URL: the link target
	UserID int64  // for MentionName: the mentioned user's id
	// DocumentID names the custom-emoji sticker document for RichTextCustomEmoji.
	// Tele does not download it (the mediacache pipeline is not wired into rich
	// text); Alt is rendered in its place, or a blank cell when Alt is a space,
	// which is how Telegram marks an unloaded emoji.
	DocumentID int64
	Alt        string
	Children   []RichText
}

// Plain returns the run of visible text a rich-text subtree draws: children's
// plain text concatenated, or the node's own text for a leaf. A custom emoji
// contributes its alt text; an image contributes nothing. This is what a
// snippet (reply preview, chat-list preview) shows, where styling has no room
// to matter.
func (r RichText) Plain() string {
	if len(r.Children) > 0 {
		var sb strings.Builder
		for _, c := range r.Children {
			sb.WriteString(c.Plain())
		}
		return sb.String()
	}
	if r.Kind == RichTextCustomEmoji {
		return strings.TrimSpace(r.Alt)
	}
	return r.Text
}

// RichItem is one entry of a list block: either styled text (PageListItemText)
// or nested blocks (PageListItemBlocks). Ordered lists number their items from
// the block's Start in render order.
type RichItem struct {
	Text    RichText
	Blocks  []RichBlock
	Checked bool
}

// RichCell is one table cell. Header cells render bold, which covers both a
// header row (every cell flagged) and row or column headers (one column of
// flagged cells); a bordered table additionally separates rows with grid rules.
type RichCell struct {
	Text   RichText
	Header bool
}

// RichBlockKind names a block of a rich message.
type RichBlockKind int

const (
	RichParagraph RichBlockKind = iota
	RichHeading                 // section title levels, see Level
	RichSubtitle
	RichFooter
	RichPreformatted
	RichBlockquote
	RichPullquote
	RichAuthorDate
	RichDivider
	RichAnchor
	RichList
	RichOrderedList
	RichDetails
	RichTable
	RichRelatedArticles
	RichChannel
	RichMath
	RichPhoto
	RichVideo
	RichAudio
	RichMap
	RichEmbed
	RichEmbedPost
	RichCollage
	RichSlideshow
	RichCover
	RichUnsupported
)

// RichBlock is a single block. Which fields are meaningful depends on Kind; the
// others are left zero. The shape is a union so that one renderer can walk a
// message's blocks uniformly, mirroring how the PageBlock classes share the
// PageBlockClass interface on the wire.
type RichBlock struct {
	Kind   RichBlockKind
	Text   RichText    // the block's own rich text (paragraph, headings, quotes, …)
	Level  int         // heading depth for RichHeading: 1 title … 4 kicker
	Blocks []RichBlock // nested content (details body, blockquote bodies, collage/slideshow items)

	// List blocks.
	Items    []RichItem
	Ordered  bool
	Reversed bool // ordered lists count down from Start
	Start    int  // first item number; 0 when Telegram gives none (renderer starts at 1)

	// Preformatted / math.
	Language string

	// Details.
	Title RichText
	Open  bool // whether the block renders expanded

	// Table.
	Bordered bool
	Striped  bool
	Rows     [][]RichCell

	// Author/date.
	Author RichText
	Date   string // formatted publication date; set by the converter

	// Anchor name, embed/web URLs.
	Name string // RichAnchor
	URL  string // RichEmbed / RichEmbedPost / RichChannel username? see below

	// Channel block: a chat reference drawn as its title.
	ChannelTitle string

	// Related articles.
	ArticleTitle string
	Articles     []RichArticle

	// Media blocks: Photo/Video/Audio/Map/Embed/Cover hold a placeholder label;
	// the actual bytes are fetched through the classic message media pipeline,
	// which is not wired into rich blocks yet.
	Caption RichText
	Geo     RichGeo // RichMap
}

// RichArticle is a related-article suggestion shown under a block.
type RichArticle struct {
	Title string
	URL   string
}

// RichGeo is a map center for a RichMap block.
type RichGeo struct {
	Lat, Long float64
	Zoom      int
}

// RichMessage is the converted form of tg.RichMessage: the block sequence plus
// the right-to-left flag. RTL is stored but not yet honoured by the renderer.
type RichMessage struct {
	Rtl    bool
	Blocks []RichBlock
}

// HasContent reports whether the message carries any block to render. A rich
// message that converted to nothing (all blocks unsupported, say) should fall
// back to the classic text path rather than draw an empty bubble.
func (r *RichMessage) HasContent() bool {
	return r != nil && len(r.Blocks) > 0
}
