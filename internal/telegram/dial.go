package telegram

import (
	"context"
	"errors"
	"fmt"
	"net"
	"strings"
	"sync"
)

// stackPreference remembers which IP family reached the API.
// A dual-stack dial can finish the handshake on an address that then resets
// or hangs, and the transport keeps that socket. After such a failure the
// next dial stays on the other family until that one fails too.
type stackPreference struct {
	mu     sync.Mutex
	active string
}

func (p *stackPreference) dial(ctx context.Context, d *net.Dialer, addr string) (net.Conn, error) {
	primary, fallback, pinned := p.order()
	network := "tcp"
	if pinned {
		network = primary
	}
	conn, err := d.DialContext(ctx, network, addr)
	if err == nil {
		p.set(familyOf(conn))
		return conn, nil
	}
	if ctx.Err() != nil {
		return nil, err
	}
	conn, err2 := d.DialContext(ctx, fallback, addr)
	if err2 != nil {
		return nil, fmt.Errorf("%s: %v; %s: %v", network, err, fallback, err2)
	}
	p.set(familyOf(conn))
	return conn, nil
}

func (p *stackPreference) order() (primary, fallback string, pinned bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	switch p.active {
	case "tcp4":
		return "tcp4", "tcp6", true
	case "tcp6":
		return "tcp6", "tcp4", true
	default:
		return "tcp6", "tcp4", false
	}
}

func (p *stackPreference) set(family string) {
	if family != "tcp4" && family != "tcp6" {
		return
	}
	p.mu.Lock()
	p.active = family
	p.mu.Unlock()
}

func (p *stackPreference) noteTransportErr(err error) {
	if !isTransportFailure(err) {
		return
	}
	family := familyFromError(err)
	p.mu.Lock()
	defer p.mu.Unlock()
	if family == "" {
		family = p.active
		if family == "" {
			family = "tcp6"
		}
	}
	if p.active != "" && p.active != family {
		return
	}
	if family == "tcp4" {
		p.active = "tcp6"
		return
	}
	p.active = "tcp4"
}

func isTransportFailure(err error) bool {
	if err == nil || errors.Is(err, context.Canceled) {
		return false
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return true
	}
	var op *net.OpError
	if errors.As(err, &op) {
		return true
	}
	var ne net.Error
	return errors.As(err, &ne)
}

func familyOf(conn net.Conn) string {
	return familyOfAddr(conn.RemoteAddr())
}

func familyOfAddr(addr net.Addr) string {
	if addr == nil {
		return ""
	}
	host, _, err := net.SplitHostPort(addr.String())
	if err != nil {
		host = addr.String()
	}
	return familyOfIP(host)
}

func familyFromError(err error) string {
	var op *net.OpError
	if errors.As(err, &op) && op.Addr != nil {
		if family := familyOfAddr(op.Addr); family != "" {
			return family
		}
	}
	msg := err.Error()
	i := strings.LastIndex(msg, "->")
	if i < 0 {
		return ""
	}
	rest := msg[i+2:]
	if sp := strings.IndexAny(rest, " \t"); sp >= 0 {
		rest = rest[:sp]
	}
	rest = strings.TrimSuffix(rest, ":")
	if strings.HasPrefix(rest, "[") {
		end := strings.Index(rest, "]")
		if end > 1 {
			return familyOfIP(rest[1:end])
		}
		return ""
	}
	host, _, splitErr := net.SplitHostPort(rest)
	if splitErr != nil {
		return ""
	}
	return familyOfIP(host)
}

func familyOfIP(s string) string {
	ip := net.ParseIP(s)
	if ip == nil {
		return ""
	}
	if ip.To4() != nil {
		return "tcp4"
	}
	return "tcp6"
}
