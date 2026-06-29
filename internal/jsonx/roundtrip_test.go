package jsonx

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestRoundtripRealData decodes every .json file produced by the original
// Node.js WikiComma (placed under ../../../data) and re-encodes it with our
// encoder, asserting the bytes are identical. This proves the encoder
// reproduces JSON.stringify(x, null, 4) across all real metadata shapes:
// site/sitemap/file_map/page_id_map/page meta/forum/user buckets/cookies.
func TestRoundtripRealData(t *testing.T) {
	root := filepath.Join("..", "..", "..", "data")
	if _, err := os.Stat(root); err != nil {
		t.Skipf("reference data not present at %s", root)
	}

	var files []string
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() && strings.HasSuffix(path, ".json") {
			files = append(files, path)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk: %v", err)
	}
	if len(files) == 0 {
		t.Skip("no json files found")
	}

	var mismatches, ok int
	for _, f := range files {
		raw, err := os.ReadFile(f)
		if err != nil {
			t.Errorf("read %s: %v", f, err)
			continue
		}
		v, err := Decode(raw)
		if err != nil {
			t.Errorf("decode %s: %v", f, err)
			continue
		}
		out, err := Marshal(v)
		if err != nil {
			t.Errorf("marshal %s: %v", f, err)
			continue
		}
		if !bytes.Equal(raw, out) {
			mismatches++
			if mismatches <= 5 {
				t.Errorf("mismatch in %s\n%s", f, firstDiff(raw, out))
			}
		} else {
			ok++
		}
	}
	t.Logf("round-trip: %d ok, %d mismatch, %d total", ok, mismatches, len(files))
	if mismatches != 0 {
		t.Fatalf("%d files did not round-trip byte-for-byte", mismatches)
	}
}

func firstDiff(a, b []byte) string {
	n := len(a)
	if len(b) < n {
		n = len(b)
	}
	for i := 0; i < n; i++ {
		if a[i] != b[i] {
			lo := i - 40
			if lo < 0 {
				lo = 0
			}
			ahi := i + 40
			if ahi > len(a) {
				ahi = len(a)
			}
			bhi := i + 40
			if bhi > len(b) {
				bhi = len(b)
			}
			return "at byte " + itoa(i) + ":\n  orig: ..." + string(a[lo:ahi]) +
				"...\n  ours: ..." + string(b[lo:bhi]) + "..."
		}
	}
	if len(a) != len(b) {
		return "length differs: orig=" + itoa(len(a)) + " ours=" + itoa(len(b))
	}
	return "no byte diff found (??)"
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var b [20]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		b[i] = '-'
	}
	return string(b[i:])
}
