package protocol_test

import (
	"reflect"
	"slices"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/sorokin-vladimir/tele/internal/core"
	"github.com/sorokin-vladimir/tele/internal/ipc/protocol"
)

// Every method of the surface crosses a connection, and nothing else does. The
// calls are written out rather than read off the interface (reading them off
// it doubles the binary, #208), so this is what keeps the two in step: a
// method added to the surface and not to the calls fails here, not on a
// connection.
func TestCalls_AreEveryMethodOfTheSurface(t *testing.T) {
	surface := reflect.TypeFor[core.Surface]()
	var want []string
	for i := range surface.NumMethod() {
		want = append(want, surface.Method(i).Name)
	}
	var got []string
	for name := range protocol.Calls {
		got = append(got, name)
	}
	slices.Sort(want)
	slices.Sort(got)

	assert.Equal(t, want, got)
}

// Every call is a read or a write, never neither: cli.account_commands set to
// read lets a command make the one and not the other, so a call nobody
// placed would slip through on whichever side the host guessed (#181).
func TestCalls_EveryOneIsAReadOrAWrite(t *testing.T) {
	for name := range protocol.Calls {
		_, read := protocol.Reads[name]
		_, write := protocol.Writes[name]
		assert.True(t, read != write, "%s is placed as a read %v and as a write %v", name, read, write)
	}
	assert.Contains(t, protocol.Writes, "Send")
	assert.Contains(t, protocol.Writes, "MarkRead")
	assert.Contains(t, protocol.Reads, "History")
}
