package hosting

import (
	"context"
	"net"
	"sync"
	"time"

	"github.com/sorokin-vladimir/tele/internal/core"
	"github.com/sorokin-vladimir/tele/internal/ipc/serve"
	"github.com/sorokin-vladimir/tele/internal/ipc/wire"
)

// Server serves the endpoint: every client that connects gets an attachment of
// its own among the host's clients, for as long as its connection lasts.
type Server struct {
	l       net.Listener
	host    serve.Host
	clients *Clients
	ctx     context.Context
	cancel  context.CancelFunc
	conns   sync.WaitGroup

	mu sync.Mutex
	// served is the attachment of every connection still being served.
	served map[*core.Attachment]struct{}
}

// Serve accepts clients on l, answering each one's hello and its calls about
// the host from host, until Stop.
func Serve(ctx context.Context, l net.Listener, host serve.Host, clients *Clients) *Server {
	ctx, cancel := context.WithCancel(ctx)
	s := &Server{l: l, host: host, clients: clients, ctx: ctx, cancel: cancel, served: make(map[*core.Attachment]struct{})}
	s.conns.Add(1)
	go s.accept()
	return s
}

func (s *Server) accept() {
	defer s.conns.Done()
	for {
		conn, err := s.l.Accept()
		if err != nil {
			return // the listener closed: the host is stopping
		}
		att := core.NewAttachment()
		s.mu.Lock()
		s.served[att] = struct{}{}
		s.mu.Unlock()
		remove := s.clients.Add(att)
		s.conns.Add(1)
		go func() {
			defer s.conns.Done()
			_ = serve.Conn(s.ctx, conn, greeted{Attachment: att, clients: s.clients}, s.host)
			remove()
			s.mu.Lock()
			delete(s.served, att)
			s.mu.Unlock()
		}()
	}
}

// greeted is a client's attachment as it is served: its hello tells the host
// what kind of client it is (#213).
type greeted struct {
	*core.Attachment
	clients *Clients
}

func (g greeted) Greeted(h wire.Hello) { g.clients.introduce(g.Attachment, h.Kind) }

// Stop stops serving for reason. The endpoint goes first, so nobody new
// connects; every client still there is told the host is stopping and why, and
// its connection ends on that. A client that does not take it within grace is
// cut off.
func (s *Server) Stop(grace time.Duration, reason core.StopReason) {
	_ = s.l.Close()
	s.mu.Lock()
	for att := range s.served {
		// Telling waits on the client; one that never reads must not hold up
		// the others, nor the stop past grace.
		go att.Tell(core.HostStopping{Reason: reason})
	}
	s.mu.Unlock()

	done := make(chan struct{})
	go func() {
		s.conns.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(grace):
	}
	s.cancel()
	<-done
}
