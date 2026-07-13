package tg

import (
	"testing"

	"github.com/gotd/td/tg"
)

func TestBuildChatMembers(t *testing.T) {
	users := []tg.UserClass{
		&tg.User{ID: 1, AccessHash: 11, FirstName: "Alice", LastName: "A", Username: "alice"},
		&tg.User{ID: 2, AccessHash: 22, FirstName: "Bob"},
		&tg.User{ID: 3, Deleted: true},
		&tg.User{ID: 4, Bot: true, FirstName: "Botty"},
	}
	got := buildChatMembers(users)
	if len(got) != 2 {
		t.Fatalf("want 2 members (deleted+bot skipped), got %d: %+v", len(got), got)
	}
	if got[0].UserID != 1 || got[0].Username != "alice" || got[0].DisplayName != "Alice A" || got[0].AccessHash != 11 {
		t.Fatalf("unexpected first member: %+v", got[0])
	}
	if got[1].DisplayName != "Bob" || got[1].Username != "" {
		t.Fatalf("unexpected second member: %+v", got[1])
	}
}
