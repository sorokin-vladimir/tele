package config

import (
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sorokin-vladimir/tele/internal/settings"
)

// This is why reflection is used at all. It cannot produce the registry - it
// knows nothing about labels, legal values, units or when a change takes hold -
// but it can notice a field of Config that nobody has described, which is the
// failure this whole design exists to prevent: a setting that works, and that
// nobody can find.
//
// A new field fails this test. Fix it by declaring the setting, or by excluding
// it with a reason that names an issue. See ADR 0008.
func TestRegistry_CoversEveryConfigField(t *testing.T) {
	declared := make(map[string]bool, len(registry))
	for _, e := range registry {
		declared[e.Key] = true
	}
	for _, x := range exclusions {
		declared[x.Key] = true
	}

	for _, key := range configKeys(t) {
		assert.True(t, declared[key],
			"config key %q is in neither the registry nor the exclusions: declare it as a setting, or exclude it with a reason", key)
	}
}

// The reverse direction: a registry entry naming a key that no longer exists is
// as broken as a field nobody described, and it fails quietly - the setting
// simply never shows a value.
func TestRegistry_DeclaresNothingThatIsNotAConfigField(t *testing.T) {
	actual := configKeys(t)
	for _, e := range registry {
		assert.Contains(t, actual, e.Key, "registry declares %q, which is not a field of Config", e.Key)
	}
	for _, x := range exclusions {
		assert.Contains(t, actual, x.Key, "exclusions name %q, which is not a field of Config", x.Key)
	}
}

// An exclusion without a reason is a swept-under-the-carpet setting. The reason
// is expected to name an issue, so that "why is this not editable" has an
// answer that is being worked on rather than an answer that is "someone forgot".
func TestExclusions_ExplainThemselvesAndNameAnIssue(t *testing.T) {
	for _, x := range exclusions {
		assert.NotEmpty(t, x.Reason, "exclusion %q has no reason", x.Key)
		assert.Contains(t, x.Reason, "#", "exclusion %q gives a reason that names no issue: %s", x.Key, x.Reason)
	}
}

// Every editable setting whose value has a shape must have a default to fall
// back to, or repairIllegal has nothing to repair an illegal value with and can
// only shrug.
func TestRegistry_EditableSettingsHaveDefaults(t *testing.T) {
	defaults := defaultValues()
	for _, e := range registry {
		if e.ReadOnly || e.Widget == settings.Text {
			continue
		}
		assert.Contains(t, defaults, e.Key, "%s is editable and has no default", e.Key)
	}
}

// A default that its own declaration would refuse is the one illegal value
// nobody can fix by editing the file.
func TestRegistry_DefaultsAreLegal(t *testing.T) {
	for key, value := range defaultValues() {
		e, ok := Setting(key)
		require.True(t, ok, "default for %q, which is not a declared setting", key)
		assert.NoError(t, e.Validate(value), "default for %s is a value the setting refuses", key)
	}
}

// Entries are grouped and ordered by hand so that the overlay reads like the
// file. A group split into two runs would render as two headings with the same
// name.
func TestRegistry_GroupsAreContiguous(t *testing.T) {
	var seen []string
	for i, e := range registry {
		if i > 0 && registry[i-1].Group == e.Group {
			continue
		}
		assert.NotContains(t, seen, e.Group, "group %q appears in two separate runs of the registry", e.Group)
		seen = append(seen, e.Group)
	}
}

// A setting nobody can read about is only half declared: the label names it and
// the help says what it does, and neither substitutes for the other.
func TestRegistry_EntriesAreDescribed(t *testing.T) {
	for _, e := range registry {
		assert.NotEmpty(t, e.Label, "%s has no label", e.Key)
		assert.NotEmpty(t, e.Help, "%s has no help", e.Key)
		if e.Widget == settings.Choice {
			assert.NotEmpty(t, e.Choices, "%s is a choice with nothing to choose from", e.Key)
		}
	}
}

// configKeys walks Config and returns the dotted path of every key the config
// file can carry. Fields tagged mapstructure:"-" are derived rather than
// configured and are not keys at all.
func configKeys(t *testing.T) []string {
	t.Helper()
	var keys []string
	var walk func(rt reflect.Type, prefix string)
	walk = func(rt reflect.Type, prefix string) {
		for i := range rt.NumField() {
			f := rt.Field(i)
			if !f.IsExported() {
				// Bookkeeping the file cannot reach: nothing can be written to
				// an unexported field, so it is not a key.
				continue
			}
			tag := f.Tag.Get("mapstructure")
			require.NotEmpty(t, tag, "%s.%s has no mapstructure tag, so nothing can tell whether it is a setting", rt.Name(), f.Name)
			if tag == "-" {
				continue
			}
			path := tag
			if prefix != "" {
				path = prefix + "." + tag
			}
			if f.Type.Kind() == reflect.Struct {
				walk(f.Type, path)
				continue
			}
			keys = append(keys, path)
		}
	}
	walk(reflect.TypeFor[Config](), "")
	slices.Sort(keys)
	return keys
}

// The walk is the load-bearing part of the completeness test, so it is checked
// against what the config file demonstrably carries rather than trusted.
func TestConfigKeys_WalksNestingAndSkipsDerivedFields(t *testing.T) {
	keys := configKeys(t)

	assert.Contains(t, keys, "telegram.api_id", "a nested key")
	assert.Contains(t, keys, "ui.toasts.error_zone", "a twice-nested key")
	assert.Contains(t, keys, "state_dir", "a key at the file's root")
	assert.Contains(t, keys, "keybindings", "a map is one key, not a key per entry")
	assert.Contains(t, keys, "ui.theme", "an any-typed key is still a key")

	for _, derived := range []string{"ui.ThemeSlots", "ThemesDir", "SessionPinned", "Warnings"} {
		assert.NotContains(t, keys, derived, "%s is derived, not configured", derived)
	}
	for _, key := range keys {
		assert.Equal(t, strings.ToLower(key), key, "viper lowercases keys, so %q could never be matched", key)
	}
}
