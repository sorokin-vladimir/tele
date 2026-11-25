package protocol_test

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sorokin-vladimir/tele/internal/core"
	"github.com/sorokin-vladimir/tele/internal/core/project"
	"github.com/sorokin-vladimir/tele/internal/ipc/protocol"
	"github.com/sorokin-vladimir/tele/internal/telerr"
)

// A window crosses under the name the protocol gives it and comes back as the
// window it was. The names are the protocol: a client built from another
// commit reads them.
func TestWindows_EachCrossesUnderItsName(t *testing.T) {
	cases := map[string]project.Window{
		"chat_list":  project.ChatListWindow{Folder: 3, Offset: 20, Limit: 40},
		"topic_list": project.TopicListWindow{ChatID: -1001, Offset: 5, Limit: 10},
		"history": project.HistoryWindow{
			ChatID: -1001, TopicID: 7,
			Anchor: project.Anchor{Kind: project.AnchorMessage, MsgID: 42},
			Before: 30, After: 30,
		},
		"account": project.AccountWindow{},
	}
	for want, w := range cases {
		t.Run(want, func(t *testing.T) {
			name, raw, err := protocol.Windows.Encode(w)
			require.NoError(t, err)
			assert.Equal(t, want, name)

			got, err := protocol.Windows.Decode(name, raw)
			require.NoError(t, err)
			assert.Equal(t, w, got)
		})
	}
}

// Every window and every event the code declares has a name. A family is
// whatever carries its marker method, found in the source rather than listed
// here, so a new member without a name fails this test instead of the first
// connection that tries to send one.
func TestFamilies_EveryMemberHasAName(t *testing.T) {
	families := []struct {
		dir, marker string
		members     []reflect.Type
	}{
		{"../../core/project", "isWindow", protocol.Windows.Members()},
		{"../../core", "isEvent", protocol.Events.Members()},
	}
	for _, f := range families {
		t.Run(f.marker, func(t *testing.T) {
			declared := typesWithMethod(t, f.dir, f.marker)
			require.NotEmpty(t, declared, "no %s in %s: the marker moved", f.marker, f.dir)

			var named []string
			for _, m := range f.members {
				named = append(named, m.Name())
			}
			assert.ElementsMatch(t, declared, named)
		})
	}
}

// typesWithMethod lists the types in dir's package that declare method.
func typesWithMethod(t *testing.T, dir, method string) []string {
	t.Helper()
	paths, err := filepath.Glob(filepath.Join(dir, "*.go"))
	require.NoError(t, err)
	fset := token.NewFileSet()
	var types []string
	for _, path := range paths {
		if strings.HasSuffix(path, "_test.go") {
			continue
		}
		file, err := parser.ParseFile(fset, path, nil, 0)
		require.NoError(t, err)
		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Recv == nil || fn.Name.Name != method {
				continue
			}
			recv := fn.Recv.List[0].Type
			if star, ok := recv.(*ast.StarExpr); ok {
				recv = star.X
			}
			types = append(types, recv.(*ast.Ident).Name)
		}
	}
	return types
}

// An event crosses the same way, errors included: what a client is told about
// a failure is the domain error, which is a value.
func TestEvents_EachCrossesUnderItsName(t *testing.T) {
	cases := map[string]core.Event{
		"incoming":          core.Incoming{ChatID: -1001, TopicID: 7},
		"typing":            core.Typing{ChatID: -1001, TopicID: 7, Label: "Ann is typing"},
		"progress":          core.Progress{ChatID: 5, Ref: "r1", Part: 2, Parts: 3, Done: 100, Total: 300},
		"download_progress": core.DownloadProgress{Ref: "d1", Done: 10, Total: 20},
		"failure": core.Failure{ChatID: 5, Op: core.OpSend, Err: &telerr.Error{
			Kind: telerr.Rejected, Op: "messages.sendMessage", Detail: "MESSAGE_TOO_LONG",
		}},
		"notification": core.Notification{
			ChatID: -1001, TopicID: 7, Title: "Club / Rules",
			ChatTitle: "Club", TopicTitle: "Rules", Body: "hello",
		},
		"account_ended": core.AccountEnded{Reason: core.EndLoggedOutElsewhere, Discarded: 2},
		"message_gone":  core.MessageGone{ChatID: -1001, TopicID: 7, MsgID: 500, Nearest: 499},
		"host_failure": core.HostFailure{Err: &telerr.Error{
			Kind: telerr.Internal, Detail: "the account that ended could not be removed",
		}},
	}
	for want, ev := range cases {
		t.Run(want, func(t *testing.T) {
			name, raw, err := protocol.Events.Encode(ev)
			require.NoError(t, err)
			assert.Equal(t, want, name)

			got, err := protocol.Events.Decode(name, raw)
			require.NoError(t, err)
			assert.Equal(t, ev, got)
		})
	}
}
