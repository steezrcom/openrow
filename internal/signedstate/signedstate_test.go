package signedstate

import (
	"bytes"
	"crypto/rand"
	"strings"
	"testing"
)

func mustKey(t *testing.T) []byte {
	t.Helper()
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		t.Fatalf("rand: %v", err)
	}
	return b
}

func TestRoundTrip(t *testing.T) {
	s, err := New(mustKey(t))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	in := []byte("hello world")
	tok := s.Build(in)
	out, err := s.Open(tok)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if !bytes.Equal(in, out) {
		t.Fatalf("got %q want %q", out, in)
	}
}

func TestRejectsTamperedPayload(t *testing.T) {
	s, err := New(mustKey(t))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	tok := s.Build([]byte("payload"))
	// Flip a byte in the payload portion.
	dot := strings.IndexByte(tok, '.')
	if dot < 1 {
		t.Fatalf("no dot in token: %s", tok)
	}
	// Replace first payload char with a different one that's still valid base64url.
	swap := byte('A')
	if tok[0] == 'A' {
		swap = 'B'
	}
	bad := string(swap) + tok[1:]
	if _, err := s.Open(bad); err == nil {
		t.Fatalf("expected error on tampered payload")
	}
}

func TestRejectsTamperedSignature(t *testing.T) {
	s, err := New(mustKey(t))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	tok := s.Build([]byte("payload"))
	// Flip the last char of the signature.
	last := len(tok) - 1
	swap := byte('A')
	if tok[last] == 'A' {
		swap = 'B'
	}
	bad := tok[:last] + string(swap)
	if _, err := s.Open(bad); err == nil {
		t.Fatalf("expected error on tampered signature")
	}
}

func TestRejectsWrongKey(t *testing.T) {
	a, _ := New(mustKey(t))
	b, _ := New(mustKey(t))
	tok := a.Build([]byte("payload"))
	if _, err := b.Open(tok); err == nil {
		t.Fatalf("expected error when verified with a different key")
	}
}

func TestRejectsMalformedToken(t *testing.T) {
	s, _ := New(mustKey(t))
	cases := []string{
		"",
		"no-dot",
		".only-sig",
		"only-payload.",
		"!!!.!!!",
	}
	for _, c := range cases {
		if _, err := s.Open(c); err == nil {
			t.Fatalf("expected error on %q", c)
		}
	}
}

func TestNewRejectsEmptyKey(t *testing.T) {
	if _, err := New(nil); err == nil {
		t.Fatalf("expected error on empty key")
	}
	if _, err := New([]byte{}); err == nil {
		t.Fatalf("expected error on empty key")
	}
}

func TestDeriveIsDomainSeparated(t *testing.T) {
	root := mustKey(t)
	a := Derive(root, "domain-a")
	b := Derive(root, "domain-b")
	if bytes.Equal(a, b) {
		t.Fatalf("Derive returned same key for different domains")
	}
	if len(a) != 32 {
		t.Fatalf("Derive should return 32-byte key, got %d", len(a))
	}
}

func TestDeriveIsStable(t *testing.T) {
	root := []byte("fixed root for this test only")
	a := Derive(root, "x")
	b := Derive(root, "x")
	if !bytes.Equal(a, b) {
		t.Fatalf("Derive is not deterministic")
	}
}

func TestKeyIsCopied(t *testing.T) {
	key := mustKey(t)
	s, _ := New(key)
	tok := s.Build([]byte("p"))
	// Zero the caller's slice; Open should still work.
	for i := range key {
		key[i] = 0
	}
	if _, err := s.Open(tok); err != nil {
		t.Fatalf("Open failed after caller zeroed key: %v", err)
	}
}
