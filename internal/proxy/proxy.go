// Package proxy is how tele reaches Telegram's data centres: what the config
// says about the route, whether it can be taken, and the resolver that takes
// it. Everything in here is about getting to Telegram and nothing in here knows
// what is sent once there.
//
// The route is obeyed or tele does not start (ADR 0017), which is why parsing
// refuses rather than falls back: the value a broken proxy section would fall
// back to is a direct connection, and that is the one outcome the section
// exists to avoid.
package proxy

import (
	"fmt"
	"net"
	"net/url"
	"os"
	"strconv"
	"strings"
)

// The values proxy.type takes. Two of them mean no proxy and they are not the
// same thing: auto is the file saying nothing, and leaves ALL_PROXY in charge
// the way it has been since v1.3.0; direct is a person saying so, and switches
// the variable off.
const (
	TypeAuto    = "auto"
	TypeDirect  = "direct"
	TypeMTProto = "mtproto"
	TypeSOCKS5  = "socks5"
)

// Types is every value proxy.type takes, in the order the settings overlay
// offers them. Declared here rather than in the settings registry because this
// package is what refuses the others.
func Types() []string { return []string{TypeAuto, TypeDirect, TypeMTProto, TypeSOCKS5} }

// joinTypes lists the types for a refusal to name. A refusal that says what is
// wrong without saying what is right is half a message.
func joinTypes() string { return strings.Join(Types(), ", ") }

// Config is the proxy section of config.yml. It lives here, and the config
// package embeds it, so that the section and the code that judges it cannot
// drift apart - and so that judging it needs no import back.
type Config struct {
	Type   string `mapstructure:"type"`
	Server string `mapstructure:"server"`
	Port   int    `mapstructure:"port"`
	// Secret reaches an mtproto proxy; it is written hex or base64url.
	Secret string `mapstructure:"secret"`
	// Username and Password reach a socks5 proxy that asks who is calling.
	Username string `mapstructure:"username"`
	Password string `mapstructure:"password"`
}

// Route is a proxy section that has been read and found workable: the same
// decision, in the shape a connection needs it. Nothing downstream re-reads the
// config, so nothing downstream can disagree about which route is in force.
type Route struct {
	Type string
	// Addr is host:port, empty for the two types that dial nobody.
	Addr               string
	Secret             Secret
	Username, Password string
}

// Direct reports that this route puts nothing between tele and Telegram - by
// being told to, or by finding nothing in the environment to use.
func (r Route) Direct() bool {
	return r.Type == TypeDirect || (r.Type == TypeAuto && envProxyURL() == nil)
}

// Parse reads the proxy section and refuses anything that cannot be taken.
//
// Every refusal names the key it is about, because the message it ends up in
// is read by somebody who has no tele in front of them and, quite possibly, no
// other way to reach Telegram.
func Parse(cfg Config) (Route, error) {
	t := strings.TrimSpace(cfg.Type)
	if t == "" {
		// Absence, which is what auto is. Spelled out here so that a Config
		// nobody filled in is the same route as a file that says auto.
		t = TypeAuto
	}

	switch t {
	case TypeAuto, TypeDirect:
		return Route{Type: t}, nil
	case TypeMTProto, TypeSOCKS5:
	default:
		return Route{}, fmt.Errorf("proxy.type is %q; it is one of %s", cfg.Type, joinTypes())
	}

	addr, err := address(cfg)
	if err != nil {
		return Route{}, err
	}
	r := Route{Type: t, Addr: addr}

	if t == TypeMTProto {
		secret, err := ParseSecret(cfg.Secret)
		if err != nil {
			return Route{}, err
		}
		r.Secret = secret
		return r, nil
	}

	// SOCKS5 authentication is a pair. One half of it is not a proxy that lets
	// you in without a password, it is a proxy that turns you away - so it is
	// refused here, where the reason can be given, rather than at the server,
	// where it arrives as a connection that failed.
	user, pass := cfg.Username, cfg.Password
	switch {
	case user == "" && pass != "":
		return Route{}, fmt.Errorf("proxy.password is set without proxy.username; a socks5 proxy is given both or neither")
	case user != "" && pass == "":
		return Route{}, fmt.Errorf("proxy.username is set without proxy.password; a socks5 proxy is given both or neither")
	}
	r.Username, r.Password = user, pass
	return r, nil
}

// address builds host:port, refusing the two ways it can be unusable.
func address(cfg Config) (string, error) {
	server := strings.TrimSpace(cfg.Server)
	if server == "" {
		return "", fmt.Errorf("proxy.server is empty; proxy.type %s needs an address to dial", cfg.Type)
	}
	if cfg.Port < 1 || cfg.Port > 65535 {
		return "", fmt.Errorf("proxy.port is %d; a port is between 1 and 65535", cfg.Port)
	}
	return net.JoinHostPort(server, strconv.Itoa(cfg.Port)), nil
}

// Notice is something worth saying at launch about the route. An ID marks the
// ones that are shown once: a notice without one describes something that is
// still true in the file and is said every time, and one with an ID describes a
// line that only wants moving.
//
// The config package turns these into its own warnings. They are not errors:
// what is described here changes nothing about which route is taken, and a
// route that cannot be taken has already refused in Parse.
type Notice struct {
	Text string
	ID   string
}

// allProxyDeprecated keys the one notice that is shown once. The variable still
// works and will go on working for this release; what is left to do is move a
// line, and saying so at every launch would be nagging.
const allProxyDeprecated = "proxy.all_proxy.deprecated"

// Notices says what is true about the section but did not stop the start: a
// value that belongs to a different type and is therefore ignored, and what the
// environment is contributing when the file left the route to it.
//
// Ignoring a value silently is the thing to avoid here. A secret sitting under
// a socks5 proxy is harmless, but somebody who wrote it believes it is in use.
func Notices(cfg Config) []Notice {
	t := strings.TrimSpace(cfg.Type)
	if t == "" {
		t = TypeAuto
	}

	var out []Notice
	if ignored := ignoredKeys(cfg, t); len(ignored) > 0 {
		out = append(out, Notice{Text: fmt.Sprintf("%s: set under proxy.type %s, which does not use %s; ignored",
			strings.Join(ignored, ", "), t, plural(len(ignored), "it", "them"))})
	}

	if t != TypeAuto {
		return out
	}
	raw, ok := envProxy()
	if !ok {
		return out
	}
	if envProxyURL() == nil {
		// x/net reads the same variable, finds it unusable exactly here, and
		// connects directly without a word. The connection somebody believes is
		// going through a proxy is not, and that is worth saying at every
		// launch, because it stays true until the variable is fixed.
		out = append(out, Notice{Text: fmt.Sprintf(
			"ALL_PROXY is set to %q, which tele cannot dial through - only socks5:// is read from that variable; it is ignored and the connection goes direct", raw)})
		return out
	}
	out = append(out, Notice{
		ID: allProxyDeprecated,
		Text: "ALL_PROXY is in use because proxy.type is auto. It is deprecated and will be removed: " +
			"set proxy.type to socks5 in the config instead, where it applies to tele alone. See docs/configuration.md",
	})
	return out
}

// ignoredKeys names the values this route will not read. A type that dials
// nobody reads none of them.
func ignoredKeys(cfg Config, t string) []string {
	set := map[string]bool{
		"proxy.server":   strings.TrimSpace(cfg.Server) != "",
		"proxy.port":     cfg.Port != 0,
		"proxy.secret":   strings.TrimSpace(cfg.Secret) != "",
		"proxy.username": cfg.Username != "",
		"proxy.password": cfg.Password != "",
	}
	var used []string
	switch t {
	case TypeMTProto:
		used = []string{"proxy.server", "proxy.port", "proxy.secret"}
	case TypeSOCKS5:
		used = []string{"proxy.server", "proxy.port", "proxy.username", "proxy.password"}
	}
	for _, key := range used {
		delete(set, key)
	}

	// Listed in the order the keys appear in the file, so the notice reads like
	// the section it is about.
	var out []string
	for _, key := range []string{"proxy.server", "proxy.port", "proxy.secret", "proxy.username", "proxy.password"} {
		if set[key] {
			out = append(out, key)
		}
	}
	return out
}

func plural(n int, one, many string) string {
	if n == 1 {
		return one
	}
	return many
}

// envProxy returns what ALL_PROXY says, as written. Both spellings are read,
// in the order x/net/proxy reads them, so what tele reports and what tele dials
// through can never be two different variables.
func envProxy() (string, bool) {
	for _, name := range []string{"ALL_PROXY", "all_proxy"} {
		if v := os.Getenv(name); v != "" {
			return v, true
		}
	}
	return "", false
}

// envProxyURL is ALL_PROXY when it names a proxy x/net/proxy will actually
// dial through, and nil otherwise - unset, unparsable, or a scheme that
// package refuses, which in practice means anything but socks5. All three end
// the same way there: the dialer falls back to a direct connection and says
// nothing. Asking the same question here is what lets tele say the answer out
// loud.
func envProxyURL() *url.URL {
	raw, ok := envProxy()
	if !ok {
		return nil
	}
	u, err := url.Parse(raw)
	if err != nil {
		return nil
	}
	switch u.Scheme {
	case "socks5", "socks5h":
		return u
	default:
		return nil
	}
}
