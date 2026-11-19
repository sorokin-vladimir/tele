package components

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"charm.land/lipgloss/v2"
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
	assert.Nil(t, richMessageLines(nil, 40, false))
	assert.Nil(t, richMessageLines(&domain.RichMessage{}, 40, false))
}

func TestRichMessageLines_ParagraphWrapAndHug(t *testing.T) {
	r := richMessageWith(domain.RichBlock{Kind: domain.RichParagraph, Text: para("один два три четыре пять")})
	rows := richMessageLines(r, 10, false)
	require.NotEmpty(t, rows)
	// Wrapped into two rows, not padded to the 10-wide budget.
	assert.Greater(t, len(rows), 1)
	for _, row := range rows {
		assert.LessOrEqual(t, lipglossWidth(row), 10, "paragraph row must fit its content width")
	}
	assert.Equal(t, 10, richMessageWidth(r, 10, false), "a paragraph filling the budget wants full width")
}

func TestRichMessageLines_ShortMessageHugs(t *testing.T) {
	r := richMessageWith(domain.RichBlock{Kind: domain.RichParagraph, Text: para("hi")})
	assert.Equal(t, "hi", stripAll(richMessageLines(r, 200, false))[0])
	assert.Equal(t, 2, richMessageWidth(r, 200, false), "short text hugs its natural width")
}

func TestRichMessageLines_HeadingSubtitleBlockquote(t *testing.T) {
	r := richMessageWith(
		domain.RichBlock{Kind: domain.RichHeading, Level: 1, Text: para("Headline")},
		domain.RichBlock{Kind: domain.RichSubtitle, Text: para("sub")},
		domain.RichBlock{Kind: domain.RichParagraph, Text: para("body")},
		domain.RichBlock{Kind: domain.RichBlockquote, Text: para("quoted")},
	)
	rows := stripAll(richMessageLines(r, 60, false))
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
	rows := stripAll(richMessageLines(r, 40, false))
	assert.Equal(t, "• milk", strings.TrimSpace(rows[0]))
	assert.Equal(t, "• eggs", strings.TrimSpace(rows[1]))

	ol := richMessageWith(domain.RichBlock{
		Kind: domain.RichOrderedList,
		Items: []domain.RichItem{
			{Text: para("first")},
			{Text: para("second")},
		},
	})
	orows := stripAll(richMessageLines(ol, 40, false))
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
	rows := stripAll(richMessageLines(r, 60, false))
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
	rows := richMessageLines(r, 60, false) // raw, ANSI intact
	require.Len(t, rows, 5)                // top rule, header, rule, body, bottom rule
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
	rows := richMessageLines(r, 60, false)
	require.Len(t, rows, 2)
	for _, row := range rows {
		// Split the row at the two-space gap between the unbordered columns and
		// check each side independently: only the flagged first column is bold.
		first, second := splitRowColumns(row)
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
	assert.Equal(t, 9, richMessageWidth(r, 60, false), "a small bordered table hugs its natural width (pads + bars + two 1-wide cells)")
}

func TestRichMessageLines_QuotesFitWidth(t *testing.T) {
	long := strings.Repeat("quoted words ", 8)
	cases := []struct {
		name string
		rich *domain.RichMessage
	}{
		{"plain", richMessageWith(domain.RichBlock{Kind: domain.RichBlockquote, Text: para(long)})},
		{"nested", richMessageWith(domain.RichBlock{
			Kind: domain.RichBlockquote,
			Blocks: []domain.RichBlock{
				{Kind: domain.RichBlockquote, Text: para(long)},
			},
		})},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			for _, w := range []int{1, 2, 3, 10, 24, 80} {
				rows := richMessageLines(tc.rich, w, false)
				require.NotEmpty(t, rows)
				assertRowsFit(t, rows, w)
			}
		})
	}
}

func TestRichMessageLines_QuoteInsideListFits(t *testing.T) {
	long := strings.Repeat("quoted words ", 8)
	r := richMessageWith(domain.RichBlock{
		Kind: domain.RichList,
		Items: []domain.RichItem{
			{Blocks: []domain.RichBlock{
				{Kind: domain.RichBlockquote, Text: para(long)},
			}},
		},
	})
	for _, w := range []int{1, 2, 8, 24, 80} {
		assertRowsFit(t, richMessageLines(r, w, false), w)
	}
}

func TestRichMessageLines_NestedListAndQuoteInList(t *testing.T) {
	r := richMessageWith(domain.RichBlock{
		Kind: domain.RichList,
		Items: []domain.RichItem{
			{Text: para("outer")},
			{Blocks: []domain.RichBlock{
				{Kind: domain.RichList, Items: []domain.RichItem{
					{Text: para("nested")},
				}},
				{Kind: domain.RichBlockquote, Text: para("quoted in a list")},
			}},
		},
	})
	rows := stripAll(richMessageLines(r, 40, false))
	joined := strings.Join(rows, "\n")
	assert.Contains(t, joined, "outer")
	assert.Contains(t, joined, "nested", "a nested list renders its items")
	assert.Contains(t, joined, "quoted in a list", "a quote nested in a list renders")
	assert.Contains(t, joined, "▌", "the quote keeps its glyph")
	assertRowsFit(t, richMessageLines(r, 40, false), 40)
}

func TestRichMessageLines_TablesFitWidth(t *testing.T) {
	rows := make([][]domain.RichCell, 3)
	for ri := range rows {
		for ci := 0; ci < 6; ci++ {
			rows[ri] = append(rows[ri], domain.RichCell{Text: para("column")})
		}
	}
	r := richMessageWith(domain.RichBlock{Kind: domain.RichTable, Bordered: true, Rows: rows})
	for _, w := range []int{5, 10, 20, 24, 60} {
		assertRowsFit(t, richMessageLines(r, w, false), w)
	}
}

func TestDistributeWidths_NeverExceedsBudget(t *testing.T) {
	cases := []struct {
		natural []int
		budget  int
	}{
		{[]int{10, 10, 10}, 20},
		{[]int{5, 1, 1}, 3},
		{[]int{0, 10, 0}, 4},
		{[]int{2, 100}, 50},
		{[]int{1, 1, 1}, 2},
	}
	for _, tc := range cases {
		widths := distributeWidths(tc.natural, tc.budget)
		require.Len(t, widths, len(tc.natural))
		sum := 0
		for i, w := range widths {
			assert.GreaterOrEqual(t, w, 1, "column %d must keep a cell", i)
			assert.LessOrEqual(t, w, max(tc.natural[i], 1), "column %d must not grow past its text", i)
			sum += w
		}
		assert.LessOrEqual(t, sum, max(tc.budget, len(tc.natural)), "widths must stay within the budget")
	}
}

// assertRowsFit checks the property the bubble relies on: no row of a rich
// block may be wider than the content width it was laid out for.
func assertRowsFit(t *testing.T, rows []string, w int) {
	t.Helper()
	for _, row := range rows {
		assert.LessOrEqual(t, lipglossWidth(row), w, "row %q exceeds width %d", xansi.Strip(row), w)
	}
}

func TestRichMessageLines_DetailsCollapsedAndExpanded(t *testing.T) {
	r := richMessageWith(domain.RichBlock{
		Kind:  domain.RichDetails,
		Title: para("More"),
		Blocks: []domain.RichBlock{
			{Kind: domain.RichParagraph, Text: para("the hidden body")},
		},
	})
	collapsed := strings.Join(stripAll(richMessageLines(r, 40, false)), "\n")
	assert.Contains(t, collapsed, "▸ More")
	assert.NotContains(t, collapsed, "the hidden body")

	expanded := strings.Join(stripAll(richMessageLines(r, 40, true)), "\n")
	assert.Contains(t, expanded, "▾ More")
	assert.Contains(t, expanded, "the hidden body")
}

func TestMessageList_ToggleDetailsUpdatesHeightsAndRender(t *testing.T) {
	msg := domain.Message{
		ID:   1,
		Date: time.Now(),
		Rich: richMessageWith(domain.RichBlock{
			Kind:  domain.RichDetails,
			Title: para("Summary"),
			Blocks: []domain.RichBlock{
				{Kind: domain.RichParagraph, Text: para("hidden secret inside details")},
			},
		}),
	}
	ml := NewMessageList(20, 60)
	ml.SetMessages([]domain.Message{msg})

	assert.False(t, ml.DetailsExpanded())
	collapsedView := xansi.Strip(ml.View())
	assert.Contains(t, collapsedView, "▸ Summary")
	assert.NotContains(t, collapsedView, "hidden secret inside details")
	assert.Equal(t, ml.itemHeight(0), len(ml.renderItem(0, false)))

	assert.True(t, ml.ToggleDetails())
	assert.True(t, ml.DetailsExpanded())
	expandedView := xansi.Strip(ml.View())
	assert.Contains(t, expandedView, "▾ Summary")
	assert.Contains(t, expandedView, "hidden secret inside details")
	assert.Equal(t, ml.itemHeight(0), len(ml.renderItem(0, false)))

	assert.False(t, ml.ToggleDetails())
	assert.NotContains(t, xansi.Strip(ml.View()), "hidden secret inside details")
}

func TestRichMessageLines_MediaAndDivider(t *testing.T) {
	r := richMessageWith(
		domain.RichBlock{Kind: domain.RichPhoto, Caption: para("sunset")},
		domain.RichBlock{Kind: domain.RichVideo},
		domain.RichBlock{Kind: domain.RichDivider},
	)
	rows := stripAll(richMessageLines(r, 40, false))
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

	for _, expanded := range []bool{false, true} {
		for _, group := range []bool{false, true} {
			for w := 24; w <= 200; w += 3 {
				ml := NewMessageList(30, w)
				ml.isGroup = group
				ml.SetDetailsExpanded(expanded)
				ml.SetMessages(msgs)
				for i := range ml.items {
					if ml.items[i].kind != itemMessage {
						continue
					}
					est := ml.itemHeight(i)
					got := len(ml.renderItem(i, false))
					if est != got {
						t.Fatalf("expanded=%v group=%v w=%d item %d (msg %d): itemHeight=%d rendered=%d",
							expanded, group, w, i, ml.items[i].msg.ID, est, got)
					}
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

func TestRichItemHeight_AllBlockKindsAndWidthParity(t *testing.T) {
	now := time.Now()
	longToken := strings.Repeat("verylongunbreakableidentifier/", 4) + "end"
	linkedText := domain.RichText{
		Kind: domain.RichTextConcat,
		Children: []domain.RichText{
			para("см. "),
			{Kind: domain.RichTextURL, URL: "https://example.com/very/long/url/with/query?foo=bar&baz=qux", Children: []domain.RichText{
				{Kind: domain.RichTextBold, Children: []domain.RichText{para("длинная ссылка на документацию ⚙️ и ещё слова")}},
			}},
			para(" и "),
			{Kind: domain.RichTextFixed, Children: []domain.RichText{para("inline_code_with_underscores_and_⚙️_icon")}},
			para("\nвторой абзац с эмодзи ✅ и пробелами   "),
		},
	}

	msgs := []domain.Message{
		{ID: 1, Date: now, Text: "anchor"},
		{
			ID: 2, Date: now, ReplyToMsgID: 1, Forward: &domain.ForwardInfo{From: "Anton"},
			SenderName: "Very Long Bot Display Name Here",
			Reactions:  []domain.Reaction{{Emoji: "👍", Count: 5}, {Emoji: "⚙️", Count: 1}},
			Rich: richMessageWith(
				domain.RichBlock{Kind: domain.RichAuthorDate, Author: para("Автор Статьи"), Date: now.Unix()},
				domain.RichBlock{Kind: domain.RichAuthorDate, Date: now.Unix()},
				domain.RichBlock{Kind: domain.RichHeading, Level: 1, Text: para("Заголовок первого уровня ⚙️")},
				domain.RichBlock{Kind: domain.RichSubtitle, Text: linkedText},
				domain.RichBlock{Kind: domain.RichParagraph, Text: linkedText},
				domain.RichBlock{Kind: domain.RichPreformatted, Text: para("func main() {\n\tif ok {\n\t\tfmt.Println(\"" + longToken + "\")\n\t}\n}")},
				domain.RichBlock{Kind: domain.RichBlockquote, Caption: para("Источник"), Blocks: []domain.RichBlock{
					{Kind: domain.RichParagraph, Text: linkedText},
					{Kind: domain.RichBlockquote, Text: para("вложенная цитата: " + longToken)},
				}},
				domain.RichBlock{Kind: domain.RichPullquote, Text: para("Большая вынесенная цитата для проверки переноса"), Caption: para("Автор")},
				domain.RichBlock{Kind: domain.RichList, Items: []domain.RichItem{
					{Blocks: []domain.RichBlock{
						{Kind: domain.RichParagraph, Text: linkedText},
						{Kind: domain.RichDetails, Title: para("Вложенные детали"), Blocks: []domain.RichBlock{
							{Kind: domain.RichPreformatted, Text: para("line1\n\tindented_line_longer_than_usual_buffer_width")},
						}},
					}},
				}},
				domain.RichBlock{Kind: domain.RichDetails, Title: para("Длинный заголовок спойлера который может переноситься"), Blocks: []domain.RichBlock{
					{Kind: domain.RichParagraph, Text: para(strings.Repeat("слово ", 40))},
				}},
				domain.RichBlock{Kind: domain.RichTable, Bordered: true, Rows: [][]domain.RichCell{
					{{Text: para("Col1"), Header: true}, {Text: para("Col2"), Header: true}, {Text: para("Col3"), Header: true}},
					{{Text: para("⚙️ one two three four")}, {Text: para(longToken)}, {Text: linkedText}},
				}},
				domain.RichBlock{Kind: domain.RichRelatedArticles, Title: para("Связанные"), Articles: []domain.RichArticle{
					{Title: "Очень длинный заголовок связанной статьи", URL: "https://example.com/article/1"},
				}},
				domain.RichBlock{Kind: domain.RichPhoto, Caption: linkedText},
				domain.RichBlock{Kind: domain.RichCollage, Caption: para("альбом"), Blocks: []domain.RichBlock{
					{Kind: domain.RichPhoto}, {Kind: domain.RichVideo},
				}},
				domain.RichBlock{Kind: domain.RichChannel, ChannelTitle: "Anton Channel"},
				domain.RichBlock{Kind: domain.RichMath, Text: para("x^2 + y^2 = z^2")},
				domain.RichBlock{Kind: domain.RichDivider},
				domain.RichBlock{Kind: domain.RichFooter, Text: para("Подвал сообщения")},
				domain.RichBlock{Kind: domain.RichUnsupported},
			),
		},
		{ID: 3, Date: now, IsOut: true, Rich: richMessageWith(
			domain.RichBlock{Kind: domain.RichParagraph, Text: para(strings.Repeat("строка ", 60))},
		)},
	}

	for _, expanded := range []bool{false, true} {
		for _, group := range []bool{false, true} {
			for w := 24; w <= 160; w += 7 {
				ml := NewMessageList(30, w)
				ml.isGroup = group
				ml.SetShowIndicator(true)
				ml.SetDetailsExpanded(expanded)
				ml.SetMessages(msgs)
				for i := range ml.items {
					est := ml.itemHeight(i)
					lines := ml.renderItem(i, true)
					if est != len(lines) {
						t.Fatalf("expanded=%v group=%v w=%d item %d: itemHeight=%d rendered=%d", expanded, group, w, i, est, len(lines))
					}
					for li, line := range lines {
						assert.NotContains(t, line, "\n", "expanded=%v group=%v w=%d item=%d line=%d has newline", expanded, group, w, i, li)
						if w >= 40 {
							assert.LessOrEqual(t, lipgloss.Width(line), w, "expanded=%v group=%v w=%d item=%d line=%d", expanded, group, w, i, li)
							assert.LessOrEqual(t, xansi.StringWidth(line), w, "expanded=%v group=%v w=%d item=%d line=%d", expanded, group, w, i, li)
						}
					}
				}
			}
		}
	}
}

func TestRichMessage_ScrollUpBoundaryDoesNotJumpViewport(t *testing.T) {
	now := time.Now()
	var blocks []domain.RichBlock
	for i := 0; i < 40; i++ {
		blocks = append(blocks, domain.RichBlock{
			Kind: domain.RichParagraph,
			Text: para(strings.Repeat("текст ", 8)),
		})
	}
	msgs := []domain.Message{
		{ID: 1, Date: now, Rich: richMessageWith(blocks...)}, // tall rich message, h > viewHeight
		{ID: 2, Date: now, Text: "TAIL-ONE"},
	}
	for id := 3; id <= 15; id++ {
		msgs = append(msgs, domain.Message{ID: id, Date: now, Text: fmt.Sprintf("TAIL-%d", id)})
	}
	ml := NewMessageList(20, 80)
	ml.SetMessages(msgs)

	// Scroll up step by step until item 2 (msg 2 = TAIL-ONE) sits at row 0 of the viewport.
	for ml.ViewStart() > 2 || (ml.ViewStart() == 2 && ml.LineOffset() > 0) {
		ml.ScrollUp()
	}
	require.Equal(t, 2, ml.ViewStart())
	require.Equal(t, 0, ml.LineOffset())
	before := strings.Split(xansi.Strip(ml.View()), "\n")
	require.Equal(t, 20, len(before))

	// A single ScrollUp across the tall rich message boundary must only shift
	// visible lines down smoothly by 2 rows (last content line + bottom border),
	// leaving TAIL-ONE visible near the top rather than jumping an entire viewport
	// height upward.
	ml.ScrollUp()
	assert.Equal(t, 1, ml.ViewStart())
	assert.Equal(t, ml.itemHeight(1)-2, ml.LineOffset())
	after := strings.Split(xansi.Strip(ml.View()), "\n")
	require.Equal(t, 20, len(after))
	assert.Equal(t, before[0], after[2], "existing viewport content shifts down smoothly by 2 lines")
	assert.Contains(t, after[2], "╭")

	// Scrolling back down one step reverses cleanly across the boundary.
	ml.ScrollDown()
	assert.Equal(t, 2, ml.ViewStart())
	assert.Equal(t, 0, ml.LineOffset())
}

func TestRichMessage_PrependAndScrollUpSmoothly(t *testing.T) {
	now := time.Now()
	tail := make([]domain.Message, 0, 12)
	for id := 10; id <= 21; id++ {
		tail = append(tail, domain.Message{ID: id, Date: now, Text: fmt.Sprintf("NEW-%d", id)})
	}
	ml := NewMessageList(15, 80)
	ml.SetMessages(tail)
	// Scroll up so item 1 (msg 10 = NEW-10) is at the top edge.
	for ml.ViewStart() > 1 || (ml.ViewStart() == 1 && ml.LineOffset() > 0) {
		ml.ScrollUp()
	}
	before := strings.Split(xansi.Strip(ml.View()), "\n")
	require.Contains(t, before[0], "╭")
	require.Contains(t, before[1], "NEW-10")

	var blocks []domain.RichBlock
	for i := 0; i < 30; i++ {
		blocks = append(blocks, domain.RichBlock{Kind: domain.RichParagraph, Text: para(strings.Repeat("слово ", 8))})
	}
	ml.PrependMessages([]domain.Message{
		{ID: 5, Date: now, Rich: richMessageWith(blocks...)},
	})
	// Prepending older history keeps the current frame untouched.
	assert.Equal(t, before, strings.Split(xansi.Strip(ml.View()), "\n"))

	// Scrolling up into the newly prepended tall rich message reveals only its bottom edge.
	ml.ScrollUp()
	after := strings.Split(xansi.Strip(ml.View()), "\n")
	assert.Equal(t, before[0], after[2])
	assert.Equal(t, before[1], after[3])
}

func lipglossWidth(s string) int {
	return len([]rune(xansi.Strip(s)))
}
