package wiki

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"wikit/internal/jsonx"
)

// TestPageMetaRoundtrip reads every reference page metadata file, decodes it into
// the working struct, re-emits it via the ordered builder (choosing the
// construction branch from the original key order), and asserts the bytes match.
// This validates both the struct mapping and the two field-order branches.
func TestPageMetaRoundtrip(t *testing.T) {
	dir := filepath.Join("..", "..", "..", "data", "pu-cn-wiki", "meta", "pages")
	roundtripDir(t, dir, func(o *jsonx.Object) *jsonx.Object {
		return readPageMeta(o).object()
	})
}

func TestForumThreadRoundtrip(t *testing.T) {
	base := filepath.Join("..", "..", "..", "data", "pu-cn-wiki", "meta", "forum")
	entries, err := os.ReadDir(base)
	if err != nil {
		t.Skipf("no forum data: %v", err)
	}
	for _, e := range entries {
		if !e.IsDir() || e.Name() == "category" {
			continue
		}
		roundtripDir(t, filepath.Join(base, e.Name()), func(o *jsonx.Object) *jsonx.Object {
			return readForumThread(o).object()
		})
	}
}

func TestForumCategoryRoundtrip(t *testing.T) {
	dir := filepath.Join("..", "..", "..", "data", "pu-cn-wiki", "meta", "forum", "category")
	roundtripDir(t, dir, func(o *jsonx.Object) *jsonx.Object {
		return readForumCategory(o).object()
	})
}

func TestSiteMetaRoundtrip(t *testing.T) {
	path := filepath.Join("..", "..", "..", "data", "pu-cn-wiki", "meta", "site.json")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Skipf("no site.json: %v", err)
	}
	v, err := jsonx.Decode(raw)
	if err != nil {
		t.Fatal(err)
	}
	o := v.(*jsonx.Object)
	sm := SiteMeta{
		Domain:   getStr(o, "domain"),
		SiteID:   asInt64(mustGet(o, "site_id")),
		Slug:     getStr(o, "slug"),
		HomePage: getStr(o, "home_page"),
		Language: getStr(o, "language"),
	}
	out, _ := jsonx.Marshal(sm.object())
	if !bytes.Equal(raw, out) {
		t.Fatalf("site.json mismatch:\n got: %s\nwant: %s", out, raw)
	}
}

func roundtripDir(t *testing.T, dir string, rebuild func(*jsonx.Object) *jsonx.Object) {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Skipf("no data at %s: %v", dir, err)
	}
	var ok, bad int
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		path := filepath.Join(dir, e.Name())
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
		o, isObj := v.(*jsonx.Object)
		if !isObj {
			continue
		}
		out, err := jsonx.Marshal(rebuild(o))
		if err != nil {
			t.Errorf("marshal %s: %v", path, err)
			continue
		}
		if bytes.Equal(raw, out) {
			ok++
		} else {
			bad++
			if bad <= 3 {
				t.Errorf("mismatch %s\n%s", path, firstDiff(raw, out))
			}
		}
	}
	t.Logf("%s: %d ok, %d bad", dir, ok, bad)
	if bad != 0 {
		t.Fatalf("%d files did not round-trip", bad)
	}
}

func firstDiff(a, b []byte) string {
	n := len(a)
	if len(b) < n {
		n = len(b)
	}
	for i := 0; i < n; i++ {
		if a[i] != b[i] {
			lo := i - 60
			if lo < 0 {
				lo = 0
			}
			ah := i + 60
			if ah > len(a) {
				ah = len(a)
			}
			bh := i + 60
			if bh > len(b) {
				bh = len(b)
			}
			return "byte " + itoa(i) + ":\n  orig ..." + string(a[lo:ah]) + "...\n  ours ..." + string(b[lo:bh]) + "..."
		}
	}
	if len(a) != len(b) {
		return "length: orig=" + itoa(len(a)) + " ours=" + itoa(len(b))
	}
	return "?"
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
