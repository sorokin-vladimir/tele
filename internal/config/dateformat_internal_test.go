package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A Go time layout is free text: Format accepts any string, so the only
// layouts refused are the ones a formatted time cannot be parsed back with,
// plus the ones that are not strings at all and the empty one (#238).
func TestLegalDateFormat(t *testing.T) {
	legal := []any{
		nil, // absence: a reset to the default
		"15:04",
		"02.01.2006 15:04",
		"3:04pm",
		"Jan _2 3:04pm",
		"2006-01-02T15:04:05Z07:00",
		"Monday, January 2, 2006 15:04:05.000 MST",
	}
	for _, v := range legal {
		assert.NoError(t, legalDateFormat(v), "%#v should be legal", v)
	}

	illegal := []any{
		1504,           // a number is not a layout string
		true,           // neither is a boolean
		[]any{"15:04"}, // nor a list
		"",             // an empty layout would draw no timestamp at all
		"100%",         // a layout that does not round-trip through Parse
	}
	for _, v := range illegal {
		assert.Error(t, legalDateFormat(v), "%#v should be refused", v)
	}
}

// The check is wired to the key it belongs to, so repairIllegal and Store.Set
// both find it by looking the key up.
func TestKeyChecks_CoversDateFormat(t *testing.T) {
	check, ok := keyChecks[dateFormatKey]
	require.True(t, ok, "%s has no check", dateFormatKey)
	assert.NoError(t, check(DefaultDateFormat))
	assert.Error(t, check(1504))
}
