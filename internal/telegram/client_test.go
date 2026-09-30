package telegram

import (
	"context"
	"errors"
	"net"
	"strings"
	"testing"
	"time"
)

func TestRedactToken(t *testing.T) {
	c := New("SECRETTOKEN", 1)
	err := c.wrapErr(fmtErr("Get https://api.telegram.org/botSECRETTOKEN/getUpdates: reset"))
	if err == nil || strings.Contains(err.Error(), "SECRETTOKEN") {
		t.Fatalf("token leaked: %v", err)
	}
	if !strings.Contains(err.Error(), "***") {
		t.Fatalf("expected redaction: %v", err)
	}
}

func TestFamilyFromResetError(t *testing.T) {
	v6 := errors.New("read tcp [2a0b:4140:c66d::2]:44446->[2001:67c:4e8:f004::9]:443: read: connection reset by peer")
	if got := familyFromError(v6); got != "tcp6" {
		t.Fatalf("ipv6 family: %s", got)
	}
	v4 := errors.New("read tcp 193.188.21.232:37032->149.154.166.110:443: read: connection reset by peer")
	if got := familyFromError(v4); got != "tcp4" {
		t.Fatalf("ipv4 family: %s", got)
	}
}

func TestStackSwitchesAfterTransportError(t *testing.T) {
	p := &stackPreference{}
	p.set("tcp6")
	p.noteTransportErr(resetErr("2001:67c:4e8:f004::9"))
	primary, _, pinned := p.order()
	if !pinned || primary != "tcp4" {
		t.Fatalf("after ipv6 reset: pinned=%v primary=%s", pinned, primary)
	}

	p.noteTransportErr(resetErr("2001:67c:4e8:f004::9"))
	primary, _, pinned = p.order()
	if !pinned || primary != "tcp4" {
		t.Fatalf("stale ipv6 error left tcp4: pinned=%v primary=%s", pinned, primary)
	}

	p.set("tcp6")
	p.noteTransportErr(context.DeadlineExceeded)
	primary, _, pinned = p.order()
	if !pinned || primary != "tcp4" {
		t.Fatalf("after deadline: pinned=%v primary=%s", pinned, primary)
	}

	p.noteTransportErr(errors.New("Bad Gateway"))
	primary, _, _ = p.order()
	if primary != "tcp4" {
		t.Fatalf("http error changed family to %s", primary)
	}
}

func resetErr(ip string) error {
	return &net.OpError{
		Op:   "read",
		Net:  "tcp",
		Addr: &net.TCPAddr{IP: net.ParseIP(ip), Port: 443},
		Err:  errors.New("connection reset by peer"),
	}
}

func TestDialPinnedIPv4(t *testing.T) {
	ln, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	go func() {
		c, err := ln.Accept()
		if err == nil {
			c.Close()
		}
	}()

	p := &stackPreference{}
	p.set("tcp4")
	conn, err := p.dial(context.Background(), &net.Dialer{Timeout: time.Second}, ln.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	if !strings.HasPrefix(conn.RemoteAddr().String(), "127.0.0.1:") {
		t.Fatalf("expected ipv4 dial, got %s", conn.RemoteAddr())
	}
}

func TestDialFallsBackToIPv6WhenIPv4Unusable(t *testing.T) {
	ln, err := net.Listen("tcp6", "[::1]:0")
	if err != nil {
		t.Skip(err)
	}
	defer ln.Close()
	go func() {
		c, err := ln.Accept()
		if err == nil {
			c.Close()
		}
	}()

	p := &stackPreference{}
	p.set("tcp4")
	conn, err := p.dial(context.Background(), &net.Dialer{Timeout: time.Second}, ln.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	if !strings.Contains(conn.RemoteAddr().String(), "::1") {
		t.Fatalf("expected ipv6 fallback, got %s", conn.RemoteAddr())
	}
	primary, _, pinned := p.order()
	if !pinned || primary != "tcp6" {
		t.Fatalf("fallback did not stick: pinned=%v primary=%s", pinned, primary)
	}
}

func TestDialReportsBothFailures(t *testing.T) {
	ln, err := net.Listen("tcp6", "[::1]:0")
	if err != nil {
		t.Skip(err)
	}
	addr := ln.Addr().String()
	ln.Close()

	p := &stackPreference{}
	p.set("tcp4")
	_, err = p.dial(context.Background(), &net.Dialer{Timeout: time.Second}, addr)
	if err == nil {
		t.Fatal("expected dial error")
	}
	msg := err.Error()
	if !strings.Contains(msg, "tcp4:") || !strings.Contains(msg, "tcp6:") {
		t.Fatalf("expected both families in error, got %v", err)
	}
}

func fmtErr(s string) error { return errStr(s) }

type errStr string

func (e errStr) Error() string { return string(e) }
