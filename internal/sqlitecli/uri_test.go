package sqlitecli

import (
	"runtime"
	"strings"
	"testing"
)

func TestImmutableURI(t *testing.T) {
	got := ImmutableURI("/tmp/a b/contact.db")
	if !strings.HasPrefix(got, "file:///tmp/a%20b/contact.db?") {
		t.Fatalf("uri = %q", got)
	}
	if !strings.Contains(got, "mode=ro") || !strings.Contains(got, "immutable=1") {
		t.Fatalf("uri missing readonly options: %q", got)
	}
}

func TestImmutableURIWindowsDrivePath(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("windows path semantics only")
	}
	got := ImmutableURI(`C:\Users\me\a b\contact.db`)
	if !strings.HasPrefix(got, "file:///C:/Users/me/a%20b/contact.db?") {
		t.Fatalf("uri = %q", got)
	}
}
