// Package dial is the client's end of an attachment over a connection: the
// surface a client holds, made of calls to a host on the other side, and the
// stream the host tells it on. A client cannot tell it from an attachment in
// its own process (#209).
package dial

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sync"
	"sync/atomic"
	"time"

	"github.com/sorokin-vladimir/tele/internal/core"
	"github.com/sorokin-vladimir/tele/internal/core/project"
	"github.com/sorokin-vladimir/tele/internal/domain"
	"github.com/sorokin-vladimir/tele/internal/ipc/protocol"
	"github.com/sorokin-vladimir/tele/internal/ipc/wire"
	"github.com/sorokin-vladimir/tele/internal/priority"
	"github.com/sorokin-vladimir/tele/internal/telerr"
)

// Attachment is an attachment over a connection. Attached to a service, it
// outlives the connection: when the service goes, it reaches it again and
// carries on where it was (#210).
type Attachment struct {
	// hello is what this client says of itself, again on every reattach.
	hello wire.Hello
	// redial reaches the host again, nil when this client never waits for it.
	// pauseCap and giveUp are how it waits, settled when it attaches.
	redial   func(context.Context) (io.ReadWriteCloser, error)
	pauseCap time.Duration
	giveUp   time.Duration
	// nextID is the last request id used, nextSub the last subscription id.
	// Both only grow, across connections too, and both are this side's to
	// choose.
	nextID  atomic.Uint64
	nextSub atomic.Uint64

	// stream carries deltas and events to the client, in the order the host
	// sent them. It holds as much as the host's side of an attachment does.
	stream chan any
	// closed is closed when the client closes the attachment.
	closed    chan struct{}
	closeOnce sync.Once

	mu sync.Mutex
	// host is what the host said of itself in its last hello.
	host wire.Hello
	// link is the connection the attachment runs over now.
	link *link
	// waiting holds the calls whose answer is still to come, by request id.
	waiting map[uint64]chan wire.Frame
	// subs are the client's subscriptions, and focus what it showed last:
	// what it says again to a host it reaches again.
	subs    map[project.SubID]project.Window
	focus   domain.HistoryKey
	focused bool
	// uninstalled is set when the service said it stops for good.
	uninstalled bool
	// lostErr says why the attachment ended, once its stream has.
	lostErr error
}

// link is one connection an attachment runs over.
type link struct {
	conn io.ReadWriteCloser
	enc  *wire.Encoder
	// lost is closed when the connection ends, and why says why.
	lost chan struct{}
	why  *telerr.Error
	// ready is closed once what the client held has been said again over
	// the connection; the client's own calls wait for it, so none overtakes
	// that.
	ready chan struct{}
}

// newLink is a link over conn, ready for the client's calls.
func newLink(conn io.ReadWriteCloser) *link {
	l := &link{conn: conn, enc: wire.NewEncoder(conn), lost: make(chan struct{}), ready: make(chan struct{})}
	close(l.ready)
	return l
}

// await waits until l is ready for the client's calls, and reports false when
// it was lost first.
func (l *link) await() bool {
	select {
	case <-l.ready:
		return true
	case <-l.lost:
		return false
	}
}

// Option is something an attachment is dialled with.
type Option func(*Attachment)

// Redial reaches the host again, for an attachment to a service to wait for
// it when it goes (#210). A redial is one attempt; the attachment tries again
// until the service is back or it gives up.
func Redial(redial func(context.Context) (io.ReadWriteCloser, error)) Option {
	return func(a *Attachment) { a.redial = redial }
}

var _ core.Surface = (*Attachment)(nil)

// Mismatch is a host this client does not use as it is: what to do about it,
// and what the host said of itself, such as the config file it reads.
type Mismatch struct {
	Verdict wire.Verdict
	Host    wire.Hello
}

func (m *Mismatch) Error() string {
	return fmt.Sprintf("the host runs %s on protocol %d, and this client cannot use it as it is (verdict %d)",
		m.Host.Binary, m.Host.Protocol, m.Verdict)
}

// Dial attaches over conn, which it owns from here: it says hello, hears the
// host's, and decides whether the two understand each other. A host it does
// not use answers with a *Mismatch, and conn is closed.
func Dial(ctx context.Context, conn io.ReadWriteCloser, hello wire.Hello, opts ...Option) (*Attachment, error) {
	host, dec, err := join(ctx, conn, hello)
	if err != nil {
		return nil, err
	}
	a := &Attachment{
		hello:    hello,
		host:     host,
		link:     newLink(conn),
		stream:   make(chan any, core.StreamSize),
		closed:   make(chan struct{}),
		waiting:  map[uint64]chan wire.Frame{},
		subs:     map[project.SubID]project.Window{},
		pauseCap: reattachPauseCap,
		giveUp:   giveUpAfter,
	}
	for _, opt := range opts {
		opt(a)
	}
	a.nextID.Store(helloID)
	go a.read(a.link, dec)
	return a, nil
}

// join says hello over conn, which it owns from here, hears the host's, and
// decides whether the two understand each other. A host this client does not
// use answers with a *Mismatch, and conn is closed.
func join(ctx context.Context, conn io.ReadWriteCloser, hello wire.Hello) (wire.Hello, *wire.Decoder, error) {
	// Hello is said before anything reads the connection, so its answer is
	// read here; a ctx that ends first closes conn, which ends the wait.
	stop := context.AfterFunc(ctx, func() { _ = conn.Close() })
	host, dec, err := greet(conn, hello)
	if !stop() || err != nil {
		_ = conn.Close()
		if ctx.Err() != nil {
			return host, nil, ctx.Err()
		}
		return host, nil, err
	}
	if v := wire.Decide(hello, host); v != wire.Proceed {
		_ = conn.Close()
		return host, nil, &Mismatch{Verdict: v, Host: host}
	}
	return host, dec, nil
}

// Introduce says hello over conn and hears the host's, without attaching: what
// someone that has no use for the host needs to name it. conn is closed.
func Introduce(ctx context.Context, conn io.ReadWriteCloser, hello wire.Hello) (wire.Hello, error) {
	stop := context.AfterFunc(ctx, func() { _ = conn.Close() })
	defer stop()
	defer func() { _ = conn.Close() }()
	host, _, err := greet(conn, hello)
	if ctx.Err() != nil {
		return host, ctx.Err()
	}
	return host, err
}

// AskStatus asks the host over conn how it is doing, without attaching: it
// says hello, hears the host's, and asks. It changes nothing, so it asks any
// host on this protocol, whatever the versions; a host on another one is not
// asked anything it might misread, and answers with a *Mismatch beside its
// hello (#208). conn is closed.
func AskStatus(ctx context.Context, conn io.ReadWriteCloser, hello wire.Hello) (wire.Hello, wire.HostStatus, error) {
	stop := context.AfterFunc(ctx, func() { _ = conn.Close() })
	defer stop()
	defer func() { _ = conn.Close() }()
	var status wire.HostStatus
	host, dec, err := greet(conn, hello)
	if err != nil {
		return host, status, orCtx(ctx, err)
	}
	if host.Protocol != hello.Protocol {
		return host, status, &Mismatch{Verdict: wire.Refuse, Host: host}
	}
	err = askOnce(ctx, conn, dec, wire.MethodStatus, &status)
	return host, status, err
}

// AskRestart asks the host over conn to restart, without attaching: a client
// newer than a service asks it, on a connection of its own (#183). A host that
// agrees stops, and AskRestart returns once it has gone, so that whoever
// reaches the endpoint next finds the service started again, not the one
// stopping. conn is closed.
func AskRestart(ctx context.Context, conn io.ReadWriteCloser, hello wire.Hello) (wire.RestartResult, error) {
	stop := context.AfterFunc(ctx, func() { _ = conn.Close() })
	defer stop()
	defer func() { _ = conn.Close() }()
	var result wire.RestartResult
	_, dec, err := greet(conn, hello)
	if err != nil {
		return result, orCtx(ctx, err)
	}
	if err := askOnce(ctx, conn, dec, wire.MethodRestart, &result); err != nil || result.Refused != "" {
		return result, err
	}
	for {
		if _, err := dec.Decode(); err != nil {
			return result, orCtx(ctx, nil)
		}
	}
}

// AskUninstall tells the host over conn that it is being uninstalled, without
// attaching (#182). Only a service takes note; it is stopped afterwards by
// its service manager, not by this. conn is closed.
func AskUninstall(ctx context.Context, conn io.ReadWriteCloser, hello wire.Hello) (wire.UninstallResult, error) {
	stop := context.AfterFunc(ctx, func() { _ = conn.Close() })
	defer stop()
	defer func() { _ = conn.Close() }()
	var result wire.UninstallResult
	_, dec, err := greet(conn, hello)
	if err != nil {
		return result, orCtx(ctx, err)
	}
	err = askOnce(ctx, conn, dec, wire.MethodUninstall, &result)
	return result, err
}

// askOnce asks method of the host over conn, once hello is behind it, and
// decodes the answer into dst. Whatever else the host says before its answer
// is meant for a client drawing the account, which the one asking is not.
func askOnce(ctx context.Context, conn io.Writer, dec *wire.Decoder, method string, dst any) error {
	if err := wire.NewEncoder(conn).Encode(wire.Request(askID, method, nil)); err != nil {
		return orCtx(ctx, hostLost())
	}
	for {
		f, err := dec.Decode()
		if err != nil {
			return orCtx(ctx, hostLost())
		}
		if f.ID != askID {
			continue
		}
		if f.Error != nil {
			return f.Error
		}
		if err := json.Unmarshal(f.Result, dst); err != nil {
			return protocol.Violation(fmt.Errorf("%w: %s: %v", wire.ErrProtocol, method, err))
		}
		return nil
	}
}

// askID is the request id askOnce asks under, the one after hello's.
const askID = helloID + 1

// orCtx is err, or why ctx ended when it has: an ended ctx closes the
// connection, and what the read then says is only the symptom.
func orCtx(ctx context.Context, err error) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}
	return err
}

// helloID is the request id hello goes under: the first one.
const helloID = 1

func greet(conn io.ReadWriter, hello wire.Hello) (wire.Hello, *wire.Decoder, error) {
	var host wire.Hello
	raw, err := json.Marshal(hello)
	if err != nil {
		return host, nil, err
	}
	if err := wire.NewEncoder(conn).Encode(wire.Request(helloID, wire.MethodHello, raw)); err != nil {
		return host, nil, hostLost()
	}
	dec := wire.NewDecoder(conn)
	f, err := dec.Decode()
	if err != nil {
		if errors.Is(err, wire.ErrProtocol) {
			return host, nil, protocol.Violation(err)
		}
		return host, nil, hostLost()
	}
	if f.Error != nil {
		return host, nil, f.Error
	}
	if f.ID != helloID || f.Result == nil {
		return host, nil, protocol.Violation(fmt.Errorf("%w: the host did not answer hello", wire.ErrProtocol))
	}
	if err := json.Unmarshal(f.Result, &host); err != nil {
		return host, nil, protocol.Violation(fmt.Errorf("%w: hello: %v", wire.ErrProtocol, err))
	}
	return host, dec, nil
}

// Stream is everything the host tells this client, deltas and events alike,
// in the order it told them: the same values an attachment in the host's own
// process carries.
func (a *Attachment) Stream() <-chan any { return a.stream }

// Host is what the host said of itself when the client attached, or attached
// again last: its versions, and the config file it reads.
func (a *Attachment) Host() wire.Hello {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.host
}

// Lost is why the attachment ended, once its stream has: the connection
// ending, the host saying why it went, ErrGaveUp for a service that did not
// come back, or a *Mismatch for one that came back as a tele this client does
// not use.
func (a *Attachment) Lost() error {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.lostErr
}

// Close ends the attachment: the client is gone.
func (a *Attachment) Close() error {
	a.closeOnce.Do(func() { close(a.closed) })
	return a.current().conn.Close()
}

// current is the link the attachment runs over now.
func (a *Attachment) current() *link {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.link
}

// hostLost is what a call answers when the connection ended: the host may be
// back, and the account it served may be too.
func hostLost() *telerr.Error {
	return &telerr.Error{Kind: telerr.Network, Op: "host", Transient: true}
}

// read takes the host's frames off the connection until it ends: answers go
// to the calls waiting for them, deltas and events onto the stream. A client
// that is not draining its stream holds this up, and the host with it, as it
// would hold up an attachment in the host's own process.
//
// When a connection ends, every call waiting on it and every call after is
// answered with why. A client of a service then waits for it to come back
// over a new connection; for anyone else, or once waiting is over, the stream
// ends, which is how its holder hears of it.
func (a *Attachment) read(l *link, dec *wire.Decoder) {
	for {
		why, broke := a.readUntilEnd(l, dec)
		a.drop(l, why, broke)
		if broke || !a.waitsForHost() {
			a.end(why)
			return
		}
		next, nextDec, err := a.reattach()
		if err != nil {
			a.end(err)
			return
		}
		l, dec = next, nextDec
	}
}

// drop gives up l, which ended for why.
func (a *Attachment) drop(l *link, why *telerr.Error, broke bool) {
	a.mu.Lock()
	l.why = why
	close(l.lost)
	a.mu.Unlock()
	if broke {
		_ = l.enc.Encode(wire.Fail(0, why))
	}
	_ = l.conn.Close()
}

// end ends the attachment for why: its stream ends.
func (a *Attachment) end(why error) {
	a.mu.Lock()
	a.lostErr = why
	a.mu.Unlock()
	close(a.stream)
}

// readUntilEnd reads l until the connection ends, and says why it did. broke
// says the host broke the protocol, which it is told before the connection
// goes.
func (a *Attachment) readUntilEnd(l *link, dec *wire.Decoder) (why *telerr.Error, broke bool) {
	for {
		f, err := dec.Decode()
		if err != nil {
			if errors.Is(err, wire.ErrProtocol) {
				return protocol.Violation(err), true
			}
			return hostLost(), false
		}
		shape, err := f.Shape()
		if err != nil {
			return protocol.Violation(err), true
		}
		switch shape {
		case wire.ShapeError:
			if f.ID == 0 {
				// The host says why it is going: that is the reason, not
				// the connection ending after it.
				return f.Error, false
			}
			fallthrough
		case wire.ShapeReply:
			a.mu.Lock()
			ch, ok := a.waiting[f.ID]
			delete(a.waiting, f.ID)
			a.mu.Unlock()
			if ok {
				ch <- f
			}
		case wire.ShapeDelta, wire.ShapeEvent:
			v, err := protocol.Unframe(f)
			if err != nil {
				return protocol.Violation(err), true
			}
			if s, ok := v.(core.HostStopping); ok && s.Reason == core.StopUninstalled {
				a.mu.Lock()
				a.uninstalled = true
				a.mu.Unlock()
			}
			select {
			case a.stream <- v:
			case <-a.closed:
				return hostLost(), false
			}
		default:
			return protocol.Violation(fmt.Errorf("%w: a host does not send %v", wire.ErrProtocol, shape)), true
		}
	}
}

// call makes method with args and waits for its answer, which it decodes into
// dsts. A call whose ctx ends first is called off on the host and answers with
// ctx's error; its answer, if one comes, is dropped.
func (a *Attachment) call(ctx context.Context, method string, args []any, dsts ...any) error {
	c := protocol.Calls[method]
	params, err := c.Params(args...)
	if err != nil {
		return err
	}
	result, err := a.roundTrip(ctx, method, params)
	if err != nil {
		return err
	}
	return c.Decode(result, dsts...)
}

// roundTrip sends method with params and waits for the result that answers
// it.
func (a *Attachment) roundTrip(ctx context.Context, method string, params json.RawMessage) (json.RawMessage, error) {
	id := a.nextID.Add(1)
	answer := make(chan wire.Frame, 1)
	a.mu.Lock()
	l := a.link
	select {
	case <-l.lost:
		// The host is away, or gone for good: the call is not kept for a
		// host that may never answer it.
		a.mu.Unlock()
		return nil, l.why
	default:
	}
	a.waiting[id] = answer
	a.mu.Unlock()
	defer func() {
		a.mu.Lock()
		delete(a.waiting, id)
		a.mu.Unlock()
	}()

	if !l.await() {
		return nil, l.why
	}
	req := wire.Request(id, method, params)
	// The context does not cross the socket, so what it says about the call
	// goes in the frame.
	req.Background = priority.IsBackground(ctx)
	if err := l.enc.Encode(req); err != nil {
		return nil, hostLost()
	}
	select {
	case f := <-answer:
		if f.Error != nil {
			return nil, f.Error
		}
		return f.Result, nil
	case <-ctx.Done():
		_ = l.enc.Encode(wire.Cancel(id))
		return nil, ctx.Err()
	case <-l.lost:
		return nil, l.why
	}
}

// askHost makes a call about the host and decodes its answer.
func askHost[T any](a *Attachment, ctx context.Context, method string, params any) (T, error) {
	var v T
	raw, err := json.Marshal(params)
	if err != nil {
		return v, err
	}
	result, err := a.roundTrip(ctx, method, raw)
	if err != nil {
		return v, err
	}
	if err := json.Unmarshal(result, &v); err != nil {
		return v, protocol.Violation(fmt.Errorf("%w: %s: %v", wire.ErrProtocol, method, err))
	}
	return v, nil
}

// RestartHost asks a service to restart (#183). One that agrees stops after
// answering, and says it is stopping to restart.
func (a *Attachment) RestartHost(ctx context.Context) (wire.RestartResult, error) {
	return askHost[wire.RestartResult](a, ctx, wire.MethodRestart, nil)
}

// HostSettings asks the host for its settings as it holds them (#315).
func (a *Attachment) HostSettings(ctx context.Context) (wire.HostSettings, error) {
	return askHost[wire.HostSettings](a, ctx, wire.MethodSettings, struct{}{})
}

// SetHostSetting asks the host to change one of its settings to value, encoded
// as JSON; nil resets it.
func (a *Attachment) SetHostSetting(ctx context.Context, key string, value json.RawMessage) (wire.SetResult, error) {
	return askHost[wire.SetResult](a, ctx, wire.MethodSet, wire.SetParams{Key: key, Value: value})
}

// ReloadHost asks the host to read its config file again.
func (a *Attachment) ReloadHost(ctx context.Context) (wire.ReloadResult, error) {
	return askHost[wire.ReloadResult](a, ctx, wire.MethodReload, struct{}{})
}

// tell makes method with args and does not wait: it has nothing to answer.
// While the host is away it is said to nobody; what of it matters to a host
// reached again is said again then.
func (a *Attachment) tell(method string, args ...any) {
	l := a.current()
	if l.await() {
		a.tellOver(l, method, args...)
	}
}

func (a *Attachment) tellOver(l *link, method string, args ...any) {
	params, err := protocol.Calls[method].Params(args...)
	if err != nil {
		return
	}
	_ = l.enc.Encode(wire.Request(a.nextID.Add(1), method, params))
}

func do(a *Attachment, ctx context.Context, method string, args ...any) error {
	return a.call(ctx, method, args)
}

func ask[T any](a *Attachment, ctx context.Context, method string, args ...any) (T, error) {
	var v T
	err := a.call(ctx, method, args, &v)
	return v, err
}

func ask2[T, U any](a *Attachment, ctx context.Context, method string, args ...any) (T, U, error) {
	var v T
	var u U
	err := a.call(ctx, method, args, &v, &u)
	return v, u, err
}
