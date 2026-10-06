package tg

import (
	"time"

	"github.com/gotd/td/tg"
	"github.com/sorokin-vladimir/tele/internal/domain"
)

// convertRichMessage converts the MTProto RichMessage attached to a Message
// (blocks + photos/documents) into tele's domain model. A nil rich message, or
// one with nothing renderable, returns nil so the caller falls back to the
// classic text+entities body.
//
// RichMessage blocks are the Instant View PageBlock family: paragraphs,
// headings, lists, tables, media and so on. Media blocks carry a PhotoID /
// VideoID / AudioID that refers into the message's photo/document arrays. Tele
// does not draw those yet — the media pipeline is keyed to one message-level
// photo/document, not to a block's — so the converter keeps each media block's
// caption (and, for map blocks, its coordinates) and the renderer draws a
// placeholder. See internal/ui/components/richrender.go.
func convertRichMessage(rm *tg.RichMessage) *domain.RichMessage {
	if rm == nil {
		return nil
	}
	out := &domain.RichMessage{
		Rtl:    rm.GetRtl(),
		Blocks: convertRichBlocks(rm.Blocks),
	}
	if !out.HasContent() {
		return nil
	}
	return out
}

func convertRichBlocks(blocks []tg.PageBlockClass) []domain.RichBlock {
	if len(blocks) == 0 {
		return nil
	}
	out := make([]domain.RichBlock, 0, len(blocks))
	for _, b := range blocks {
		out = append(out, convertRichBlock(b))
	}
	return out
}

func convertRichBlock(b tg.PageBlockClass) domain.RichBlock {
	switch v := b.(type) {
	case *tg.PageBlockParagraph:
		return domain.RichBlock{Kind: domain.RichParagraph, Text: convertRichText(v.Text)}
	case *tg.PageBlockTitle:
		return domain.RichBlock{Kind: domain.RichHeading, Level: 1, Text: convertRichText(v.Text)}
	case *tg.PageBlockHeader:
		return domain.RichBlock{Kind: domain.RichHeading, Level: 2, Text: convertRichText(v.Text)}
	case *tg.PageBlockSubheader:
		return domain.RichBlock{Kind: domain.RichHeading, Level: 3, Text: convertRichText(v.Text)}
	case *tg.PageBlockKicker:
		return domain.RichBlock{Kind: domain.RichHeading, Level: 4, Text: convertRichText(v.Text)}
	case *tg.PageBlockSubtitle:
		return domain.RichBlock{Kind: domain.RichSubtitle, Text: convertRichText(v.Text)}
	case *tg.PageBlockFooter:
		return domain.RichBlock{Kind: domain.RichFooter, Text: convertRichText(v.Text)}
	case *tg.PageBlockPreformatted:
		return domain.RichBlock{Kind: domain.RichPreformatted, Text: convertRichText(v.Text), Language: v.Language}
	case *tg.PageBlockBlockquote:
		return domain.RichBlock{
			Kind:    domain.RichBlockquote,
			Text:    convertRichText(v.Text),
			Caption: convertRichText(v.Caption),
		}
	case *tg.PageBlockBlockquoteBlocks:
		return domain.RichBlock{
			Kind:    domain.RichBlockquote,
			Blocks:  convertRichBlocks(v.Blocks),
			Caption: convertRichText(v.Caption),
		}
	case *tg.PageBlockPullquote:
		return domain.RichBlock{
			Kind:    domain.RichPullquote,
			Text:    convertRichText(v.Text),
			Caption: convertRichText(v.Caption),
		}
	case *tg.PageBlockAuthorDate:
		blk := domain.RichBlock{
			Kind:   domain.RichAuthorDate,
			Author: convertRichText(v.Author),
		}
		if v.PublishedDate > 0 {
			blk.Date = time.Unix(int64(v.PublishedDate), 0).Format("2 January 2006")
		}
		return blk
	case *tg.PageBlockDivider:
		return domain.RichBlock{Kind: domain.RichDivider}
	case *tg.PageBlockAnchor:
		return domain.RichBlock{Kind: domain.RichAnchor, Name: v.Name}
	case *tg.PageBlockList:
		return domain.RichBlock{Kind: domain.RichList, Items: convertRichListItems(v.Items)}
	case *tg.PageBlockOrderedList:
		start, ok := v.GetStart()
		if !ok {
			start = 0
		}
		return domain.RichBlock{
			Kind:     domain.RichOrderedList,
			Ordered:  true,
			Reversed: v.GetReversed(),
			Start:    start,
			Items:    convertRichOrderedItems(v.Items),
		}
	case *tg.PageBlockDetails:
		return domain.RichBlock{
			Kind:   domain.RichDetails,
			Open:   v.Open,
			Title:  convertRichText(v.Title),
			Blocks: convertRichBlocks(v.Blocks),
		}
	case *tg.PageBlockTable:
		blk := domain.RichBlock{
			Kind:     domain.RichTable,
			Bordered: v.Bordered,
			Striped:  v.Striped,
			Title:    convertRichText(v.Title),
			Rows:     make([][]domain.RichCell, 0, len(v.Rows)),
		}
		for _, row := range v.Rows {
			cells := make([]domain.RichCell, 0, len(row.Cells))
			for _, c := range row.Cells {
				cells = append(cells, domain.RichCell{
					Text:   convertRichText(c.Text),
					Header: c.Header,
				})
			}
			blk.Rows = append(blk.Rows, cells)
		}
		return blk
	case *tg.PageBlockRelatedArticles:
		blk := domain.RichBlock{Kind: domain.RichRelatedArticles, Title: convertRichText(v.Title)}
		for _, a := range v.Articles {
			blk.Articles = append(blk.Articles, domain.RichArticle{
				Title: a.Title,
				URL:   a.URL,
			})
		}
		return blk
	case *tg.PageBlockChannel:
		return domain.RichBlock{Kind: domain.RichChannel, ChannelTitle: channelBlockTitle(v.Channel)}
	case *tg.PageBlockMath:
		return domain.RichBlock{Kind: domain.RichMath, Text: domain.RichText{Kind: domain.RichTextPlain, Text: v.Source}}
	case *tg.PageBlockPhoto:
		return mediaBlock(domain.RichPhoto, v.Caption, nil)
	case *tg.PageBlockVideo:
		return mediaBlock(domain.RichVideo, v.Caption, nil)
	case *tg.PageBlockAudio:
		return mediaBlock(domain.RichAudio, v.Caption, nil)
	case *tg.PageBlockMap:
		blk := mediaBlock(domain.RichMap, v.Caption, nil)
		if geo, ok := v.Geo.(*tg.GeoPoint); ok {
			blk.Geo = domain.RichGeo{Lat: geo.Lat, Long: geo.Long, Zoom: v.Zoom}
		}
		return blk
	case *tg.PageBlockEmbed:
		return mediaBlock(domain.RichEmbed, v.Caption, &v.URL)
	case *tg.PageBlockEmbedPost:
		return mediaBlock(domain.RichEmbedPost, v.Caption, &v.URL)
	case *tg.PageBlockCollage:
		return domain.RichBlock{
			Kind:    domain.RichCollage,
			Caption: convertRichTextCaption(v.Caption),
			Blocks:  convertRichBlocks(v.Items),
		}
	case *tg.PageBlockSlideshow:
		return domain.RichBlock{
			Kind:    domain.RichSlideshow,
			Caption: convertRichTextCaption(v.Caption),
			Blocks:  convertRichBlocks(v.Items),
		}
	case *tg.PageBlockCover:
		// A cover wraps a single block that headlines the message; it renders as
		// its content would.
		return convertRichBlock(v.Cover)
	case *tg.PageBlockUnsupported:
		return domain.RichBlock{Kind: domain.RichUnsupported}
	default:
		return domain.RichBlock{Kind: domain.RichUnsupported}
	}
}

// mediaBlock builds a media placeholder block. url, when non-nil, names the
// target an embed opens; tele renders it dimmed under the caption.
func mediaBlock(kind domain.RichBlockKind, caption tg.PageCaption, url *string) domain.RichBlock {
	blk := domain.RichBlock{
		Kind:    kind,
		Caption: convertRichTextCaption(caption),
	}
	if url != nil {
		blk.URL = *url
	}
	// The converter does not carry the label itself: the renderer draws a
	// placeholder for each media kind from Kind alone. Keeping the kind is what
	// lets the placeholder say "photo", "video", "audio" and so on without a
	// parallel text field that could drift.
	return blk
}

// convertRichCaption converts a page caption into a block caption. A caption is
// the block's main text; PageCaption also carries a credit line that tele folds
// into the caption text, separated by a space, when present.
func convertRichTextCaption(c tg.PageCaption) domain.RichText {
	t := convertRichText(c.Text)
	credit := convertRichText(c.Credit)
	if credit.Kind == domain.RichTextPlain && credit.Text == "" {
		return t
	}
	if t.Kind == domain.RichTextPlain && t.Text == "" {
		return credit
	}
	return domain.RichText{
		Kind: domain.RichTextConcat,
		Children: []domain.RichText{
			t,
			{Kind: domain.RichTextPlain, Text: " "},
			credit,
		},
	}
}

func convertRichListItems(items []tg.PageListItemClass) []domain.RichItem {
	out := make([]domain.RichItem, 0, len(items))
	for _, it := range items {
		switch v := it.(type) {
		case *tg.PageListItemText:
			out = append(out, domain.RichItem{Text: convertRichText(v.Text), Checked: v.Checked})
		case *tg.PageListItemBlocks:
			out = append(out, domain.RichItem{Blocks: convertRichBlocks(v.Blocks), Checked: v.Checked})
		}
	}
	return out
}

func convertRichOrderedItems(items []tg.PageListOrderedItemClass) []domain.RichItem {
	out := make([]domain.RichItem, 0, len(items))
	for _, it := range items {
		switch v := it.(type) {
		case *tg.PageListOrderedItemText:
			out = append(out, domain.RichItem{Text: convertRichText(v.Text), Checked: v.Checked})
		case *tg.PageListOrderedItemBlocks:
			out = append(out, domain.RichItem{Blocks: convertRichBlocks(v.Blocks), Checked: v.Checked})
		}
	}
	return out
}

// channelBlockTitle names the chat a PageBlockChannel advertises, falling back
// to an id-based label when the title is absent, matching the dialog-list code.
func channelBlockTitle(ch tg.ChatClass) string {
	if ch == nil {
		return ""
	}
	switch v := ch.(type) {
	case *tg.Channel:
		return channelTitle(v)
	case *tg.Chat:
		return groupTitle(v)
	}
	return ""
}

// convertRichText converts one rich-text node recursively. Styled nodes (bold,
// italic, …) wrap their children with their Kind; Concat joins several nodes;
// leaves carry Text. The mapping mirrors convertEntities for classic messages,
// so the same terminal styles cover both.
func convertRichText(rt tg.RichTextClass) domain.RichText {
	switch v := rt.(type) {
	case nil, *tg.TextEmpty:
		return domain.RichText{}
	case *tg.TextPlain:
		return domain.RichText{Kind: domain.RichTextPlain, Text: v.Text}
	case *tg.TextBold:
		return styled(domain.RichTextBold, v.Text)
	case *tg.TextItalic:
		return styled(domain.RichTextItalic, v.Text)
	case *tg.TextUnderline:
		return styled(domain.RichTextUnderline, v.Text)
	case *tg.TextStrike:
		return styled(domain.RichTextStrike, v.Text)
	case *tg.TextFixed:
		return styled(domain.RichTextFixed, v.Text)
	case *tg.TextURL:
		return node(domain.RichTextURL, v.Text, v.URL, 0)
	case *tg.TextEmail:
		return node(domain.RichTextEmail, v.Text, v.Email, 0)
	case *tg.TextPhone:
		return node(domain.RichTextPhone, v.Text, v.Phone, 0)
	case *tg.TextConcat:
		children := make([]domain.RichText, 0, len(v.Texts))
		for _, c := range v.Texts {
			children = append(children, convertRichText(c))
		}
		return domain.RichText{Kind: domain.RichTextConcat, Children: children}
	case *tg.TextSubscript:
		return styled(domain.RichTextSubscript, v.Text)
	case *tg.TextSuperscript:
		return styled(domain.RichTextSuperscript, v.Text)
	case *tg.TextMarked:
		return styled(domain.RichTextMarked, v.Text)
	case *tg.TextImage:
		return domain.RichText{Kind: domain.RichTextImage, DocumentID: v.DocumentID}
	case *tg.TextAnchor:
		return styled(domain.RichTextAnchor, v.Text)
	case *tg.TextMath:
		return domain.RichText{Kind: domain.RichTextMath, Text: v.Source}
	case *tg.TextCustomEmoji:
		return domain.RichText{Kind: domain.RichTextCustomEmoji, DocumentID: v.DocumentID, Alt: v.Alt}
	case *tg.TextSpoiler:
		return styled(domain.RichTextSpoiler, v.Text)
	case *tg.TextMention:
		return styled(domain.RichTextMention, v.Text)
	case *tg.TextMentionName:
		return domain.RichText{
			Kind:   domain.RichTextMentionName,
			Text:   richTextPlain(v.Text),
			UserID: v.UserID,
		}
	case *tg.TextHashtag:
		return styled(domain.RichTextHashtag, v.Text)
	case *tg.TextCashtag:
		return styled(domain.RichTextCashtag, v.Text)
	case *tg.TextBotCommand:
		return styled(domain.RichTextBotCommand, v.Text)
	case *tg.TextBankCard:
		return styled(domain.RichTextBankCard, v.Text)
	case *tg.TextAutoURL:
		return styled(domain.RichTextAutoURL, v.Text)
	case *tg.TextAutoEmail:
		return styled(domain.RichTextAutoEmail, v.Text)
	case *tg.TextAutoPhone:
		return styled(domain.RichTextAutoPhone, v.Text)
	case *tg.TextDate:
		// TextDate shows a date/time formatted per its flag set. Tele shows the
		// plain child text; the flags decide nothing the terminal can draw.
		return convertRichText(v.Text)
	default:
		return domain.RichText{}
	}
}

// styled wraps a rich text child in a single-style node. The child is kept as a
// node (even a single plain leaf) so the renderer can flatten the tree to
// text+entities uniformly; containers never carry Text of their own.
func styled(kind domain.RichTextKind, child tg.RichTextClass) domain.RichText {
	out := domain.RichText{Kind: kind}
	if c := convertRichText(child); c.Kind != domain.RichTextPlain || c.Text != "" {
		out.Children = []domain.RichText{c}
	}
	return out
}

// node builds a styled node that carries both children and extra data (a link
// target, a phone number). Most such nodes hold one child.
func node(kind domain.RichTextKind, child tg.RichTextClass, data string, userID int64) domain.RichText {
	out := styled(kind, child)
	out.URL = data
	out.UserID = userID
	return out
}

// richTextPlain is the plain text of a rich-text subtree; used where a node
// holds a nested text as its visible content (TextMentionName).
func richTextPlain(rt tg.RichTextClass) string {
	if rt == nil {
		return ""
	}
	return convertRichText(rt).Plain()
}
