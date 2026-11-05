package components

import (
	"fmt"
	"strings"
	"time"

	"charm.land/lipgloss/v2"
	"github.com/sorokin-vladimir/tele/internal/domain"
	"github.com/sorokin-vladimir/tele/internal/markup"
	"github.com/sorokin-vladimir/tele/internal/ui/textwidth"
	"github.com/sorokin-vladimir/tele/internal/ui/theme"
)

// Rich-message rendering. A rich message is a sequence of blocks (paragraph,
// heading, table, list, …); the renderer turns them into display rows laid out
// at a given content width, styled like the classic body — inline rich text is
// flattened to text + MessageEntities and painted through renderEntities with a
// per-block base style, so the terminal treats a rich paragraph exactly like a
// classic one.
//
// The message-list geometry treats the body as a list of final rows: it pads
// each row to the bubble width and frames it. Rich blocks that cannot share one
// width with a paragraph — tables and preformatted blocks — therefore wrap
// their own content to the content width and never rely on the caller's wrap.

// richMessageLines lays out a rich message into display rows at content width w.
// Blocks are separated by a blank row. Rows carry their own styling; they are
// not padded to width — the bubble renderer pads them. The result has no
// trailing or leading blank row.
func richMessageLines(r *domain.RichMessage, w int) []string {
	if r == nil || len(r.Blocks) == 0 {
		return nil
	}
	rows := renderBlockSequence(r.Blocks, w, 0)
	if len(rows) == 0 {
		// Every block rendered nothing (an anchor-only message, say). Keep the
		// height and render lock-step by drawing one empty content row, exactly
		// as the classic empty-text path does.
		rows = []string{""}
	}
	return rows
}

// richMessageWidth returns the content width a rich message would like, laid
// out at up to maxW: the widest row it produces, capped at maxW. Callers use it
// the way the text path measures msg.Text to size the bubble.
func richMessageWidth(r *domain.RichMessage, maxW int) int {
	if r == nil || len(r.Blocks) == 0 {
		return 0
	}
	w := 0
	for i := range r.Blocks {
		rows := richBlockLines(&r.Blocks[i], maxW, 0)
		for _, row := range rows {
			if lw := textwidth.Width(strings.TrimRight(row, " ")); lw > w {
				w = lw
			}
		}
	}
	if w > maxW {
		w = maxW
	}
	if w < 1 {
		w = 1
	}
	return w
}

// richBlockLines renders one block into rows at content width w. depth is the
// block nesting depth and indents nested content (details bodies, list items).
func richBlockLines(b *domain.RichBlock, w int, depth int) []string {
	if w < 1 {
		w = 1
	}
	switch b.Kind {
	case domain.RichParagraph:
		return paraLines(b.Text, w, theme.S().Body)
	case domain.RichHeading:
		base := theme.S().Body
		if b.Level >= 4 {
			base = base.Foreground(theme.T().Accent)
		} else {
			base = base.Bold(true)
		}
		return paraLines(b.Text, w, base)
	case domain.RichSubtitle:
		return paraLines(b.Text, w, theme.S().Body.Italic(true))
	case domain.RichFooter:
		return paraLines(b.Text, w, theme.S().Body.Foreground(theme.T().TextDim))
	case domain.RichAuthorDate:
		return authorDateLines(b, w)
	case domain.RichPreformatted:
		return preformattedLines(b, w)
	case domain.RichBlockquote:
		return blockquoteLines(b, w, depth)
	case domain.RichPullquote:
		return pullquoteLines(b, w)
	case domain.RichList, domain.RichOrderedList:
		return listLines(b, w)
	case domain.RichDetails:
		return detailsLines(b, w, depth)
	case domain.RichTable:
		return tableLines(b, w)
	case domain.RichAnchor:
		return nil // an anchor has no visible content
	case domain.RichDivider:
		return []string{divider(w)}
	case domain.RichMath:
		return paraLines(b.Text, w, theme.S().Body.Italic(true))
	case domain.RichRelatedArticles:
		return relatedLines(b, w)
	case domain.RichChannel:
		if b.ChannelTitle == "" {
			return nil
		}
		return []string{theme.S().BodyBold.Render(b.ChannelTitle)}
	case domain.RichUnsupported:
		return []string{theme.S().OverlayHintDim.Render("[unsupported content]")}

	case domain.RichPhoto, domain.RichVideo, domain.RichAudio,
		domain.RichMap, domain.RichEmbed, domain.RichEmbedPost:
		return mediaLines(b, w)
	case domain.RichCollage, domain.RichSlideshow:
		return groupMediaLines(b, w)
	}
	return nil
}

// paraLines renders a block's rich text as wrapped paragraph rows at width w,
// painted with base. An empty text produces no rows.
func paraLines(rt domain.RichText, w int, base lipgloss.Style) []string {
	text, ents := flattenRich(rt)
	if strings.TrimSpace(text) == "" {
		return nil
	}
	return wrapStyled(renderEntities(text, ents, base), w)
}

// authorDateLines renders the "author · date" line of a byline block.
func authorDateLines(b *domain.RichBlock, w int) []string {
	author := b.Author.Plain()
	date := ""
	if b.Date != 0 {
		date = time.Unix(b.Date, 0).Format("2 January 2006")
	}
	switch {
	case author == "" && date == "":
		return nil
	case author == "":
		return []string{theme.S().Timestamp.Render(date)}
	case date == "":
		return paraLines(b.Author, w, theme.S().Body.Bold(true))
	default:
		name := paraLines(b.Author, w, theme.S().Body.Bold(true))
		if len(name) == 0 {
			return []string{theme.S().Timestamp.Render(date)}
		}
		// Fold the date onto the last name row so the byline stays on one line
		// when it fits.
		stamped := theme.S().Timestamp.Render("  " + date)
		last := name[len(name)-1]
		joined := last + stamped
		if textwidth.Width(joined) <= w {
			name[len(name)-1] = joined
			return name
		}
		name = append(name, stamped)
		return name
	}
}

// preformattedLines renders a preformatted block one source line per row, each
// painted with the code style, preserving the line breaks.
func preformattedLines(b *domain.RichBlock, w int) []string {
	text := b.Text.Plain()
	if strings.TrimSpace(text) == "" {
		return nil
	}
	code := theme.S().Body.Background(theme.T().SurfaceCode).Foreground(theme.T().TextCode)
	var rows []string
	for _, line := range strings.Split(text, "\n") {
		if line == "" {
			rows = append(rows, "")
			continue
		}
		rows = append(rows, wrapStyled(code.Render(line), w)...)
	}
	return rows
}

// blockquoteLines renders a quote block: its own text (or nested blocks) with a
// quote glyph on the left of every row. depth controls the inner indentation of
// nested quote blocks.
func blockquoteLines(b *domain.RichBlock, w int, depth int) []string {
	prefix := quotePrefix(depth)
	if textwidth.Width(prefix) >= w {
		// No room for the glyph: the quote's content still gets the whole width.
		prefix = ""
	}
	innerW := w - textwidth.Width(prefix)
	if innerW < 1 {
		innerW = 1
	}
	var rows []string
	var quoteRows []string
	if len(b.Blocks) > 0 {
		quoteRows = renderBlockSequence(b.Blocks, innerW, depth)
	} else {
		quoteRows = paraLines(b.Text, innerW, theme.S().Body)
	}
	for _, row := range quoteRows {
		rows = append(rows, prefix+row)
	}
	if rows == nil {
		return nil
	}
	if hasCaption(b.Caption) {
		caption := paraLines(b.Caption, innerW, theme.S().Body.Foreground(theme.T().TextDim))
		if len(caption) > 0 {
			rows = append(rows, "")
			rows = append(rows, caption...)
		}
	}
	return rows
}

func quotePrefix(depth int) string {
	return theme.Pad(2*depth) + theme.S().Quote.Render(quoteGlyph)
}

// hasCaption reports whether a caption node carries anything to draw. An empty
// plain node is how the converter spells "no caption".
func hasCaption(rt domain.RichText) bool {
	return rt.Kind != domain.RichTextPlain || rt.Text != ""
}

// pullquoteLines renders a pull quote (larger, set-off quote): an empty row, the
// quote text, then its caption as the attribution.
func pullquoteLines(b *domain.RichBlock, w int) []string {
	body := paraLines(b.Text, w, theme.S().Body.Italic(true))
	if len(body) == 0 {
		return nil
	}
	if hasCaption(b.Caption) {
		caption := paraLines(b.Caption, w, theme.S().Body.Foreground(theme.T().TextDim))
		if len(caption) > 0 {
			body = append(body, "")
			body = append(body, caption...)
		}
	}
	return body
}

// listLines renders a bulleted or ordered list, hanging the item markers in
// their own column so wrapped item lines align under the first word.
func listLines(b *domain.RichBlock, w int) []string {
	markers := make([]string, len(b.Items))
	markerW := 0
	start := b.Start
	if start == 0 {
		start = 1
	}
	if b.Reversed {
		start = start + len(b.Items) - 1
	}
	for i := range b.Items {
		if b.Kind == domain.RichOrderedList {
			n := start
			if b.Reversed {
				n = start - i
			} else {
				n = start + i
			}
			markers[i] = fmt.Sprintf("%d. ", n)
		} else {
			markers[i] = "• "
		}
		if lw := textwidth.Width(markers[i]); lw > markerW {
			markerW = lw
		}
	}
	// Pad the markers into one column, then paint them: a marker is a cell on
	// screen and must carry the body style like the text that follows it.
	for i := range markers {
		markers[i] += theme.Pad(markerW - textwidth.Width(markers[i]))
		markers[i] = theme.S().Body.Render(markers[i])
	}
	if markerW >= w {
		// No room for a marker column; the item text gets the whole width.
		for i := range markers {
			markers[i] = ""
		}
		markerW = 0
	}

	innerW := w - markerW
	if innerW < 1 {
		innerW = 1
	}
	var rows []string
	for i, item := range b.Items {
		if len(item.Blocks) > 0 {
			itemRows := renderBlockSequence(item.Blocks, innerW, 1)
			for j, row := range itemRows {
				prefix := theme.Pad(markerW)
				if j == 0 {
					prefix = markers[i]
				}
				rows = append(rows, prefix+row)
			}
			continue
		}
		lines := paraLines(item.Text, innerW, theme.S().Body)
		for j, line := range lines {
			prefix := theme.Pad(markerW)
			if j == 0 {
				prefix = markers[i]
			}
			rows = append(rows, prefix+line)
		}
	}
	return rows
}

// detailsLines renders a collapsible details block. Terminal tele cannot toggle
// the block, so the content always renders, indented under the title. depth
// indents nested details.
func detailsLines(b *domain.RichBlock, w int, depth int) []string {
	title := paraLines(b.Title, w, theme.S().Body.Bold(true))
	if len(title) == 0 && len(b.Blocks) == 0 {
		return nil
	}
	rows := append([]string{}, title...)
	if len(b.Blocks) > 0 {
		if len(rows) > 0 {
			rows = append(rows, "")
		}
		prefix := theme.Pad(2)
		if textwidth.Width(prefix) >= w {
			// No room for the indent: the body gets the whole width.
			prefix = ""
		}
		innerW := w - textwidth.Width(prefix)
		if innerW < 1 {
			innerW = 1
		}
		content := renderBlockSequence(b.Blocks, innerW, depth+1)
		for _, row := range content {
			rows = append(rows, prefix+row)
		}
	}
	return rows
}

// renderBlockSequence lays out a sequence of sibling blocks at width w with
// blank-row separation, sharing the top-level join logic.
func renderBlockSequence(blocks []domain.RichBlock, w int, depth int) []string {
	var rows []string
	for i := range blocks {
		blockRows := richBlockLines(&blocks[i], w, depth)
		if len(blockRows) == 0 {
			continue
		}
		if len(rows) > 0 {
			rows = append(rows, "")
		}
		rows = append(rows, blockRows...)
	}
	if len(rows) > 0 && rows[len(rows)-1] == "" {
		rows = rows[:len(rows)-1]
	}
	return rows
}

// relatedLines renders the "related articles" suggestions: a title line, then
// each article as an underlined title with its URL beneath, dimmed.
func relatedLines(b *domain.RichBlock, w int) []string {
	rows := paraLines(b.Title, w, theme.S().Body.Bold(true))
	for _, a := range b.Articles {
		if a.Title == "" {
			continue
		}
		if len(rows) > 0 {
			rows = append(rows, "")
		}
		rows = append(rows, wrapStyled(theme.S().Body.Underline(true).Render(a.Title), w)...)
		if a.URL != "" {
			rows = append(rows, wrapStyled(theme.S().OverlayHintDim.Render(a.URL), w)...)
		}
	}
	return rows
}

// divider is a full-width row of box-drawing dashes, drawn at the content width
// it is laid out at.
func divider(w int) string {
	if w < 1 {
		w = 1
	}
	return theme.S().Quote.Render(strings.Repeat("─", w))
}

// mediaLines renders a single-media placeholder block: a dim "[type]" row
// (embed blocks add their URL), then any caption beneath.
func mediaLines(b *domain.RichBlock, w int) []string {
	label := mediaLabel(b.Kind)
	if b.URL != "" {
		label = "embed: " + b.URL
	}
	var rows []string
	rows = append(rows, wrapStyled(theme.S().OverlayHintDim.Render("["+label+"]"), w)...)
	if hasCaption(b.Caption) {
		caption := paraLines(b.Caption, w, theme.S().Body)
		if len(caption) > 0 {
			rows = append(rows, "")
			rows = append(rows, caption...)
		}
	}
	return rows
}

// groupMediaLines renders a collage or slideshow as its member media blocks
// (each a placeholder row), followed by the group caption.
func groupMediaLines(b *domain.RichBlock, w int) []string {
	var rows []string
	for i := range b.Blocks {
		part := mediaLines(&b.Blocks[i], w)
		if len(part) == 0 {
			continue
		}
		if len(rows) > 0 {
			rows = append(rows, "")
		}
		rows = append(rows, part...)
	}
	if hasCaption(b.Caption) {
		caption := paraLines(b.Caption, w, theme.S().Body)
		if len(caption) > 0 {
			if len(rows) > 0 {
				rows = append(rows, "")
			}
			rows = append(rows, caption...)
		}
	}
	return rows
}

func mediaLabel(kind domain.RichBlockKind) string {
	switch kind {
	case domain.RichPhoto:
		return "photo"
	case domain.RichVideo:
		return "video"
	case domain.RichAudio:
		return "audio"
	case domain.RichMap:
		return "map"
	case domain.RichEmbedPost:
		return "post"
	}
	return "embed"
}

// tableLines renders a table into display rows fitted to content width w. Cell
// text wraps at its column width and each column is padded to a fixed slot, so
// the vertical separators line up across rows. Cells flagged Header render bold,
// honouring header rows and row/column headers alike. A bordered table draws the
// box and grid rules; a plain one separates columns with padding only.
//
// The framing is chosen to fit: when the table would not fit, cell padding is
// dropped first, then the border. A table with more columns than the bubble has
// cells falls back to one text row per table row.
func tableLines(b *domain.RichBlock, w int) []string {
	if len(b.Rows) == 0 {
		return nil
	}
	cols := 0
	for _, row := range b.Rows {
		if len(row) > cols {
			cols = len(row)
		}
	}
	if cols == 0 {
		return nil
	}

	// Natural column width: the widest cell text. Measured on the plain text,
	// since ANSI runs would skew the display-width count.
	natural := make([]int, cols)
	for _, row := range b.Rows {
		for ci := range row {
			if ci < cols {
				if lw := textwidth.Width(row[ci].Text.Plain()); lw > natural[ci] {
					natural[ci] = lw
				}
			}
		}
	}

	// Column layout cost. Every column slot normally carries a space of padding
	// on each side of its text. A bordered table also spends one column per
	// separator bar (cols+1 of them: both edges and between columns).
	padW := 1
	bordered := b.Bordered
	seps := 0
	if bordered {
		seps = cols + 1
	}
	budget := w - 2*cols*padW - seps
	if budget < cols && padW > 0 {
		padW = 0
		budget = w - seps
	}
	if budget < cols && bordered {
		bordered = false
		budget = w
	}
	if budget < cols {
		return tableFallbackLines(b.Rows, w)
	}
	widths := distributeWidths(natural, budget)

	// Wrap every cell to its column width; the rows below then emit as many
	// table lines as the tallest cell needs. A flagged Header cell is painted
	// with the bold body style.
	cellRows := make([][][]string, len(b.Rows))
	for ri, row := range b.Rows {
		cellRows[ri] = make([][]string, cols)
		for ci := 0; ci < cols; ci++ {
			if ci >= len(row) {
				continue // a row shorter than the table leaves empty trailing cells
			}
			base := theme.S().Body
			if row[ci].Header {
				base = theme.S().BodyBold
			}
			cellRows[ri][ci] = paraLines(row[ci].Text, widths[ci], base)
			if len(cellRows[ri][ci]) == 0 {
				cellRows[ri][ci] = []string{""}
			}
		}
	}

	var rows []string
	if bordered {
		rows = append(rows, ruleRow(widths, padW, "┌", "┬", "┐"))
	}
	for ri := range b.Rows {
		height := 0
		for ci := 0; ci < cols; ci++ {
			if h := len(cellRows[ri][ci]); h > height {
				height = h
			}
		}
		for line := 0; line < height; line++ {
			var sb strings.Builder
			if bordered {
				sb.WriteString(theme.S().Quote.Render("│"))
			}
			for ci := 0; ci < cols; ci++ {
				cell := ""
				if line < len(cellRows[ri][ci]) {
					cell = cellRows[ri][ci][line]
				}
				// Pad the wrapped line out to the column width so separators
				// and the next line line up across rows, then frame the slot.
				cell += theme.PadTo(textwidth.Width(cell), widths[ci])
				if padW > 0 {
					sb.WriteString(theme.Pad(1))
				}
				sb.WriteString(cell)
				if padW > 0 {
					sb.WriteString(theme.Pad(1))
				}
				if bordered {
					sb.WriteString(theme.S().Quote.Render("│"))
				}
			}
			rows = append(rows, sb.String())
		}
		if bordered && ri < len(b.Rows)-1 {
			rows = append(rows, ruleRow(widths, padW, "├", "┼", "┤"))
		}
	}
	if bordered {
		rows = append(rows, ruleRow(widths, padW, "└", "┴", "┘"))
	}
	return rows
}

// tableFallbackLines renders a table too wide for its column slots as one
// wrapped text row per row, keeping the cell text rather than dropping columns.
func tableFallbackLines(rows [][]domain.RichCell, w int) []string {
	var out []string
	for _, row := range rows {
		parts := make([]string, 0, len(row))
		for _, c := range row {
			text := c.Text.Plain()
			if strings.TrimSpace(text) != "" {
				parts = append(parts, text)
			}
		}
		if len(parts) == 0 {
			continue
		}
		out = append(out, wrapStyled(theme.S().Body.Render(strings.Join(parts, " | ")), w)...)
	}
	return out
}

// ruleRow builds one horizontal table rule (top, middle, or bottom) across the
// column slots.
func ruleRow(widths []int, padW int, left, mid, right string) string {
	bar := theme.S().Quote
	var sb strings.Builder
	sb.WriteString(bar.Render(left))
	for ci := range widths {
		sb.WriteString(bar.Render(strings.Repeat("─", widths[ci]+2*padW)))
		if ci < len(widths)-1 {
			sb.WriteString(bar.Render(mid))
		}
	}
	sb.WriteString(bar.Render(right))
	return sb.String()
}

// distributeWidths shrinks natural column widths to fit a budget of text
// columns. When it shrinks, every column keeps at least one cell and the widths
// never sum past the budget.
func distributeWidths(natural []int, budget int) []int {
	n := len(natural)
	widths := make([]int, n)
	if n == 0 {
		return widths
	}
	total := 0
	for _, nw := range natural {
		total += nw
	}
	if total <= budget {
		copy(widths, natural)
		return widths
	}
	if budget < n {
		budget = n
	}
	// Reserve one cell per column, then share what is left in proportion to the
	// natural width above that minimum. Floor shares leave a few cells over;
	// the largest remainders take them one at a time.
	extra := total - n
	remaining := budget - n
	rems := make([]int, n)
	for i, nw := range natural {
		widths[i] = 1
		if extra <= 0 {
			continue
		}
		widths[i] += (nw - 1) * remaining / extra
		rems[i] = (nw - 1) * remaining % extra
	}
	left := budget
	for _, width := range widths {
		left -= width
	}
	for left > 0 {
		best := -1
		for i := range widths {
			if widths[i] >= natural[i] {
				continue
			}
			if best == -1 || rems[i] > rems[best] ||
				(rems[i] == rems[best] && widths[i] < widths[best]) {
				best = i
			}
		}
		if best == -1 {
			break
		}
		widths[best]++
		rems[best] = -1
		left--
	}
	return widths
}

// wrapStyled hard-wraps a styled string at width w and returns the rows. The
// input arrives painted run by run; this only measures and breaks lines, so the
// wrap style must not carry the canvas itself — the padding is re-emitted by
// the caller carrying it.
func wrapStyled(s string, w int) []string {
	if s == "" {
		return nil
	}
	if w < 1 {
		w = 1
	}
	var rows []string
	for _, part := range strings.Split(s, "\n") {
		if part == "" {
			rows = append(rows, "")
			continue
		}
		for _, line := range strings.Split(textwidth.Wrap(part, w), "\n") {
			rows = append(rows, strings.TrimRight(line, " "))
		}
	}
	return rows
}

// flattenRich walks a rich-text tree and flattens it to plain text plus inline
// entities (UTF-16 offsets, like every other entity in the domain), so a block's
// content can be painted by renderEntities exactly as a classic message body is.
func flattenRich(rt domain.RichText) (string, []domain.MessageEntity) {
	f := &richFlattener{}
	f.walk(rt)
	return f.sb.String(), f.ents
}

type richFlattener struct {
	sb   strings.Builder
	u16  int
	ents []domain.MessageEntity
}

func (f *richFlattener) write(s string) {
	f.sb.WriteString(s)
	f.u16 += markup.UTF16Len(s)
}

func (f *richFlattener) walk(rt domain.RichText) {
	if len(rt.Children) > 0 {
		if typ, url := richEntity(rt); typ != "" {
			start := f.u16
			for _, c := range rt.Children {
				f.walk(c)
			}
			if n := f.u16 - start; n > 0 {
				e := domain.MessageEntity{Type: typ, Offset: start, Length: n, URL: url, UserID: rt.UserID}
				f.ents = append(f.ents, e)
			}
			return
		}
		for _, c := range rt.Children {
			f.walk(c)
		}
		return
	}
	switch rt.Kind {
	case domain.RichTextImage:
		// no visible cell
	case domain.RichTextCustomEmoji:
		if alt := strings.TrimSpace(rt.Alt); alt != "" {
			f.write(alt)
		}
	case domain.RichTextMath:
		if rt.Text != "" {
			f.write(rt.Text)
		}
	default:
		if rt.Text != "" {
			f.write(rt.Text)
		}
	}
}

// richEntity maps a rich-text style onto the MessageEntity type it renders as.
// Containers and leaves with their own flattening (Concat, Image, Math,
// CustomEmoji) have no entity and return "", so their children render plain.
func richEntity(rt domain.RichText) (typ, url string) {
	switch rt.Kind {
	case domain.RichTextBold:
		return "bold", ""
	case domain.RichTextItalic:
		return "italic", ""
	case domain.RichTextUnderline:
		return "underline", ""
	case domain.RichTextStrike:
		return "strike", ""
	case domain.RichTextFixed:
		return "code", ""
	case domain.RichTextURL:
		return "text_url", rt.URL
	case domain.RichTextEmail:
		return "text_url", "mailto:" + rt.URL
	case domain.RichTextPhone:
		return "text_url", "tel:" + rt.URL
	case domain.RichTextMention:
		return "mention", ""
	case domain.RichTextMentionName:
		return "mention_name", ""
	case domain.RichTextHashtag:
		return "hashtag", ""
	case domain.RichTextCashtag:
		return "cashtag", ""
	case domain.RichTextBotCommand:
		return "bot_command", ""
	case domain.RichTextBankCard:
		return "bank_card", ""
	case domain.RichTextAutoURL:
		return "url", ""
	case domain.RichTextAutoEmail:
		return "email", ""
	case domain.RichTextAutoPhone:
		return "phone", ""
	}
	return "", ""
}
