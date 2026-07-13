package tg

import (
	"testing"

	"github.com/gotd/td/tg"

	"github.com/sorokin-vladimir/tele/internal/store"
)

func TestConvertToTGEntitiesNameMention(t *testing.T) {
	es := []store.MessageEntity{
		{Type: "mention_name", Offset: 0, Length: 5, UserID: 10, AccessHash: 99},
		{Type: "mention", Offset: 6, Length: 4}, // username: no entity emitted
	}
	out := convertToTGEntities(es)
	if len(out) != 1 {
		t.Fatalf("want 1 tg entity, got %d", len(out))
	}
	mn, ok := out[0].(*tg.InputMessageEntityMentionName)
	if !ok {
		t.Fatalf("want *tg.InputMessageEntityMentionName, got %T", out[0])
	}
	iu, ok := mn.UserID.(*tg.InputUser)
	if !ok {
		t.Fatalf("want *tg.InputUser, got %T", mn.UserID)
	}
	if mn.Offset != 0 || mn.Length != 5 || iu.UserID != 10 || iu.AccessHash != 99 {
		t.Fatalf("unexpected: %+v / %+v", mn, iu)
	}
}

func TestBuildSendRequestSetsEntities(t *testing.T) {
	es := []store.MessageEntity{{Type: "mention_name", Offset: 0, Length: 3, UserID: 1, AccessHash: 2}}
	req := buildSendRequest(&tg.InputPeerEmpty{}, "abc", 7, 0, es)
	if len(req.Entities) != 1 {
		t.Fatalf("want 1 entity in request, got %d", len(req.Entities))
	}
}

func TestBuildSendRequestNoEntities(t *testing.T) {
	req := buildSendRequest(&tg.InputPeerEmpty{}, "hi", 7, 0, nil)
	if len(req.Entities) != 0 {
		t.Fatalf("want 0 entities, got %d", len(req.Entities))
	}
}
