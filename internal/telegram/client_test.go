package telegram

import (
	"strings"
	"testing"
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

func fmtErr(s string) error { return errStr(s) }

type errStr string

func (e errStr) Error() string { return string(e) }
