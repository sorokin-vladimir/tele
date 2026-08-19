// Package settings describes what a setting is, independently of where its
// value is kept.
//
// A setting belongs to a store: the config file, which is this machine's, or
// the Telegram account, which is the same from every client (ADR 0010). This
// package knows about neither. It holds the shape both stores share - what a
// setting is called, how it is edited, what it will accept, when a change to it
// takes hold, and how far a change has got - so that a store can declare its
// settings without the overlay learning where they live.
//
// Nothing here imports YAML, Telegram or the UI, and nothing here should.
package settings

import (
	"fmt"
	"slices"
	"strings"
)

// Widget is how a setting is edited. It is a property of the setting rather
// than of the store: a privacy rule chosen from a fixed set is the same choice
// as a toast zone chosen from a fixed set, whichever side of the wire it lives
// on.
//
// The Go type cannot stand in for this. photos.max_long_side_px and
// photos.disk_cache_size are both integers and are edited differently, which is
// the reason a setting is declared rather than discovered (ADR 0008).
type Widget uint8

const (
	// Toggle is on or off.
	Toggle Widget = iota
	// Choice is one of a fixed set of named values.
	Choice
	// Number is a count, bounded by Min and Max.
	Number
	// Bytes is a size held in bytes and shown in the unit a person thinks in.
	Bytes
	// Text is free text. It carries no type constraint, because the one setting
	// that uses it - ui.theme - is legitimately either a name or a map of
	// names, and the config loader is what judges it.
	Text
)

func (w Widget) String() string {
	switch w {
	case Toggle:
		return "toggle"
	case Choice:
		return "choice"
	case Number:
		return "number"
	case Bytes:
		return "bytes"
	case Text:
		return "text"
	}
	return fmt.Sprintf("widget(%d)", uint8(w))
}

// Applicability is when a change to a setting takes hold. Three states rather
// than two: a setting that needs no restart is not necessarily one whose effect
// is visible at once, and telling a person to restart when they need not is as
// wrong as letting them think nothing happened.
//
// It describes settings kept in a file. A setting kept on the account takes
// hold when the server confirms, which is a different question.
type Applicability uint8

const (
	// Immediate takes hold the moment it is written.
	Immediate Applicability = iota
	// NextUse takes hold the next time the app does the thing it governs. What
	// is already on screen was drawn under the old value and stays that way,
	// which is not a failure to apply.
	NextUse
	// Startup is read once, when the app starts, and not read again. A change
	// is written and kept; the running app goes on with the value it started
	// with.
	Startup
)

func (a Applicability) String() string {
	switch a {
	case Immediate:
		return "immediate"
	case NextUse:
		return "next-use"
	case Startup:
		return "startup"
	}
	return fmt.Sprintf("applicability(%d)", uint8(a))
}

// Status is how far a change to a setting has got. A file store collapses it -
// a write finishes or fails before anyone can look - and an account store does
// not, because a write there is a request that can be outstanding.
type Status uint8

const (
	// Saved means the store holds the value being shown. It is where a file
	// setting always is unless a write just failed.
	Saved Status = iota
	// Unknown means the value has not arrived yet. A working state rather than
	// a failure, the way a partial profile is.
	Unknown
	// Saving means a change has been sent and nothing has come back.
	Saving
	// Failed means the change did not take and will not be retried on its own.
	// A value Telegram understood and refused is one cause of this, and it is
	// the cause that can say what was wrong.
	Failed
)

func (s Status) String() string {
	switch s {
	case Saved:
		return "saved"
	case Unknown:
		return "unknown"
	case Saving:
		return "saving"
	case Failed:
		return "failed"
	}
	return fmt.Sprintf("status(%d)", uint8(s))
}

// Entry declares one setting. Everything the overlay needs to show and edit a
// setting is here, and nothing about where the value is kept.
type Entry struct {
	// Key addresses the setting inside its store. For the config file it is the
	// dotted path of the YAML key, so a row on screen is findable in the file by
	// the same path.
	Key string
	// Group is the heading the setting is shown under. File settings use their
	// section in the file; settings with no file to be in name their own group.
	Group string
	// Label is what the setting is called on screen.
	Label string
	// Help is a sentence or two saying what the setting does. It is prose
	// rather than a caption, which is why it is a field and not a struct tag.
	Help string
	// Widget is how the value is edited.
	Widget Widget
	// Applies is when a change takes hold.
	Applies Applicability
	// ReadOnly marks a setting shown but not changed from here, because
	// changing it is not editing a value. Independent of Applies: a startup
	// setting is editable, it just does not take hold until the next start.
	ReadOnly bool
	// Secret marks a value that must not be drawn in full. Credentials are
	// read-only and still end up on someone's screen recording.
	Secret bool
	// Unit is what a number counts, shown beside it: "px", "messages".
	Unit string
	// Choices are the values a Choice accepts, in the order they are offered.
	Choices []string
	// Min and Max bound a Number or Bytes. Max at or below Min means no upper
	// bound, which is what a cache size wants.
	Min, Max int64
}

// Validate reports whether v is a value this setting will accept. It is the one
// statement of legality: the widget refuses on it, and the config loader checks
// the file against it, so a value typed into the overlay and the same value
// written into the file by hand are judged the same way (ADR 0008).
//
// A nil value is absence, and absence is legal - it means the setting takes its
// default.
func (e Entry) Validate(v any) error {
	if v == nil {
		return nil
	}
	switch e.Widget {
	case Toggle:
		if _, ok := v.(bool); !ok {
			return fmt.Errorf("must be true or false")
		}
	case Choice:
		s, ok := v.(string)
		if !ok {
			return fmt.Errorf("must be one of %s", strings.Join(e.Choices, ", "))
		}
		if !slices.Contains(e.Choices, s) {
			return fmt.Errorf("%q is not one of %s", s, strings.Join(e.Choices, ", "))
		}
	case Number, Bytes:
		n, ok := toInt64(v)
		if !ok {
			return fmt.Errorf("must be a whole number")
		}
		if n < e.Min {
			return fmt.Errorf("must be at least %d", e.Min)
		}
		if e.Max > e.Min && n > e.Max {
			return fmt.Errorf("must be at most %d", e.Max)
		}
	case Text:
		// No constraint by design; see the Text doc comment.
	}
	return nil
}

// toInt64 accepts the shapes a YAML integer arrives in. A float is accepted
// only when it is whole: 1e6 is a legal way to write a cache size, 1.5 is not a
// legal way to write anything here.
func toInt64(v any) (int64, bool) {
	switch n := v.(type) {
	case int:
		return int64(n), true
	case int8:
		return int64(n), true
	case int16:
		return int64(n), true
	case int32:
		return int64(n), true
	case int64:
		return n, true
	case uint:
		return int64(n), true
	case uint8:
		return int64(n), true
	case uint16:
		return int64(n), true
	case uint32:
		return int64(n), true
	case uint64:
		return int64(n), true
	case float32:
		return wholeFloat(float64(n))
	case float64:
		return wholeFloat(n)
	}
	return 0, false
}

func wholeFloat(f float64) (int64, bool) {
	n := int64(f)
	if float64(n) != f {
		return 0, false
	}
	return n, true
}

// Store is where a set of settings' values live. There are two kinds - the
// config file and the Telegram account - and the overlay is written against
// this rather than against either.
//
// Only the file store exists today. The seam is here because the registry is
// what the completeness test checks and what the overlay renders, so adding a
// second store afterwards would mean reopening both (ADR 0010).
//
// A store's value can change without the app asking - another client changes an
// account setting, a person edits the config file in an editor. How a store
// says so is deliberately undecided: the file store learns of it by being
// reloaded, and the account store will need to push, which is a shape worth
// choosing when there is something to push.
type Store interface {
	// Entries returns the settings this store holds, in the order they are
	// shown.
	Entries() []Entry
	// Origin names where the values are kept, in words a person can act on: the
	// path of the file, or what account this is. Read-only settings show it, so
	// that being unable to change a setting here is not a dead end.
	Origin() string
	// Value answers the current value of a setting and how far the last change
	// to it has got. A Status other than Saved is about the store, not about the
	// value: an Unknown value has not arrived, and a Failed one is the value the
	// store still holds, not the one that was refused.
	Value(key string) (v any, status Status)
	// Set asks the store to take a new value. The error is refusal to attempt -
	// no such setting, a value Validate rejects, a read-only setting - and not
	// the outcome of the attempt, which arrives through Value as the status
	// leaves Saving.
	Set(key string, v any) error
}
