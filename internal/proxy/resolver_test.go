package proxy_test

import (
	"context"
	"io"
	"net"
	"strconv"
	"testing"
	"time"

	"github.com/gotd/td/telegram/dcs"
	"github.com/gotd/td/tg"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sorokin-vladimir/tele/internal/proxy"
)

// The whole chain, locally: a resolver built from the config dials a SOCKS5
// server, which is asked for a data centre that is really a listener in this
// test, and the bytes gotd writes to open the transport come out the other end.
// Anything less than this passes with a dialer that was built and then never
// given to the resolver.
func TestResolver_SOCKS5ReachesTheDataCentreThroughTheProxy(t *testing.T) {
	dc := newDataCentre(t)
	socks := newSocksServer(t, "me", "secret")
	host, port := socks.addr()

	route, err := proxy.Parse(proxy.Config{
		Type: proxy.TypeSOCKS5, Server: host, Port: port,
		Username: "me", Password: "secret",
	})
	require.NoError(t, err)
	resolver, err := proxy.Resolver(route)
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	conn, err := resolver.Primary(ctx, 2, dc.list())
	require.NoError(t, err)
	defer conn.Close() //nolint:errcheck // a test's connection, nothing depends on closing it

	targets, logins := socks.seen()
	assert.Equal(t, []string{dc.addr()}, targets, "the data centre was reached through the proxy")
	assert.Equal(t, []string{"me:secret"}, logins, "the credentials from the config were offered")

	select {
	case got := <-dc.received:
		assert.NotEmpty(t, got, "gotd's transport handshake arrived at the far end")
	case <-ctx.Done():
		t.Fatal("nothing arrived at the data centre")
	}
}

// A route that names no proxy still has to produce a working resolver: auto and
// direct are how tele reaches Telegram when nobody put anything in between.
func TestResolver_BuildsEveryRoute(t *testing.T) {
	t.Setenv("ALL_PROXY", "")

	for _, r := range []proxy.Route{
		{Type: proxy.TypeAuto},
		{Type: proxy.TypeDirect},
		{Type: proxy.TypeSOCKS5, Addr: "10.0.0.1:1080"},
		mtprotoRoute(t),
	} {
		t.Run(r.Type, func(t *testing.T) {
			resolver, err := proxy.Resolver(r)
			require.NoError(t, err)
			assert.NotNil(t, resolver)
		})
	}
}

func TestResolver_RefusesATypeNobodyDeclared(t *testing.T) {
	_, err := proxy.Resolver(proxy.Route{Type: "http"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "auto, direct, mtproto, socks5")
}

// gotd reads the secret a second time when it builds the resolver, and it is
// stricter about some things than we are. A secret that got past ParseSecret
// and then fails here would fail after the interface is up, which is the one
// place this must not happen.
func TestResolver_MTProtoTakesEverySecretParseSecretAccepts(t *testing.T) {
	for _, secret := range []string{plainHex, ddHex, eeHex} {
		route, err := proxy.Parse(proxy.Config{Type: proxy.TypeMTProto, Server: "10.0.0.1", Port: 443, Secret: secret})
		require.NoError(t, err, secret)

		resolver, err := proxy.Resolver(route)
		require.NoError(t, err, secret)
		assert.NotNil(t, resolver)
	}
}

func TestProbe(t *testing.T) {
	t.Run("a proxy that answers", func(t *testing.T) {
		dc := newDataCentre(t)
		host, port := dc.addr2()

		route, err := proxy.Parse(proxy.Config{Type: proxy.TypeSOCKS5, Server: host, Port: port})
		require.NoError(t, err)
		assert.NoError(t, proxy.Probe(t.Context(), route))
	})

	t.Run("a proxy that is not there", func(t *testing.T) {
		host, port := closedPort(t)

		route, err := proxy.Parse(proxy.Config{Type: proxy.TypeSOCKS5, Server: host, Port: port})
		require.NoError(t, err)

		err = proxy.Probe(t.Context(), route)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "not answering")
		assert.Contains(t, err.Error(), route.Addr, "the message names what was dialled")
	})

	// ALL_PROXY was exported for the shell, not for tele, and a route that
	// dials nobody has nothing to dial. Neither is a reason to refuse to start.
	t.Run("routes with nothing to dial", func(t *testing.T) {
		assert.NoError(t, proxy.Probe(t.Context(), proxy.Route{Type: proxy.TypeAuto}))
		assert.NoError(t, proxy.Probe(t.Context(), proxy.Route{Type: proxy.TypeDirect}))
	})
}

// The log line names the address and the kind of secret, because a wrong port
// and an ee secret that should have been dd are what people actually get wrong.
// It never names the secret or the password: this line ends up in issues.
func TestDescribe(t *testing.T) {
	t.Setenv("ALL_PROXY", "")

	mtproto := mtprotoRoute(t)
	socks5 := proxy.Route{Type: proxy.TypeSOCKS5, Addr: "10.0.0.1:1080", Username: "me", Password: "hunter2"}

	assert.Equal(t, "direct", proxy.Route{Type: proxy.TypeDirect}.Describe())
	assert.Contains(t, proxy.Route{Type: proxy.TypeAuto}.Describe(), "connecting directly")
	assert.Equal(t, "socks5 via 10.0.0.1:1080 (auth: yes)", socks5.Describe())
	assert.Equal(t, "socks5 via 10.0.0.1:1080 (auth: no)",
		proxy.Route{Type: proxy.TypeSOCKS5, Addr: "10.0.0.1:1080"}.Describe())
	assert.Equal(t, "mtproto via 10.0.0.1:443 (secret: ee)", mtproto.Describe())

	for _, r := range []proxy.Route{mtproto, socks5} {
		assert.NotContains(t, r.Describe(), "hunter2")
		assert.NotContains(t, r.Describe(), plainHex, "the secret never reaches the log")
	}
}

func TestDescribe_AutoNamesWhatALLPROXYSays(t *testing.T) {
	t.Setenv("ALL_PROXY", "socks5://me:hunter2@10.0.0.1:1080")

	got := proxy.Route{Type: proxy.TypeAuto}.Describe()
	assert.Contains(t, got, "ALL_PROXY")
	assert.Contains(t, got, "10.0.0.1:1080")
	assert.Contains(t, got, "auth: yes")
	assert.NotContains(t, got, "hunter2", "the variable's password is a credential too")
}

func mtprotoRoute(t *testing.T) proxy.Route {
	t.Helper()
	r, err := proxy.Parse(proxy.Config{Type: proxy.TypeMTProto, Server: "10.0.0.1", Port: 443, Secret: eeHex})
	require.NoError(t, err)
	return r
}

// dataCentre stands in for Telegram: a listener that reports the first thing
// anybody says to it.
type dataCentre struct {
	ln       net.Listener
	received chan []byte
}

func newDataCentre(t *testing.T) *dataCentre {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	t.Cleanup(func() { _ = ln.Close() })

	dc := &dataCentre{ln: ln, received: make(chan []byte, 1)}
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			go func() {
				defer conn.Close() //nolint:errcheck // a test's connection, nothing depends on closing it
				buf := make([]byte, 64)
				n, err := conn.Read(buf)
				if err != nil && n == 0 {
					return
				}
				select {
				case dc.received <- buf[:n]:
				default:
				}
				_, _ = io.Copy(io.Discard, conn)
			}()
		}
	}()
	return dc
}

func (d *dataCentre) addr() string { return d.ln.Addr().String() }

func (d *dataCentre) addr2() (host string, port int) {
	host, p, _ := net.SplitHostPort(d.addr())
	port, _ = strconv.Atoi(p)
	return host, port
}

// list is the DC list gotd resolves against, pointing at this listener.
func (d *dataCentre) list() dcs.List {
	host, port := d.addr2()
	return dcs.List{Options: []tg.DCOption{{ID: 2, IPAddress: host, Port: port}}}
}

// closedPort returns an address nobody is listening on, by listening on one and
// then stopping.
func closedPort(t *testing.T) (host string, port int) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	h, p, _ := net.SplitHostPort(ln.Addr().String())
	require.NoError(t, ln.Close())
	port, _ = strconv.Atoi(p)
	return h, port
}
