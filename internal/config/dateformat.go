package config

import (
	"fmt"
	"time"
)

// DefaultDateFormat is the layout a message timestamp is drawn with when the
// file names none: the 24-hour clock the message list has always used. The UI
// package keeps the same layout as its own fallback, because components do not
// import this package (#238).
const DefaultDateFormat = "15:04"

// dateFormatKey is ui.date_format. It is the one setting whose legality is not
// a shape its widget can judge - a Go time layout is free text - so it is
// checked by keyChecks, in repairIllegal against the file and in Store.Set
// against a value about to be written, the way the proxy section is.
const dateFormatKey = "ui.date_format"

// keyChecks are legality checks beyond what a widget's shape can say, by key.
// They run wherever Entry.Validate runs, so a value the overlay refuses is the
// same value the file refuses (ADR 0008).
var keyChecks = map[string]func(any) error{
	dateFormatKey: legalDateFormat,
}

// legalDateFormat reports whether v is a value ui.date_format accepts: a Go
// time layout that is neither empty nor unparseable.
//
// Format never fails on any string, so the only test a layout can fail is
// parsing a time formatted with it back again; a layout that cannot round-trip
// is one whose rendering cannot be read back either. A nil value is absence -
// a reset to the default - and is legal.
func legalDateFormat(v any) error {
	if v == nil {
		return nil
	}
	layout, ok := v.(string)
	if !ok {
		return fmt.Errorf("must be a Go time layout string, e.g. %q", DefaultDateFormat)
	}
	if layout == "" {
		return fmt.Errorf("must not be empty")
	}
	ref := time.Date(2006, time.January, 2, 15, 4, 5, 987654321, time.UTC)
	if _, err := time.Parse(layout, ref.Format(layout)); err != nil {
		return fmt.Errorf("%q is not a usable time layout: %v", layout, err)
	}
	return nil
}
