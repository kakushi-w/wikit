package httpc

import (
	"net/url"
	"os"
	"path/filepath"
	"testing"
)

// TestDumpRevisionSource fetches a known page-source revision and writes the raw
// ajax JSON body to /tmp for offline inspection while building the HTML
// extractor. Not a real test; run explicitly.
func TestDumpRevisionSource(t *testing.T) {
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
	form.Set("revision_id", "1543136789") // level-pu-11 revision 43
	form.Set("moduleName", "history/PageSourceModule")
	form.Set("wikidot_token7", tok.Value)

	body, err := c.Post(base+"/ajax-module-connector.php", &Options{
		Body:            []byte(form.Encode()),
		Headers:         map[string]string{"Content-Type": "application/x-www-form-urlencoded", "Referer": base},
		FollowRedirects: &noFollow,
	})
	if err != nil {
		t.Fatalf("post: %v", err)
	}
	out := filepath.Join(os.TempDir(), "rev43.json")
	if err := os.WriteFile(out, body, 0644); err != nil {
		t.Fatal(err)
	}
	t.Logf("wrote %s (%d bytes)", out, len(body))
}
