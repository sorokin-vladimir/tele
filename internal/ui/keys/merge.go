package keys

import (
	"fmt"
	"sort"
	"strings"
)

// KnownActions returns the set of Action values that have runtime handlers and
// can therefore be bound from config. Excludes ActionNone and the unhandled
// ActionLeft/ActionRight.
func KnownActions() map[Action]bool {
	return map[Action]bool{
		// Pane focus / app (global).
		ActionFocusFolders: true, ActionFocusChatList: true, ActionFocusChat: true,
		ActionFocusPrev: true, ActionFocusNext: true, ActionQuit: true,
		ActionShowHelp: true,
		// Navigation / scrolling.
		ActionUp: true, ActionDown: true, ActionGoTop: true, ActionGoBottom: true,
		ActionScrollHalfDown: true, ActionScrollHalfUp: true,
		ActionCursorUp: true, ActionCursorDown: true,
		// Chat / editing.
		ActionInsert: true, ActionNormal: true, ActionConfirm: true,
		ActionSearch: true, ActionOpenInViewer: true, ActionOpenExternal: true, ActionOpenContextMenu: true,
		ActionCancel: true, ActionReply: true, ActionReact: true, ActionEdit: true,
		ActionForward: true,
		ActionDelete:  true, ActionDeleteRevoke: true, ActionDeleteMe: true,
		ActionJumpToOriginal: true, ActionPlayVoice: true, ActionDownloadFile: true,
		ActionCopyMessage: true,
		// Media attach / send.
		ActionAttach: true, ActionToggleSendAs: true, ActionCancelUpload: true,
		ActionPasteImage: true,
	}
}

// KnownContexts returns the set of valid Context values.
func KnownContexts() map[Context]bool {
	return map[Context]bool{
		ContextGlobal: true, ContextFolders: true, ContextChatList: true,
		ContextChat: true, ContextComposer: true, ContextSearch: true,
		ContextContextMenu: true, ContextDeleteSubMenu: true,
		ContextFilePicker: true,
	}
}

// MergeOverrides applies action-centric overrides onto a copy of base. For each
// (context, action) the given keys REPLACE all of that action's existing keys
// in that context; unmentioned actions keep their defaults. base is never
// mutated. Returns the merged map and human-readable warnings. Processing is
// deterministic (contexts and actions are applied in sorted name order).
func MergeOverrides(base KeyMap, overrides map[string]map[string][]string) (KeyMap, []string) {
	merged := make(KeyMap, len(base))
	for ctx, bindings := range base {
		m := make(map[string]Action, len(bindings))
		for k, a := range bindings {
			m[k] = a
		}
		merged[ctx] = m
	}

	var warns []string
	knownActions := KnownActions()
	knownContexts := KnownContexts()
	touched := map[Context]bool{}

	for _, ctxName := range sortedKeys(overrides) {
		ctx := Context(ctxName)
		if !knownContexts[ctx] {
			warns = append(warns, fmt.Sprintf("unknown context %q (skipped)", ctxName))
			continue
		}
		if merged[ctx] == nil {
			merged[ctx] = map[string]Action{}
		}
		actions := overrides[ctxName]
		for _, actionName := range sortedKeys(actions) {
			action := Action(actionName)
			if !knownActions[action] {
				warns = append(warns, fmt.Sprintf("unknown action %q in context %q (skipped)", actionName, ctxName))
				continue
			}
			// Replace: drop existing keys for this action.
			for k, a := range merged[ctx] {
				if a == action {
					delete(merged[ctx], k)
				}
			}
			for _, raw := range actions[actionName] {
				key := strings.TrimSpace(raw)
				if key == "" {
					warns = append(warns, fmt.Sprintf("empty key for action %q in context %q (skipped)", actionName, ctxName))
					continue
				}
				if prev, ok := merged[ctx][key]; ok && prev != action {
					warns = append(warns, fmt.Sprintf("key %q in context %q reassigned %q -> %q", key, ctxName, prev, action))
				}
				merged[ctx][key] = action
			}
			touched[ctx] = true
		}
	}

	for _, ctx := range sortedContexts(touched) {
		warns = append(warns, chordPrefixConflicts(ctx, merged[ctx])...)
	}
	return merged, warns
}

// chordPrefixConflicts reports chords whose first token is also bound as a
// standalone key in the same context (which makes the chord unreachable).
func chordPrefixConflicts(ctx Context, bindings map[string]Action) []string {
	var warns []string
	singles := map[string]bool{}
	for k := range bindings {
		if !strings.Contains(k, " ") {
			singles[k] = true
		}
	}
	var chords []string
	for k := range bindings {
		if strings.Contains(k, " ") {
			chords = append(chords, k)
		}
	}
	sort.Strings(chords)
	for _, chord := range chords {
		first := strings.SplitN(chord, " ", 2)[0]
		if singles[first] {
			warns = append(warns, fmt.Sprintf("chord %q in context %q is unreachable: %q is also bound as a single key", chord, ctx, first))
		}
	}
	return warns
}

func sortedKeys[V any](m map[string]V) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func sortedContexts(m map[Context]bool) []Context {
	out := make([]Context, 0, len(m))
	for c := range m {
		out = append(out, c)
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}
