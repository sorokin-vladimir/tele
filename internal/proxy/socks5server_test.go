package proxy_test

import (
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"strconv"
	"sync"
	"testing"
)

// socksServer is a SOCKS5 server with just enough of the protocol to answer a
// connection and say where it was asked to go. It exists so that "the proxy is
// actually used" can be asserted rather than assumed: the failure this guards
// against is a dialer that is built correctly and then not wired into the
// resolver, which no amount of inspecting the dialer would catch.
type socksServer struct {
	ln       net.Listener
	user     string // empty: the server asks for no credentials
	password string

	mu      sync.Mutex
	targets []string
	logins  []string
}

func newSocksServer(t *testing.T, user, password string) *socksServer {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	s := &socksServer{ln: ln, user: user, password: password}
	t.Cleanup(func() { _ = ln.Close() })
	go s.serve()
	return s
}

func (s *socksServer) addr() (host string, port int) {
	host, p, _ := net.SplitHostPort(s.ln.Addr().String())
	port, _ = strconv.Atoi(p)
	return host, port
}

// seen returns what the server was asked for: the addresses connections were
// opened to, and the credentials they arrived with.
func (s *socksServer) seen() (targets, logins []string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]string(nil), s.targets...), append([]string(nil), s.logins...)
}

func (s *socksServer) serve() {
	for {
		conn, err := s.ln.Accept()
		if err != nil {
			return
		}
		go func() {
			defer conn.Close() //nolint:errcheck // a test's connection, nothing depends on closing it
			_ = s.handle(conn)
		}()
	}
}

func (s *socksServer) handle(conn net.Conn) error {
	greeting := make([]byte, 2)
	if _, err := io.ReadFull(conn, greeting); err != nil {
		return err
	}
	methods := make([]byte, greeting[1])
	if _, err := io.ReadFull(conn, methods); err != nil {
		return err
	}

	if s.user != "" {
		if _, err := conn.Write([]byte{0x05, 0x02}); err != nil {
			return err
		}
		if err := s.authenticate(conn); err != nil {
			return err
		}
	} else if _, err := conn.Write([]byte{0x05, 0x00}); err != nil {
		return err
	}

	target, err := readRequest(conn)
	if err != nil {
		return err
	}
	s.mu.Lock()
	s.targets = append(s.targets, target)
	s.mu.Unlock()

	upstream, err := net.Dial("tcp", target)
	if err != nil {
		_, _ = conn.Write([]byte{0x05, 0x01, 0x00, 0x01, 0, 0, 0, 0, 0, 0})
		return err
	}
	defer upstream.Close() //nolint:errcheck // a test's connection, nothing depends on closing it
	if _, err := conn.Write([]byte{0x05, 0x00, 0x00, 0x01, 0, 0, 0, 0, 0, 0}); err != nil {
		return err
	}

	go io.Copy(upstream, conn) //nolint:errcheck // ends with the connection, the other direction reports
	_, err = io.Copy(conn, upstream)
	return err
}

func (s *socksServer) authenticate(conn net.Conn) error {
	header := make([]byte, 2) // version, username length
	if _, err := io.ReadFull(conn, header); err != nil {
		return err
	}
	user := make([]byte, header[1])
	if _, err := io.ReadFull(conn, user); err != nil {
		return err
	}
	size := make([]byte, 1)
	if _, err := io.ReadFull(conn, size); err != nil {
		return err
	}
	password := make([]byte, size[0])
	if _, err := io.ReadFull(conn, password); err != nil {
		return err
	}

	s.mu.Lock()
	s.logins = append(s.logins, string(user)+":"+string(password))
	s.mu.Unlock()

	if string(user) != s.user || string(password) != s.password {
		_, _ = conn.Write([]byte{0x01, 0x01})
		return fmt.Errorf("socks5: wrong credentials")
	}
	_, err := conn.Write([]byte{0x01, 0x00})
	return err
}

// readRequest reads a CONNECT and returns the address it names.
func readRequest(conn net.Conn) (string, error) {
	header := make([]byte, 4) // version, command, reserved, address type
	if _, err := io.ReadFull(conn, header); err != nil {
		return "", err
	}

	var host string
	switch header[3] {
	case 0x01:
		addr := make([]byte, net.IPv4len)
		if _, err := io.ReadFull(conn, addr); err != nil {
			return "", err
		}
		host = net.IP(addr).String()
	case 0x03:
		size := make([]byte, 1)
		if _, err := io.ReadFull(conn, size); err != nil {
			return "", err
		}
		name := make([]byte, size[0])
		if _, err := io.ReadFull(conn, name); err != nil {
			return "", err
		}
		host = string(name)
	case 0x04:
		addr := make([]byte, net.IPv6len)
		if _, err := io.ReadFull(conn, addr); err != nil {
			return "", err
		}
		host = net.IP(addr).String()
	default:
		return "", fmt.Errorf("socks5: address type %#02x", header[3])
	}

	port := make([]byte, 2)
	if _, err := io.ReadFull(conn, port); err != nil {
		return "", err
	}
	return net.JoinHostPort(host, strconv.Itoa(int(binary.BigEndian.Uint16(port)))), nil
}
