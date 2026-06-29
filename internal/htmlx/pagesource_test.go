package htmlx

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// TestPageSourceMatchesReference extracts page wikitext from a captured live
// ajax response and asserts it is byte-identical to the .txt the original
// WikiComma stored for the same revision.
func TestPageSourceMatchesReference(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("testdata", "rev43.json"))
	if err != nil {
		t.Skipf("no captured ajax response: %v", err)
	}
	want, err := os.ReadFile(filepath.Join("testdata", "rev43.txt"))
	if err != nil {
		t.Skipf("no reference txt: %v", err)
	}

	var env struct {
		Body string `json:"body"`
	}
	if err := json.Unmarshal(raw, &env); err != nil {
		t.Fatalf("decode ajax json: %v", err)
	}

	doc, err := Parse(env.Body)
	if err != nil {
		t.Fatalf("parse html: %v", err)
	}
	got := PageSourceText(doc)

	if got != string(want) {
		t.Fatalf("page source mismatch: got %d bytes, want %d bytes\n%s",
			len(got), len(want), firstRuneDiff([]byte(got), want))
	}
	t.Logf("page source matches reference (%d bytes)", len(got))
}

func firstRuneDiff(a, b []byte) string {
	n := len(a)
	if len(b) < n {
		n = len(b)
	}
	for i := 0; i < n; i++ {
		if a[i] != b[i] {
			lo := i - 30
			if lo < 0 {
				lo = 0
			}
			return "first diff at byte " + itoa(i) + ":\n  got:  ..." + safe(a, lo, i+30) +
				"\n  want: ..." + safe(b, lo, i+30)
		}
	}
	return "(one is a prefix of the other; lengths differ)"
}

func safe(s []byte, lo, hi int) string {
	if lo < 0 {
		lo = 0
	}
	if hi > len(s) {
		hi = len(s)
	}
	return string(s[lo:hi])
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b [20]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	return string(b[i:])
}
