package config

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/sorokin-vladimir/tele/internal/appkey"
	"github.com/sorokin-vladimir/tele/internal/proxy"
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

	// keySource is where the app key in force came from. The file holds only
	// the person's own key; which built-in one runs in its place is settled
	// beside the file, when the process starts (#239).
	keySource appkey.Source
}

// SetKeySource records where the app key in force came from, so the rows for
// the key can name a built-in one when the file names none. Set once, before
// anything reads the store.
func (s *Store) SetKeySource(source appkey.Source) { s.keySource = source }

// builtInKey answers the two key rows while a built-in key is in force. The
// file names no key then, and its zero and its empty hash would read as a
// client without one. Which built-in key it is gets named; the key itself is
// not shown, because it is not the person's and this screen ends up in
// recordings.
func (s *Store) builtInKey(cfg *Config, key string) (settings.Described, bool) {
	if key != "telegram.api_id" && key != "telegram.api_hash" {
		return "", false
	}
	if cfg == nil || cfg.Telegram.APIID != 0 || cfg.Telegram.APIHash != "" {
		return "", false
	}
	switch s.keySource {
	case appkey.SourceInjected:
		return "built-in (official build)", true
	case appkey.SourcePublished:
		return "built-in (published)", true
	}
	return "", false
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
	cfg := s.Current()
	if builtIn, ok := s.builtInKey(cfg, key); ok {
		return builtIn, settings.Saved
	}
	v, ok := settingValue(cfg, key)
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
	e, ok := settingFor(key)
	if !ok {
		return fmt.Errorf("%s is not a setting", key)
	}
	if e.ReadOnly {
		return fmt.Errorf("%s is not changed from here; it is kept in %s", key, s.path)
	}
	if err := e.Validate(v); err != nil {
		return fmt.Errorf("%s: %w", key, err)
	}
	// A check beyond the widget's shape, where one is declared, judged before
	// the write the way the proxy section is: a value the config would repair
	// on load must not reach the file from here either (ADR 0008).
	if check, ok := keyChecks[key]; ok {
		if err := check(v); err != nil {
			return fmt.Errorf("%s: %w", key, err)
		}
	}
	// A proxy value is judged before it is written rather than after. The file
	// is written first and read back second, so a value the config will not
	// load would already be in the file by the time anybody found out - and the
	// screen that would put it right lives inside a tele that no longer starts
	// (ADR 0017).
	if strings.HasPrefix(key, proxyPrefix) {
		if err := checkProxyEdit(s.Current().Proxy, key, v); err != nil {
			return err
		}
	}

	s.writes.Lock()
	defer s.writes.Unlock()
	unlock, err := lockEdits(s.path)
	if err != nil {
		return err
	}
	err = editFile(s.path, key, v)
	unlock()
	if err != nil {
		return err
	}
	return s.load()
}

// checkProxyEdit reads the proxy section as it would be once this key holds
// this value, and refuses what Load would refuse. A nil value is a reset, which
// is absence, which for the type is auto.
//
// The whole section is judged rather than the one key, because that is the
// question being asked: not "is this a secret" but "will tele start with this".
// It makes the order of edits matter - the address and the secret go in before
// the type is switched to mtproto - so the refusal says so.
func checkProxyEdit(cur proxy.Config, key string, v any) error {
	cand := cur
	switch key {
	case proxyPrefix + "type":
		text, err := proxyText(key, v)
		if err != nil {
			return err
		}
		cand.Type = text
		if cand.Type == "" {
			cand.Type = proxy.TypeAuto
		}
	case proxyPrefix + "server":
		text, err := proxyText(key, v)
		if err != nil {
			return err
		}
		cand.Server = text
	case proxyPrefix + "port":
		cand.Port = 0
		if v != nil {
			n, ok := toInt64(v)
			if !ok {
				return fmt.Errorf("%s: %v is not a port", key, v)
			}
			cand.Port = int(n)
		}
	case proxyPrefix + "secret":
		text, err := proxyText(key, v)
		if err != nil {
			return err
		}
		cand.Secret = text
	case proxyPrefix + "username":
		text, err := proxyText(key, v)
		if err != nil {
			return err
		}
		cand.Username = text
	case proxyPrefix + "password":
		text, err := proxyText(key, v)
		if err != nil {
			return err
		}
		cand.Password = text
	default:
		return fmt.Errorf("%s is not a setting", key)
	}

	if _, err := proxy.Parse(cand); err != nil {
		if key == proxyPrefix+"type" {
			return fmt.Errorf("%w; fill in the rest of the section first, then change the type", err)
		}
		return err
	}
	return nil
}

// proxyText reads a text value out of what the overlay offers, where absence is
// a reset to nothing.
func proxyText(key string, v any) (string, error) {
	if v == nil {
		return "", nil
	}
	text, ok := v.(string)
	if !ok {
		return "", fmt.Errorf("%s: %v is not text", key, v)
	}
	return text, nil
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

// DecodeValue reads a setting's value out of JSON into the Go type the setting
// holds, which is how a value that crossed a connection is made the same value
// as one set in this process: a number is the field's integer, never a
// float64. Nothing, or null, is a reset (#315).
func DecodeValue(key string, raw json.RawMessage) (any, error) {
	if len(raw) == 0 || string(raw) == "null" {
		return nil, nil
	}
	if _, ok := settingFor(key); !ok {
		return nil, fmt.Errorf("%s is not a setting", key)
	}
	t := reflect.TypeFor[Config]()
	for _, seg := range strings.Split(key, ".") {
		field, ok := fieldTypeByTag(t, seg)
		if !ok {
			return nil, fmt.Errorf("%s is not a setting", key)
		}
		t = field
	}
	v := reflect.New(t)
	if err := json.Unmarshal(raw, v.Interface()); err != nil {
		return nil, fmt.Errorf("%s: %w", key, err)
	}
	return v.Elem().Interface(), nil
}

func fieldTypeByTag(t reflect.Type, tag string) (reflect.Type, bool) {
	if t.Kind() != reflect.Struct {
		return nil, false
	}
	for i := range t.NumField() {
		if t.Field(i).Tag.Get("mapstructure") == tag {
			return t.Field(i).Type, true
		}
	}
	return nil, false
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
