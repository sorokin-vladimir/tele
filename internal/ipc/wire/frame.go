// Package wire is how a client and a host talk over a connection: one JSON
// object per line, in a handful of shapes. It knows nothing of the methods,
// windows and events that travel in those shapes - their payloads pass through
// it as raw JSON - and nothing of the connection either: it reads from a reader
// and writes to a writer, so it is tested on buffers (#206).
package wire

import (
	"encoding/json"
	"errors"
	"fmt"

	"github.com/sorokin-vladimir/tele/internal/telerr"
)

// Frame is one line on the wire. Which fields are set decides its shape:
//
//	request  {"id":7,"method":"Send","params":{...}}
//	reply    {"id":7,"result":{...}}
//	error    {"id":7,"error":{"kind":"rate_limited",...}}
//	delta    {"sub":3,"delta":{...}}
//	event    {"event":"notification","params":{...}}
//	cancel   {"method":"$cancel","params":{"id":7}}
//
// A request's id is the caller's and is echoed by the reply or the error that
// answers it; several requests may be in flight at once. A delta is addressed
// to a subscription instead, and an event to nobody in particular. An error
// with no id is about the connection itself. Keys are whole words: the wire is
// read by hand when something goes wrong, and a line's size is its payload's.
type Frame struct {
	ID     uint64          `json:"id,omitempty"`
	Method string          `json:"method,omitempty"`
	Params json.RawMessage `json:"params,omitempty"`
	Result json.RawMessage `json:"result,omitempty"`
	Error  *telerr.Error   `json:"error,omitempty"`
	Sub    uint64          `json:"sub,omitempty"`
	Delta  json.RawMessage `json:"delta,omitempty"`
	Event  string          `json:"event,omitempty"`
	// Background marks a request nobody is waiting on, so that the host's
	// calls to Telegram for it yield to the ones somebody is (#212).
	Background bool `json:"background,omitempty"`
}

// Shape is what a frame is.
type Shape int

const (
	ShapeRequest Shape = iota + 1
	ShapeReply
	ShapeError
	ShapeDelta
	ShapeEvent
	ShapeCancel
)

// MethodCancel calls off a request in flight: a context does not cross a
// connection, so leaving a chat or escaping a download says so in a frame of
// its own. Methods that begin with "$" are the protocol's, not the owner's.
const MethodCancel = "$cancel"

// ErrProtocol is a line that is not a frame: too long, not JSON, a key the
// protocol does not have, or keys that make no shape together. Nothing read
// after it can be trusted to be in order, so the connection ends with it.
var ErrProtocol = errors.New("wire: protocol error")

func protocolError(format string, args ...any) error {
	return fmt.Errorf("%w: %s", ErrProtocol, fmt.Sprintf(format, args...))
}

// Shape says what f is, or that it is no frame at all.
func (f Frame) Shape() (Shape, error) {
	switch {
	case f.Method == MethodCancel:
		if f.ID != 0 || f.Result != nil || f.Error != nil || f.Sub != 0 || f.Delta != nil || f.Event != "" {
			return 0, protocolError("a cancel carries nothing but the id it calls off")
		}
		if _, err := f.CancelledID(); err != nil {
			return 0, err
		}
		return ShapeCancel, nil
	case f.Method != "":
		if f.ID == 0 {
			return 0, protocolError("request %q has no id", f.Method)
		}
		if f.Result != nil || f.Error != nil || f.Sub != 0 || f.Delta != nil || f.Event != "" {
			return 0, protocolError("request %q carries more than a request", f.Method)
		}
		return ShapeRequest, nil
	case f.Result != nil:
		if f.ID == 0 || f.Params != nil || f.Error != nil || f.Sub != 0 || f.Delta != nil || f.Event != "" {
			return 0, protocolError("a reply is an id and a result")
		}
		return ShapeReply, nil
	case f.Error != nil:
		if f.Params != nil || f.Sub != 0 || f.Delta != nil || f.Event != "" {
			return 0, protocolError("an error is an id and an error")
		}
		return ShapeError, nil
	case f.Delta != nil:
		if f.Sub == 0 || f.ID != 0 || f.Params != nil || f.Event != "" {
			return 0, protocolError("a delta is a subscription and a delta")
		}
		return ShapeDelta, nil
	case f.Event != "":
		if f.ID != 0 || f.Sub != 0 {
			return 0, protocolError("an event %q is addressed to nobody", f.Event)
		}
		return ShapeEvent, nil
	default:
		return 0, protocolError("a line with no shape")
	}
}

// cancelParams is what a cancel carries.
type cancelParams struct {
	ID uint64 `json:"id"`
}

// CancelledID is the request a cancel calls off.
func (f Frame) CancelledID() (uint64, error) {
	var p cancelParams
	if err := strictUnmarshal(f.Params, &p); err != nil || p.ID == 0 {
		return 0, protocolError("a cancel names the request it calls off")
	}
	return p.ID, nil
}

// The constructors take payloads already encoded. What a payload is, and how it
// is encoded, is the caller's business; taking raw JSON rather than any value
// is what keeps bytes from being mistaken for a payload and sent as a string.

// Request is a call to method with params, under the caller's id.
func Request(id uint64, method string, params json.RawMessage) Frame {
	return Frame{ID: id, Method: method, Params: params}
}

// Reply answers request id with result. A call that answers with nothing
// still answers: its result is null.
func Reply(id uint64, result json.RawMessage) Frame {
	if result == nil {
		result = json.RawMessage("null")
	}
	return Frame{ID: id, Result: result}
}

// Fail answers request id with err. Id 0 is an error about the connection.
func Fail(id uint64, err error) Frame {
	e := telerr.From(err)
	if e == nil {
		e = &telerr.Error{Kind: telerr.Internal}
	}
	return Frame{ID: id, Error: e}
}

// Delta carries one change to subscription sub.
func Delta(sub uint64, delta json.RawMessage) Frame {
	return Frame{Sub: sub, Delta: delta}
}

// Event tells what happened, under its name.
func Event(name string, params json.RawMessage) Frame {
	return Frame{Event: name, Params: params}
}

// Cancel calls off request id.
func Cancel(id uint64) Frame {
	raw, _ := json.Marshal(cancelParams{ID: id})
	return Frame{Method: MethodCancel, Params: raw}
}
