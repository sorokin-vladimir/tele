package config

import (
	"fmt"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/sorokin-vladimir/tele/internal/settings"
)

// Store is the config file, as a place settings live. It holds the config the
// app is running on, and it is the only thing that changes it.
//
// The config is swapped whole, the way a theme is: a reader takes the pointer
// and works from it, so nothing can observe half of a change. And a change gets
// there one way only - written to the file, and the file read again from
// scratch (ADR 0009). Editing a setting in the overlay and editing the same
// setting in an editor are therefore the same act, not two acts that a test has
// to keep agreeing with each other.
//
// It knows the path and the platform state directory because reloading needs
// both. Config itself does not: it is a value, and where it came from is not
// part of what it is worth.
type Store struct {
	path            string
	defaultStateDir string

	// writes serializes the read-modify-write of the file, so two settings
	// changed at once cannot each write a file that lacks the other's change.
	writes  sync.Mutex
	current atomic.Pointer[Config]
}

// Verify the file store answers everything a store has to answer, including
// the optional question only a store with absence in it can answer.
var (
	_ settings.Store      = (*Store)(nil)
	_ settings.Defaulting = (*Store)(nil)
)

// NewStore reads the config at path and holds it.
func NewStore(path, defaultStateDir string) (*Store, error) {
	s := &Store{path: path, defaultStateDir: defaultStateDir}
	if err := s.load(); err != nil {
		return nil, err
	}
	return s, nil
}

// NewStoreOf holds a config that is already in hand, for callers that loaded it
// themselves and for tests.
func NewStoreOf(cfg *Config, path, defaultStateDir string) *Store {
	s := &Store{path: path, defaultStateDir: defaultStateDir}
	s.current.Store(cfg)
	return s
}

// Current returns the config the app is running on. Safe to call as often as
// anything likes: it is a pointer load.
func (s *Store) Current() *Config { return s.current.Load() }

// Path is the file the settings are kept in.
func (s *Store) Path() string { return s.path }

// Origin names where the values are kept, for the rows that cannot be changed
// from here to point at.
func (s *Store) Origin() string { return s.path }

// Entries returns the settings this file holds, in display order.
func (s *Store) Entries() []settings.Entry { return Settings() }

// Value answers what a setting is currently worth. A file setting is either
// known or not a setting at all, so the status is Saved or the answer is
// Unknown; the states in between belong to a store that has to ask a server.
func (s *Store) Value(key string) (any, settings.Status) {
	v, ok := settingValue(s.Current(), key)
	if !ok {
		return nil, settings.Unknown
	}
	return v, settings.Saved
}

// IsDefault reports that the file does not name this setting, so what it is
// worth is what tele chose rather than what anybody did.
func (s *Store) IsDefault(key string) bool {
	cfg := s.Current()
	return cfg == nil || !cfg.named[key]
}

// Set writes a setting and applies it, in that order, because the file is what
// gets applied. A nil value resets the setting: the key is removed, and absence
// is what a default is.
//
// The error is refusal - no such setting, a value the setting will not take, a
// setting that is not changed from here - or the write itself failing. A file
// write finishes before this returns, which is why the file store never reports
// Saving.
func (s *Store) Set(key string, v any) error {
	e, ok := Setting(key)
	if !ok {
		return fmt.Errorf("%s is not a setting", key)
	}
	if e.ReadOnly {
		return fmt.Errorf("%s is not changed from here; it is kept in %s", key, s.path)
	}
	if err := e.Validate(v); err != nil {
		return fmt.Errorf("%s: %w", key, err)
	}

	s.writes.Lock()
	defer s.writes.Unlock()
	if err := editFile(s.path, key, v); err != nil {
		return err
	}
	return s.load()
}

// Reload re-reads the file and swaps the config for what it says. It is what a
// person's own edit in an editor comes in through, and what the overlay's own
// writes come in through, so there is one way in and not two.
func (s *Store) Reload() error {
	s.writes.Lock()
	defer s.writes.Unlock()
	return s.load()
}

// load reads the file and installs it. The caller holds writes, or is NewStore.
func (s *Store) load() error {
	cfg, err := Load(s.path, s.defaultStateDir)
	if err != nil {
		return err
	}
	s.current.Store(cfg)
	return nil
}

// settingValue reads a setting out of a Config by its key path.
//
// Reflection here rather than a getter per setting: the path and the field are
// already tied together by the mapstructure tag, and that tie is what the
// completeness test checks. A second, hand-written mapping would be a second
// thing to keep in step.
//
// It answers with the resolved value rather than what the file said, which is
// what a read-only row wants to show: session_file after resolveState is where
// the session actually is.
func settingValue(cfg *Config, key string) (any, bool) {
	v := reflect.ValueOf(cfg).Elem()
	for _, seg := range strings.Split(key, ".") {
		if v.Kind() != reflect.Struct {
			return nil, false
		}
		field, ok := fieldByTag(v, seg)
		if !ok {
			return nil, false
		}
		v = field
	}
	return v.Interface(), true
}

func fieldByTag(v reflect.Value, tag string) (reflect.Value, bool) {
	t := v.Type()
	for i := range t.NumField() {
		if t.Field(i).Tag.Get("mapstructure") == tag {
			return v.Field(i), true
		}
	}
	return reflect.Value{}, false
}
