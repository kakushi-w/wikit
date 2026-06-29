package httpc

import (
	"net/url"
	"os"
	"path/filepath"
	"testing"
)

// TestDumpForumPosts captures the raw ForumViewThreadPostsModule response for a
// known thread, for offline comparison of div.content innerHTML against the
// reference latest.html files.
func TestDumpForumPosts(t *testing.T) {
	c := New(8, nil)
	base := "https://pu-cn-wiki.wikidot.com"
	noFollow := false
	if _, err := c.Get(base+"/system:recent-changes", &Options{FollowRedirects: &noFollow}); err != nil {
		t.Skipf("network unavailable: %v", err)
	}
	ajaxURL, _ := url.Parse(base + "/ajax-module-connector.php")
	tok := c.Jar.GetSpecific(ajaxURL, "wikidot_token7")
	if tok == nil {
		t.Fatal("no token")
	}

	form := url.Values{}
	form.Set("t", "17623753")
	form.Set("pageNo", "1")
	form.Set("order", "")
	form.Set("moduleName", "forum/ForumViewThreadPostsModule")
	form.Set("wikidot_token7", tok.Value)

	body, err := c.Post(base+"/ajax-module-connector.php", &Options{
		Body:            []byte(form.Encode()),
		Headers:         map[string]string{"Content-Type": "application/x-www-form-urlencoded", "Referer": base},
		FollowRedirects: &noFollow,
	})
	if err != nil {
		t.Fatalf("post: %v", err)
	}
	out := filepath.Join(os.TempDir(), "forum_posts.json")
	if err := os.WriteFile(out, body, 0644); err != nil {
		t.Fatal(err)
	}
	t.Logf("wrote %s (%d bytes)", out, len(body))
}
