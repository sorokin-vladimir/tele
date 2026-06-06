package keys

import "strings"

// MatchResult classifies the outcome of feeding a key to the Matcher.
type MatchResult int

const (
	// MatchNone means the key (with any pending prefix) matched no binding and
	// is not the prefix of a chord. The pending prefix is reset.
	MatchNone MatchResult = iota
	// MatchPending means the accumulated keys form the prefix of a chord; the
	// Matcher is waiting for the next key.
	MatchPending
	// MatchAction means a terminal binding matched; the Action is returned.
	MatchAction
)

// Matcher resolves keystrokes against a KeyMap, supporting multi-key chords
// encoded as space-separated key tokens (e.g. "g g"). It tracks the pending
// chord prefix across calls. Not safe for concurrent use.
type Matcher struct {
	km       KeyMap
	prefixes map[Context]map[string]struct{}
	pending  string
}

// NewMatcher builds a Matcher from km, precomputing the set of live chord
// prefixes per context so each keystroke resolves in O(1).
func NewMatcher(km KeyMap) *Matcher {
	prefixes := make(map[Context]map[string]struct{})
	for ctx, bindings := range km {
		set := make(map[string]struct{})
		for seq := range bindings {
			tokens := strings.Split(seq, " ")
			// Every proper prefix of a multi-token chord is a live prefix.
			for i := 1; i < len(tokens); i++ {
				set[strings.Join(tokens[:i], " ")] = struct{}{}
			}
		}
		if len(set) > 0 {
			prefixes[ctx] = set
		}
	}
	return &Matcher{km: km, prefixes: prefixes}
}

// Reset clears any pending chord prefix.
func (m *Matcher) Reset() { m.pending = "" }

// Resolve feeds key (a bubbletea key token like "j" or "ctrl+d") for context
// ctx. It returns the resolved Action and a MatchResult. On MatchPending the
// Action is ActionNone and the caller should take no action and await the next
// key.
func (m *Matcher) Resolve(ctx Context, key string) (Action, MatchResult) {
	seq := key
	if m.pending != "" {
		seq = m.pending + " " + key
	}

	if action := m.lookup(ctx, seq); action != ActionNone {
		m.pending = ""
		return action, MatchAction
	}

	if m.isPrefix(ctx, seq) {
		m.pending = seq
		return ActionNone, MatchPending
	}

	// Dead end. If we were mid-chord, reset and retry the key on its own so a
	// fresh single key (or a new chord start) still works.
	if m.pending != "" {
		m.pending = ""
		return m.Resolve(ctx, key)
	}
	return ActionNone, MatchNone
}

func (m *Matcher) lookup(ctx Context, seq string) Action {
	if bindings, ok := m.km[ctx]; ok {
		if a, ok := bindings[seq]; ok {
			return a
		}
	}
	if g, ok := m.km[ContextGlobal]; ok {
		if a, ok := g[seq]; ok {
			return a
		}
	}
	return ActionNone
}

func (m *Matcher) isPrefix(ctx Context, seq string) bool {
	if set, ok := m.prefixes[ctx]; ok {
		if _, ok := set[seq]; ok {
			return true
		}
	}
	if set, ok := m.prefixes[ContextGlobal]; ok {
		if _, ok := set[seq]; ok {
			return true
		}
	}
	return false
}
