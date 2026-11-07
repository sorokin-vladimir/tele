// Package serve is the host's end of an attachment over a connection: it reads
// a client's calls off the connection, makes them on the attachment the host
// gives it, writes back what they answered, and passes on everything the
// attachment's stream carries (#209).
package serve

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"reflect"
	"sync"

	"github.com/sorokin-vladimir/tele/internal/core"
	"github.com/sorokin-vladimir/tele/internal/core/project"
	"github.com/sorokin-vladimir/tele/internal/ipc/protocol"
	"github.com/sorokin-vladimir/tele/internal/ipc/wire"
	"github.com/sorokin-vladimir/tele/internal/priority"
	"github.com/sorokin-vladimir/tele/internal/telerr"
)

// Attachment is what the host serves a connection from: the surface a client
// may call, the stream it is told on, the subscriptions under the ids the
// client chose, and the end of it all when the client goes.
type Attachment interface {
	core.Surface
	SubscribeAs(id project.SubID, w project.Window) error
	Stream() <-chan any
	// Tell puts ev on the client's stream, for what the host says to it
	// rather than what an owner does.
	Tell(ev any)
	Detach()
}

var _ Attachment = (*core.Attachment)(nil)

// Greeter is an attachment that wants the client's hello: whoever serves it
// learns from it what kind of client is on the other end (#213).
type Greeter interface {
	Greeted(wire.Hello)
}

// Host is the host's own end of a connection: what it says of itself, and the
// calls about it rather than about an account, answered with no owner seated
// as well (#315).
//
// Restart answers, and says what to do once the answer is written: a host
// that stopped first would leave the client that asked without it. What it
// says must not block, since the connection is not done until it returns.
type Host interface {
	Hello() wire.Hello
	Settings() wire.HostSettings
	SetSetting(wire.SetParams) wire.SetResult
	Reload() wire.ReloadResult
	Status() wire.HostStatus
	Restart() (wire.RestartResult, func())
	Uninstall() wire.UninstallResult
	// AccountCommands is cli.account_commands as the host holds it now: what
	// a command may do with the account (#181).
	AccountCommands() wire.CommandAccess
}

// Conn serves att over conn until the client goes, ctx ends, or one side says
// something that is not the protocol. It owns conn from here and closes it,
// and detaches att when it returns: the client is gone.
//
// The client speaks first, with its hello, and is answered with host's.
// Whether the two understand each other is the client's to decide: the host
// serves whoever stays. A break of the protocol, by either side, is said to
// the client before the connection goes, and returned.
//
// A call that takes a context runs beside the others and can be called off; one
// that does not runs on the reading goroutine, in the order it was asked.
func Conn(ctx context.Context, conn io.ReadWriteCloser, att Attachment, host Host) error {
	defer att.Detach()
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	// Ending ctx closes conn, which is what frees a goroutine blocked on it.
	stop := context.AfterFunc(ctx, func() { _ = conn.Close() })
	defer stop()

	c := &served{att: att, host: host, enc: wire.NewEncoder(conn), calls: map[uint64]context.CancelFunc{}}
	dec := wire.NewDecoder(conn)
	if err := c.greet(dec, host.Hello()); err != nil {
		return c.end(err, conn)
	}

	// running is the calls still being made. Only the reading goroutine adds
	// to it, so once that has returned it can be waited on.
	var running sync.WaitGroup
	pumped := make(chan error, 1)
	go func() { pumped <- c.pump(ctx) }()
	read := make(chan error, 1)
	go func() { read <- c.read(ctx, dec, &running) }()

	var err error
	other := pumped
	select {
	case err = <-read:
	case err = <-pumped:
		other = read
	}
	err = c.end(err, conn)
	cancel()
	// The connection's end is the first side's to say; the other side only
	// has to be done.
	<-other //nolint:errcheck
	running.Wait()
	return err
}

// greet takes the client's hello, which must come first, and answers it with
// the host's.
func (c *served) greet(dec *wire.Decoder, host wire.Hello) error {
	f, err := dec.Decode()
	if err != nil {
		return err
	}
	if f.Method != wire.MethodHello {
		return fmt.Errorf("%w: %q before hello", wire.ErrProtocol, f.Method)
	}
	if _, err := f.Shape(); err != nil {
		return err
	}
	var client wire.Hello
	if err := json.Unmarshal(f.Params, &client); err != nil {
		return fmt.Errorf("%w: hello: %v", wire.ErrProtocol, err)
	}
	c.kind = client.Kind
	if g, ok := c.att.(Greeter); ok {
		g.Greeted(client)
	}
	raw, err := json.Marshal(host)
	if err != nil {
		return err
	}
	return c.enc.Encode(wire.Reply(f.ID, raw))
}

// end closes the connection for err: a client that went is no error, and a
// break of the protocol is said to the client first.
func (c *served) end(err error, conn io.Closer) error {
	if errors.Is(err, io.EOF) {
		err = nil
	}
	if errors.Is(err, wire.ErrProtocol) {
		_ = c.enc.Encode(wire.Fail(0, protocol.Violation(err)))
	}
	_ = conn.Close()
	return err
}

// served is one connection being served.
type served struct {
	att  Attachment
	host Host
	enc  *wire.Encoder
	// kind is what the client said it is in its hello. A command is held to
	// cli.account_commands and never changes the host's settings (#181).
	kind wire.ClientKind

	mu sync.Mutex
	// calls holds what calls off each call still running, by request id.
	calls map[uint64]context.CancelFunc
}

// pump passes on everything the attachment's stream carries, until ctx ends or
// the host says it is stopping: that is the last thing the client hears, and
// the connection ends on it (#208).
func (c *served) pump(ctx context.Context) error {
	for {
		select {
		case <-ctx.Done():
			return nil
		case v := <-c.att.Stream():
			f, err := protocol.Frame(v)
			if err != nil {
				// The host's own mistake, but the client is told it the same
				// way: nothing it reads after this can be trusted.
				return fmt.Errorf("%w: %v", wire.ErrProtocol, err)
			}
			if err := c.enc.Encode(f); err != nil {
				return nil
			}
			if _, stopping := v.(core.HostStopping); stopping {
				return nil
			}
		}
	}
}

// read takes the client's frames off the connection until it ends.
func (c *served) read(ctx context.Context, dec *wire.Decoder, running *sync.WaitGroup) error {
	for {
		f, err := dec.Decode()
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}
			return err
		}
		shape, err := f.Shape()
		if err != nil {
			return err
		}
		switch shape {
		case wire.ShapeRequest:
			if err := c.request(ctx, f, running); err != nil {
				return err
			}
		case wire.ShapeCancel:
			id, err := f.CancelledID()
			if err != nil {
				return err
			}
			// A call that already answered is no longer here: the cancel
			// crossed its reply, which is no fault of either side.
			c.mu.Lock()
			if stop, ok := c.calls[id]; ok {
				stop()
			}
			c.mu.Unlock()
		default:
			return fmt.Errorf("%w: a client does not send %v", wire.ErrProtocol, shape)
		}
	}
}

// request makes the call f asks for.
func (c *served) request(ctx context.Context, f wire.Frame, running *sync.WaitGroup) error {
	if answer, then, ok, err := c.hostCall(f); ok {
		if err != nil {
			return err
		}
		raw, err := json.Marshal(answer)
		if err != nil {
			return c.fail(f.ID, err)
		}
		_ = c.enc.Encode(wire.Reply(f.ID, raw))
		if then != nil {
			then()
		}
		return nil
	}
	call, ok := protocol.Calls[f.Method]
	if !ok {
		_ = c.enc.Encode(wire.Fail(f.ID, &telerr.Error{
			Kind: telerr.Internal, Op: f.Method, Detail: "the host has no such call",
		}))
		return nil
	}
	if access := c.host.AccountCommands(); c.kind == wire.KindCommand && !commandMay(access, f.Method) {
		err := &telerr.Error{
			Kind: telerr.CommandsOff, Op: f.Method,
			Detail: fmt.Sprintf("cli.account_commands is %s for the tele holding the account", access),
		}
		if f.Method == protocol.MethodSubscribe {
			// Nobody waits on a subscription's reply: its first delta is the
			// answer, so the refusal is told where that would have come.
			c.att.Tell(core.HostFailure{Err: err})
		}
		return c.fail(f.ID, err)
	}
	args, err := call.Args(f.Params)
	if err != nil {
		return err
	}
	if f.Method == protocol.MethodSubscribe {
		id, w := args[0].Interface().(project.SubID), args[1].Interface().(project.Window)
		if err := c.att.SubscribeAs(id, w); err != nil {
			return fmt.Errorf("%w: %v", wire.ErrProtocol, err)
		}
		return c.reply(f.ID, call, nil)
	}
	if !call.Context {
		outs, err := call.Invoke(ctx, c.att, args)
		if err != nil {
			return c.fail(f.ID, err)
		}
		return c.reply(f.ID, call, outs)
	}
	callCtx, stop := context.WithCancel(ctx)
	if f.Background {
		callCtx = priority.InBackground(callCtx)
	}
	c.mu.Lock()
	c.calls[f.ID] = stop
	c.mu.Unlock()
	running.Go(func() {
		defer func() {
			c.mu.Lock()
			delete(c.calls, f.ID)
			c.mu.Unlock()
			stop()
		}()
		outs, err := call.Invoke(callCtx, c.att, args)
		if err != nil {
			_ = c.fail(f.ID, err)
			return
		}
		_ = c.reply(f.ID, call, outs)
	})
	return nil
}

// commandMay reports whether a command may make the call method under access:
// any with all, a read with read, none with none. A value the host does not
// know lets nothing through.
func commandMay(access wire.CommandAccess, method string) bool {
	switch access {
	case wire.CommandsAll:
		return true
	case wire.CommandsRead:
		_, read := protocol.Reads[method]
		return read
	default:
		return false
	}
}

// hostCall makes f when it is a call about the host, and reports whether it
// was, and what to do once it is answered. These run on the reading
// goroutine, in the order they were asked: each is a read or a write of one
// file, and a set followed by a reload must find the set done.
func (c *served) hostCall(f wire.Frame) (answer any, then func(), ok bool, err error) {
	// A command never changes the host's settings: one that could would turn
	// cli.account_commands back on (#181).
	const commandsDoNotSet = "A command does not change the settings of the tele holding the account."
	if c.kind == wire.KindCommand {
		switch f.Method {
		case wire.MethodSet:
			return wire.SetResult{Refused: commandsDoNotSet}, nil, true, nil
		case wire.MethodReload:
			return wire.ReloadResult{Failed: commandsDoNotSet}, nil, true, nil
		}
	}
	switch f.Method {
	case wire.MethodSettings:
		return c.host.Settings(), nil, true, nil
	case wire.MethodReload:
		return c.host.Reload(), nil, true, nil
	case wire.MethodStatus:
		return c.host.Status(), nil, true, nil
	case wire.MethodRestart:
		answer, then := c.host.Restart()
		return answer, then, true, nil
	case wire.MethodUninstall:
		return c.host.Uninstall(), nil, true, nil
	case wire.MethodSet:
		var p wire.SetParams
		if err := json.Unmarshal(f.Params, &p); err != nil || p.Key == "" {
			return nil, nil, true, fmt.Errorf("%w: %s names no setting", wire.ErrProtocol, f.Method)
		}
		return c.host.SetSetting(p), nil, true, nil
	}
	return nil, nil, false, nil
}

// reply answers request id with what call answered. A reply that cannot be
// written means the client is gone, which the reading side finds out by
// itself.
func (c *served) reply(id uint64, call protocol.Call, outs []reflect.Value) error {
	result, err := call.Result(outs)
	if err != nil {
		return c.fail(id, err)
	}
	_ = c.enc.Encode(wire.Reply(id, result))
	return nil
}

func (c *served) fail(id uint64, err error) error {
	_ = c.enc.Encode(wire.Fail(id, err))
	return nil
}
