package sevenzip

import (
	"os"
	"path/filepath"
	"sort"
	"testing"
)

// TestAddPageRevisions verifies the page-revision invocation yields bare
// <rev>.txt members (matching the reference archives' layout).
func TestAddPageRevisions(t *testing.T) {
	if _, err := Bin(); err != nil {
		t.Skipf("no 7z available: %v", err)
	}
	base := t.TempDir()
	dir := filepath.Join(base, "pages", "somepage")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, n := range []string{"1.txt", "2.txt", "43.txt"} {
		if err := os.WriteFile(filepath.Join(dir, n), []byte("content "+n), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	archive := filepath.Join(base, "pages", "somepage.7z")
	if err := Add(archive, filepath.Join(dir, "*.txt"), false); err != nil {
		t.Fatal(err)
	}
	got, err := List(archive)
	if err != nil {
		t.Fatal(err)
	}
	sort.Strings(got)
	want := []string{"1.txt", "2.txt", "43.txt"}
	if !equal(got, want) {
		t.Fatalf("page members = %v, want %v", got, want)
	}
}

// TestAddForumThread verifies the forum invocation yields <post>/<rev>.html
// members.
func TestAddForumThread(t *testing.T) {
	if _, err := Bin(); err != nil {
		t.Skipf("no 7z available: %v", err)
	}
	base := t.TempDir()
	thread := filepath.Join(base, "forum", "123", "456")
	for _, p := range []string{"100/latest.html", "100/9189259.html", "200/latest.html"} {
		full := filepath.Join(thread, filepath.FromSlash(p))
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	archive := filepath.Join(base, "forum", "123", "456.7z")
	if err := Add(archive, filepath.Join(thread, "*.*"), true); err != nil {
		t.Fatal(err)
	}
	got, err := List(archive)
	if err != nil {
		t.Fatal(err)
	}
	sort.Strings(got)
	want := []string{"100/9189259.html", "100/latest.html", "200/latest.html"}
	if !equal(got, want) {
		t.Fatalf("forum members = %v, want %v", got, want)
	}
}

func equal(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
