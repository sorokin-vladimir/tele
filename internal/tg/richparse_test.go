package tg

import (
	"testing"

	"github.com/gotd/td/tg"
	"github.com/sorokin-vladimir/tele/internal/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestConvertRichMessage_NilAndEmpty(t *testing.T) {
	assert.Nil(t, convertRichMessage(nil))
	assert.Nil(t, convertRichMessage(&tg.RichMessage{}))
	// A message whose blocks all render as nothing (only an anchor) still has
	// content as far as the parser is concerned: the block list is what decides.
	rich := convertRichMessage(&tg.RichMessage{
		Blocks: []tg.PageBlockClass{&tg.PageBlockAnchor{Name: "sec"}},
	})
	require.NotNil(t, rich)
	assert.Len(t, rich.Blocks, 1)
}

func TestConvertRichMessage_ParagraphWithStyles(t *testing.T) {
	blocks := []tg.PageBlockClass{
		&tg.PageBlockParagraph{
			Text: &tg.TextConcat{Texts: []tg.RichTextClass{
				&tg.TextPlain{Text: "Buy "},
				&tg.TextBold{Text: &tg.TextItalic{Text: &tg.TextPlain{Text: "milk"}}},
			}},
		},
		&tg.PageBlockPreformatted{Text: &tg.TextPlain{Text: "a  b"}, Language: "text"},
	}
	rich := convertRichMessage(&tg.RichMessage{Blocks: blocks})
	require.NotNil(t, rich)
	require.Len(t, rich.Blocks, 2)

	p := rich.Blocks[0]
	assert.Equal(t, domain.RichParagraph, p.Kind)
	assert.Equal(t, domain.RichTextConcat, p.Text.Kind)
	require.Len(t, p.Text.Children, 2)
	assert.Equal(t, domain.RichTextPlain, p.Text.Children[0].Kind)
	assert.Equal(t, "Buy ", p.Text.Children[0].Text)
	bold := p.Text.Children[1]
	assert.Equal(t, domain.RichTextBold, bold.Kind)
	require.Len(t, bold.Children, 1)
	assert.Equal(t, domain.RichTextItalic, bold.Children[0].Kind)
	require.Len(t, bold.Children[0].Children, 1)
	assert.Equal(t, "milk", bold.Children[0].Children[0].Text)

	pre := rich.Blocks[1]
	assert.Equal(t, domain.RichPreformatted, pre.Kind)
	assert.Equal(t, "a  b", pre.Text.Plain())
}

func TestConvertRichMessage_LinkAndMention(t *testing.T) {
	blocks := []tg.PageBlockClass{
		&tg.PageBlockParagraph{
			Text: &tg.TextConcat{Texts: []tg.RichTextClass{
				&tg.TextURL{Text: &tg.TextPlain{Text: "docs"}, URL: "https://example.com/doc"},
				&tg.TextPlain{Text: " by "},
				&tg.TextMentionName{Text: &tg.TextPlain{Text: "@ada"}, UserID: 77},
			}},
		},
	}
	rich := convertRichMessage(&tg.RichMessage{Blocks: blocks})
	require.NotNil(t, rich)
	text := rich.Blocks[0].Text
	require.Len(t, text.Children, 3)

	link := text.Children[0]
	assert.Equal(t, domain.RichTextURL, link.Kind)
	assert.Equal(t, "https://example.com/doc", link.URL)
	assert.Equal(t, "docs", link.Plain())

	mention := text.Children[2]
	assert.Equal(t, domain.RichTextMentionName, mention.Kind)
	assert.Equal(t, int64(77), mention.UserID)
	assert.Equal(t, "@ada", mention.Plain())
}

func TestConvertRichMessage_Table(t *testing.T) {
	blocks := []tg.PageBlockClass{
		&tg.PageBlockTable{
			Bordered: true,
			Rows: []tg.PageTableRow{
				{Cells: []tg.PageTableCell{
					{Text: &tg.TextPlain{Text: "Name"}, Header: true},
					{Text: &tg.TextPlain{Text: "Price"}, Header: true},
				}},
				{Cells: []tg.PageTableCell{
					{Text: &tg.TextPlain{Text: "Milk"}},
					{Text: &tg.TextPlain{Text: "1.2"}},
				}},
			},
		},
	}
	rich := convertRichMessage(&tg.RichMessage{Blocks: blocks})
	require.NotNil(t, rich)
	table := rich.Blocks[0]
	assert.Equal(t, domain.RichTable, table.Kind)
	assert.True(t, table.Bordered)
	require.Len(t, table.Rows, 2)
	require.Len(t, table.Rows[0], 2)
	assert.True(t, table.Rows[0][0].Header)
	assert.Equal(t, "Name", table.Rows[0][0].Text.Plain())
	assert.Equal(t, "1.2", table.Rows[1][1].Text.Plain())
}

func TestConvertRichMessage_ListDetailsDivider(t *testing.T) {
	blocks := []tg.PageBlockClass{
		&tg.PageBlockList{Items: []tg.PageListItemClass{
			&tg.PageListItemText{Text: &tg.TextPlain{Text: "one"}},
			&tg.PageListItemText{Text: &tg.TextBold{Text: &tg.TextPlain{Text: "two"}}},
		}},
		&tg.PageBlockDetails{
			Open:  true,
			Title: &tg.TextPlain{Text: "More"},
			Blocks: []tg.PageBlockClass{
				&tg.PageBlockParagraph{Text: &tg.TextPlain{Text: "hidden"}},
			},
		},
		&tg.PageBlockDivider{},
	}
	rich := convertRichMessage(&tg.RichMessage{Blocks: blocks})
	require.NotNil(t, rich)
	require.Len(t, rich.Blocks, 3)

	list := rich.Blocks[0]
	assert.Equal(t, domain.RichList, list.Kind)
	require.Len(t, list.Items, 2)
	assert.Equal(t, "one", list.Items[0].Text.Plain())
	assert.Equal(t, "two", list.Items[1].Text.Plain())

	details := rich.Blocks[1]
	assert.Equal(t, domain.RichDetails, details.Kind)
	assert.Equal(t, "More", details.Title.Plain())
	require.Len(t, details.Blocks, 1)

	assert.Equal(t, domain.RichDivider, rich.Blocks[2].Kind)
}

func TestConvertRichMessage_MediaCaption(t *testing.T) {
	caption := &tg.PageCaption{Text: &tg.TextPlain{Text: "a sunny day"}}
	blocks := []tg.PageBlockClass{
		&tg.PageBlockPhoto{PhotoID: 1, Caption: *caption},
		&tg.PageBlockCollage{
			Items: []tg.PageBlockClass{
				&tg.PageBlockPhoto{PhotoID: 2},
				&tg.PageBlockVideo{VideoID: 3},
			},
		},
	}
	rich := convertRichMessage(&tg.RichMessage{Blocks: blocks})
	require.NotNil(t, rich)
	require.Len(t, rich.Blocks, 2)

	photo := rich.Blocks[0]
	assert.Equal(t, domain.RichPhoto, photo.Kind)
	assert.Equal(t, "a sunny day", photo.Caption.Plain())
	assert.Equal(t, int64(1), photo.PhotoID, "the photo id is kept for the media pipeline")

	collage := rich.Blocks[1]
	assert.Equal(t, domain.RichCollage, collage.Kind)
	require.Len(t, collage.Blocks, 2)
	assert.Equal(t, domain.RichPhoto, collage.Blocks[0].Kind)
	assert.Equal(t, int64(2), collage.Blocks[0].PhotoID)
	assert.Equal(t, domain.RichVideo, collage.Blocks[1].Kind)
	assert.Equal(t, int64(3), collage.Blocks[1].VideoID, "the video id is kept too")
}

func TestConvertRichMessage_AudioKeepsID(t *testing.T) {
	rich := convertRichMessage(&tg.RichMessage{
		Blocks: []tg.PageBlockClass{
			&tg.PageBlockAudio{AudioID: 42},
		},
	})
	require.NotNil(t, rich)
	require.Len(t, rich.Blocks, 1)
	assert.Equal(t, domain.RichAudio, rich.Blocks[0].Kind)
	assert.Equal(t, int64(42), rich.Blocks[0].AudioID)
}

func TestConvertRichMessage_NestedListAndQuoteInList(t *testing.T) {
	rich := convertRichMessage(&tg.RichMessage{
		Blocks: []tg.PageBlockClass{
			&tg.PageBlockList{Items: []tg.PageListItemClass{
				&tg.PageListItemBlocks{Blocks: []tg.PageBlockClass{
					&tg.PageBlockList{Items: []tg.PageListItemClass{
						&tg.PageListItemText{Text: &tg.TextPlain{Text: "nested"}},
					}},
					&tg.PageBlockBlockquote{Text: &tg.TextPlain{Text: "quoted"}},
				}},
			}},
		},
	})
	require.NotNil(t, rich)
	require.Len(t, rich.Blocks, 1)
	require.Len(t, rich.Blocks[0].Items, 1)
	item := rich.Blocks[0].Items[0]
	require.Len(t, item.Blocks, 2)
	assert.Equal(t, domain.RichList, item.Blocks[0].Kind)
	assert.Equal(t, "nested", item.Blocks[0].Items[0].Text.Plain())
	assert.Equal(t, domain.RichBlockquote, item.Blocks[1].Kind)
	assert.Equal(t, "quoted", item.Blocks[1].Text.Plain())
}

func TestConvertRichMessage_AuthorDateIsRawUnix(t *testing.T) {
	rich := convertRichMessage(&tg.RichMessage{
		Blocks: []tg.PageBlockClass{
			&tg.PageBlockAuthorDate{
				Author:        &tg.TextPlain{Text: "Ada"},
				PublishedDate: 1700000000,
			},
		},
	})
	require.NotNil(t, rich)
	require.Len(t, rich.Blocks, 1)
	assert.Equal(t, int64(1700000000), rich.Blocks[0].Date,
		"the converter keeps the instant; the UI owns the formatted date")
}

func TestConvertMessage_AttachesRichMessage(t *testing.T) {
	raw := &tg.Message{
		ID:      7,
		PeerID:  &tg.PeerUser{UserID: 10},
		Date:    1700000000,
		Message: "fallback text",
	}
	raw.SetRichMessage(tg.RichMessage{
		Blocks: []tg.PageBlockClass{
			&tg.PageBlockParagraph{Text: &tg.TextPlain{Text: "structured"}},
		},
	})
	msg, ok := convertMessage(raw, 10)
	require.True(t, ok)
	require.NotNil(t, msg.Rich)
	assert.Len(t, msg.Rich.Blocks, 1)
	// The plain fallback is kept alongside for snippets.
	assert.Equal(t, "fallback text", msg.Text)
}

func TestConvertMessage_PlainMessageHasNoRich(t *testing.T) {
	raw := &tg.Message{
		ID:      7,
		PeerID:  &tg.PeerUser{UserID: 10},
		Date:    1700000000,
		Message: "plain",
	}
	msg, ok := convertMessage(raw, 10)
	require.True(t, ok)
	assert.Nil(t, msg.Rich)
}
