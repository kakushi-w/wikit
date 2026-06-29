package htmlx

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/PuerkitoBio/goquery"
)

// TestForumContentInnerHTML compares div.content innerHTML extracted from a live
// ForumViewThreadPostsModule response against the reference latest.html files.
// This is the riskiest parity check: forum post bodies are stored as innerHTML,
// so re-serialization must reproduce the original whitespace and markup exactly.
func TestForumContentInnerHTML(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("testdata", "forum_posts.json"))
	if err != nil {
		t.Skipf("no captured forum response: %v", err)
	}
	var env struct {
		Body string `json:"body"`
	}
	if err := json.Unmarshal(raw, &env); err != nil {
		t.Fatalf("decode: %v", err)
	}
	doc, err := Parse(env.Body)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}

	var compared, matched int
	var firstMismatch string
	doc.Find("div.post").Each(func(_ int, post *goquery.Selection) {
		id, ok := post.Attr("id")
		if !ok || !strings.HasPrefix(id, "post-") {
			return
		}
		postID := strings.TrimPrefix(id, "post-")
		pid, err := strconv.Atoi(postID)
		if err != nil {
			return
		}
		got, ok := ForumContentInnerHTML(env.Body, pid)
		if !ok {
			t.Errorf("post %s: content not found in raw body", postID)
			return
		}
		refPath := filepath.Join("testdata", "forum_ref", postID, "latest.html")
		want, err := os.ReadFile(refPath)
		if err != nil {
			return // post not in reference (added since backup)
		}
		compared++
		if got == string(want) {
			matched++
		} else if firstMismatch == "" {
			firstMismatch = "post " + postID + ":\n  got:  " + escapeWS(got) + "\n  want: " + escapeWS(string(want))
		}
	})

	t.Logf("forum innerHTML: %d/%d matched reference", matched, compared)
	if compared == 0 {
		t.Skip("no overlapping posts between live and reference")
	}
	if matched != compared {
		t.Fatalf("%d/%d forum posts did not match byte-for-byte\n%s", compared-matched, compared, firstMismatch)
	}
}

func escapeWS(s string) string {
	r := strings.NewReplacer("\n", "\\n", "\t", "\\t", "\r", "\\r")
	if len(s) > 300 {
		s = s[:300] + "..."
	}
	return r.Replace(s)
}
