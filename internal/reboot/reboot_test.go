package reboot

import "testing"

func TestShort(t *testing.T) {
	if got := Short("0edbb020-aaaa-bbbb"); got != "0edbb020…" {
		t.Fatalf("got %q", got)
	}
	if got := Short("abc"); got != "abc" {
		t.Fatalf("got %q", got)
	}
}
