package components

import (
	"strings"
	"testing"
	"time"

	xansi "github.com/charmbracelet/x/ansi"
	"github.com/sorokin-vladimir/tele/internal/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func para(text string) domain.RichText {
	return domain.RichText{Kind: domain.RichTextPlain, Text: text}
}

func richMessageWith(blocks ...domain.RichBlock) *domain.RichMessage {
	return &domain.RichMessage{Blocks: blocks}
}

func stripAll(rows []string) []string {
	out := make([]string, len(rows))
	for i, r := range rows {
		out[i] = xansi.Strip(r)
	}
	return out
}

func TestRichMessageLines_NoRowsWhenNil(t *testing.T) {
	assert.Nil(t, richMessageLines(nil, 40))
	assert.Nil(t, richMessageLines(&domain.RichMessage{}, 40))
}

func TestRichMessageLines_ParagraphWrapAndHug(t *testing.T) {
	r := richMessageWith(domain.RichBlock{Kind: domain.RichParagraph, Text: para("один два три четыре пять")})
	rows := richMessageLines(r, 10)
	require.NotEmpty(t, rows)
	// Wrapped into two rows, not padded to the 10-wide budget.
	assert.Greater(t, len(rows), 1)
	for _, row := range rows {
		assert.LessOrEqual(t, lipglossWidth(row), 10, "paragraph row must fit its content width")
	}
	assert.Equal(t, 10, richMessageWidth(r, 10), "a paragraph filling the budget wants full width")
}

func TestRichMessageLines_ShortMessageHugs(t *testing.T) {
	r := richMessageWith(domain.RichBlock{Kind: domain.RichParagraph, Text: para("hi")})
	assert.Equal(t, "hi", stripAll(richMessageLines(r, 200))[0])
	assert.Equal(t, 2, richMessageWidth(r, 200), "short text hugs its natural width")
}

func TestRichMessageLines_HeadingSubtitleBlockquote(t *testing.T) {
	r := richMessageWith(
		domain.RichBlock{Kind: domain.RichHeading, Level: 1, Text: para("Headline")},
		domain.RichBlock{Kind: domain.RichSubtitle, Text: para("sub")},
		domain.RichBlock{Kind: domain.RichParagraph, Text: para("body")},
		domain.RichBlock{Kind: domain.RichBlockquote, Text: para("quoted")},
	)
	rows := stripAll(richMessageLines(r, 60))
	joined := strings.Join(rows, "\n")
	assert.Contains(t, joined, "Headline")
	assert.Contains(t, joined, "sub")
	assert.Contains(t, joined, "body")
	assert.Contains(t, joined, "quoted")
	assert.Contains(t, joined, "▌ ", "a quote carries the quote glyph")
}

func TestRichMessageLines_ListMarkers(t *testing.T) {
	r := richMessageWith(domain.RichBlock{
		Kind: domain.RichList,
		Items: []domain.RichItem{
			{Text: para("milk")},
			{Text: para("eggs")},
		},
	})
	rows := stripAll(richMessageLines(r, 40))
	assert.Equal(t, "• milk", strings.TrimSpace(rows[0]))
	assert.Equal(t, "• eggs", strings.TrimSpace(rows[1]))

	ol := richMessageWith(domain.RichBlock{
		Kind:    domain.RichOrderedList,
		Ordered: true,
		Items: []domain.RichItem{
			{Text: para("first")},
			{Text: para("second")},
		},
	})
	orows := stripAll(richMessageLines(ol, 40))
	assert.Equal(t, "1. first", strings.TrimSpace(orows[0]))
	assert.Equal(t, "2. second", strings.TrimSpace(orows[1]))
}

func TestRichMessageLines_Table(t *testing.T) {
	r := richMessageWith(domain.RichBlock{
		Kind:     domain.RichTable,
		Bordered: true,
		Rows: [][]domain.RichCell{
			{
				{Text: para("Name"), Header: true},
				{Text: para("Qty"), Header: true},
			},
			{
				{Text: para("Milk")},
				{Text: para("1")},
			},
		},
	})
	rows := stripAll(richMessageLines(r, 60))
	require.NotEmpty(t, rows)
	joined := strings.Join(rows, "\n")
	assert.Contains(t, joined, "Name")
	assert.Contains(t, joined, "Milk")
	assert.Contains(t, joined, "┌", "a bordered table draws its frame")
	assert.Contains(t, joined, "│", "columns are separated by bars")
	assert.Contains(t, joined, "└")
	// Table never exceeds the content width it was laid out for.
	for _, row := range rows {
		assert.LessOrEqual(t, lipglossWidth(row), 60, "a table row must fit the content width")
	}
	// Columns line up: the bar sits at the same offset in every content row.
	assert.Contains(t, rows[1], "│", "the header row is separated by bars")
	col := strings.Index(rows[1], "│")
	assert.GreaterOrEqual(t, col, 0)
	assert.Equal(t, col, strings.Index(rows[3], "│"), "table separators must line up across rows")
}

func TestRichMessageLines_HeaderRowBold(t *testing.T) {
	r := richMessageWith(domain.RichBlock{
		Kind:     domain.RichTable,
		Bordered: true,
		Rows: [][]domain.RichCell{
			{
				{Text: para("Name"), Header: true},
				{Text: para("Qty"), Header: true},
			},
			{
				{Text: para("Milk")},
				{Text: para("1")},
			},
		},
	})
	rows := richMessageLines(r, 60) // raw, ANSI intact
	require.Len(t, rows, 5)         // top rule, header, rule, body, bottom rule
	assert.Contains(t, rows[1], "\x1b[1m", "header cells render bold")
	assert.Contains(t, xansi.Strip(rows[1]), "Name")
	assert.NotContains(t, rows[3], "\x1b[1m", "body cells stay in the body style")
	assert.Contains(t, xansi.Strip(rows[3]), "Milk")
}

func TestRichMessageLines_RowHeaderBold(t *testing.T) {
	r := richMessageWith(domain.RichBlock{
		Kind: domain.RichTable,
		Rows: [][]domain.RichCell{
			{
				{Text: para("left"), Header: true},
				{Text: para("right")},
			},
			{
				{Text: para("l2"), Header: true},
				{Text: para("r2")},
			},
		},
	})
	rows := richMessageLines(r, 60)
	require.Len(t, rows, 2)
	for _, row := range rows {
		first, second := row, row
		// Split the row at the two-space gap between the unbordered columns and
		// check each side independently: only the flagged first column is bold.
		first, second = splitRowColumns(row)
		assert.Contains(t, first, "\x1b[1m", "the flagged column header is bold")
		assert.NotContains(t, second, "\x1b[1m", "an unflagged cell stays in the body style")
	}
}

// splitRowColumns splits an unbordered two-column table row at its two-space gap,
// returning the raw (ANSI-bearing) halves. Column 0 is padded to its slot, so
// the split point is the first run of two spaces.
func splitRowColumns(row string) (string, string) {
	i := strings.Index(row, "  ")
	if i < 0 {
		return row, ""
	}
	return row[:i], row[i+2:]
}

func TestRichMessageLines_TableNarrowFits(t *testing.T) {
	r := richMessageWith(domain.RichBlock{
		Kind:     domain.RichTable,
		Bordered: true,
		Rows: [][]domain.RichCell{
			{{Text: para("a")}, {Text: para("b")}},
			{{Text: para("c")}, {Text: para("d")}},
		},
	})
	assert.Equal(t, 9, richMessageWidth(r, 60), "a small bordered table hugs its natural width (pads + bars + two 1-wide cells)")
}

func TestRichMessageLines_DetailsExpandedShowsBody(t *testing.T) {
	r := richMessageWith(domain.RichBlock{
		Kind:  domain.RichDetails,
		Title: para("More"),
		Blocks: []domain.RichBlock{
			{Kind: domain.RichParagraph, Text: para("the hidden body")},
		},
	})
	rows := stripAll(richMessageLines(r, 40))
	joined := strings.Join(rows, "\n")
	assert.Contains(t, joined, "More")
	assert.Contains(t, joined, "the hidden body")
}

func TestRichMessageLines_MediaAndDivider(t *testing.T) {
	r := richMessageWith(
		domain.RichBlock{Kind: domain.RichPhoto, Caption: para("sunset")},
		domain.RichBlock{Kind: domain.RichVideo},
		domain.RichBlock{Kind: domain.RichDivider},
	)
	rows := stripAll(richMessageLines(r, 40))
	joined := strings.Join(rows, "\n")
	assert.Contains(t, joined, "[photo]")
	assert.Contains(t, joined, "sunset")
	assert.Contains(t, joined, "[video]")
	assert.Contains(t, joined, "─")
}

// The message-list geometry (msgHeight) and the render must agree for rich
// messages too, across terminal widths and message shapes — the same lock-step
// the tail test enforces for classic text (#231).
func TestRichItemHeightMatchesRenderedLines(t *testing.T) {
	now := time.Now()
	richMsg := func(id int, blocks ...domain.RichBlock) domain.Message {
		return domain.Message{ID: id, Date: now, Rich: richMessageWith(blocks...)}
	}
	longText := strings.Repeat("слово ", 30)
	msgs := []domain.Message{
		richMsg(1, domain.RichBlock{Kind: domain.RichParagraph, Text: para("hi")}),
		richMsg(2,
			domain.RichBlock{Kind: domain.RichHeading, Level: 1, Text: para("заголовок")},
			domain.RichBlock{Kind: domain.RichParagraph, Text: para(longText)},
		),
		richMsg(3, domain.RichBlock{
			Kind:     domain.RichTable,
			Bordered: true,
			Rows: [][]domain.RichCell{
				{{Text: para("one two three four five six seven eight")}, {Text: para("B")}},
				{{Text: para("C")}, {Text: para("D longer cell content here")}},
			},
		}),
		richMsg(31, domain.RichBlock{
			Kind: domain.RichTable,
			Rows: [][]domain.RichCell{
				{
					{Text: para("ключ который должен перенестись на следующую строку"), Header: true},
					{Text: para("значение"), Header: true},
				},
				{
					{Text: para("первый")},
					{Text: para("длинное-значение-переносится")},
				},
			},
		}),
		richMsg(4,
			domain.RichBlock{
				Kind: domain.RichList,
				Items: []domain.RichItem{
					{Text: para("первый пункт списка который непременно перенесётся")},
					{Text: para("второй пункт")},
				},
			},
			domain.RichBlock{Kind: domain.RichDivider},
			domain.RichBlock{Kind: domain.RichParagraph, Text: para("всё")},
		),
		richMsg(5,
			domain.RichBlock{Kind: domain.RichBlockquote, Text: para(longText)},
			domain.RichBlock{Kind: domain.RichDetails, Title: para("детали"),
				Blocks: []domain.RichBlock{{Kind: domain.RichParagraph, Text: para("скрытое тело")}}},
		),
		richMsg(6, domain.RichBlock{Kind: domain.RichParagraph, Text: para(longText)}),
	}

	for _, group := range []bool{false, true} {
		for w := 24; w <= 200; w += 3 {
			ml := NewMessageList(30, w)
			ml.isGroup = group
			ml.SetMessages(msgs)
			for i := range ml.items {
				if ml.items[i].kind != itemMessage {
					continue
				}
				est := ml.itemHeight(i)
				got := len(ml.renderItem(i, false))
				if est != got {
					t.Fatalf("group=%v w=%d item %d (msg %d): itemHeight=%d rendered=%d",
						group, w, i, ml.items[i].msg.ID, est, got)
				}
			}
		}
	}
}

func TestFlattenRich_NestedStyles(t *testing.T) {
	text, ents := flattenRich(domain.RichText{
		Kind: domain.RichTextConcat,
		Children: []domain.RichText{
			para("a"),
			{
				Kind: domain.RichTextBold,
				Children: []domain.RichText{
					{
						Kind:     domain.RichTextItalic,
						Children: []domain.RichText{para("b")},
					},
				},
			},
		},
	})
	assert.Equal(t, "ab", text)
	require.Len(t, ents, 2)
	assert.Equal(t, "italic", ents[0].Type)
	assert.Equal(t, 1, ents[0].Offset)
	assert.Equal(t, 1, ents[0].Length)
	assert.Equal(t, "bold", ents[1].Type)
	assert.Equal(t, 1, ents[1].Offset)
	assert.Equal(t, 1, ents[1].Length)
}

func TestFlattenRich_LinkTarget(t *testing.T) {
	text, ents := flattenRich(domain.RichText{
		Kind: domain.RichTextConcat,
		Children: []domain.RichText{
			para("see "),
			{Kind: domain.RichTextURL, URL: "https://example.com/x", Children: []domain.RichText{para("docs")}},
		},
	})
	assert.Equal(t, "see docs", text)
	require.Len(t, ents, 1)
	assert.Equal(t, "text_url", ents[0].Type)
	assert.Equal(t, "https://example.com/x", ents[0].URL)
	assert.Equal(t, 4, ents[0].Offset)
	assert.Equal(t, 4, ents[0].Length)
}

func lipglossWidth(s string) int {
	return len([]rune(xansi.Strip(s)))
}
