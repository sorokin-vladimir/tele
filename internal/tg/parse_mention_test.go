package tg

import (
	"testing"

	"github.com/gotd/td/tg"
)

func TestConvertEntitiesMentionName(t *testing.T) {
	in := []tg.MessageEntityClass{
		&tg.MessageEntityMentionName{Offset: 2, Length: 4, UserID: 555},
	}
	out := convertEntities(in)
	if len(out) != 1 {
		t.Fatalf("want 1 entity, got %d", len(out))
	}
	e := out[0]
	if e.Type != "mention_name" || e.Offset != 2 || e.Length != 4 || e.UserID != 555 {
		t.Fatalf("unexpected entity: %+v", e)
	}
}
