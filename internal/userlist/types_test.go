package userlist

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"wikit/internal/jsonx"
)

// TestUserBucketRoundtrip reads every reference user bucket, decodes each User,
// re-emits the whole bucket object, and asserts byte equality. Bucket keys are
// numeric, so the encoder's ascending-key ordering is exercised too.
func TestUserBucketRoundtrip(t *testing.T) {
	dir := filepath.Join("..", "..", "..", "data", "_users")
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Skipf("no _users data: %v", err)
	}
	var ok, bad int
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ".json") || name == "pending.json" {
			continue
		}
		path := filepath.Join(dir, name)
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Errorf("read %s: %v", path, err)
			continue
		}
		v, err := jsonx.Decode(raw)
		if err != nil {
			t.Errorf("decode %s: %v", path, err)
			continue
		}
		o := v.(*jsonx.Object)
		rebuilt := jsonx.NewObject()
		for _, key := range o.Keys() {
			uv, _ := o.Get(key)
			uo := uv.(*jsonx.Object)
			rebuilt.Set(key, readUser(uo).object())
		}
		out, err := jsonx.Marshal(rebuilt)
		if err != nil {
			t.Errorf("marshal %s: %v", path, err)
			continue
		}
		if bytes.Equal(raw, out) {
			ok++
		} else {
			bad++
			if bad <= 3 {
				t.Errorf("mismatch %s\n%s", path, diff(raw, out))
			}
		}
	}
	t.Logf("user buckets: %d ok, %d bad", ok, bad)
	if bad != 0 {
		t.Fatalf("%d buckets did not round-trip", bad)
	}
}

func diff(a, b []byte) string {
	n := len(a)
	if len(b) < n {
		n = len(b)
	}
	for i := 0; i < n; i++ {
		if a[i] != b[i] {
			lo := i - 50
			if lo < 0 {
				lo = 0
			}
			ah := i + 50
			if ah > len(a) {
				ah = len(a)
			}
			bh := i + 50
			if bh > len(b) {
				bh = len(b)
			}
			return "byte offset differs:\n  orig ..." + string(a[lo:ah]) + "...\n  ours ..." + string(b[lo:bh]) + "..."
		}
	}
	return "length differs"
}
