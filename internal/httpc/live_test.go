package httpc

import (
	"net/url"
	"strings"
	"testing"
)

// TestLiveTokenAndAjax exercises the real Wikidot flow against pu-cn-wiki:
// fetch the recent-changes page (which sets wikidot_token7), then POST an ajax
// module using that token. Run with: go test ./internal/httpc -run TestLive -v
// Skipped automatically if the network is unavailable.
func TestLiveTokenAndAjax(t *testing.T) {
	c := New(8, nil)

	base := "https://pu-cn-wiki.wikidot.com"
	noFollow := false

	_, err := c.Get(base+"/system:recent-changes", &Options{FollowRedirects: &noFollow})
	if err != nil {
		t.Skipf("network unavailable: %v", err)
	}

	ajaxURL, _ := url.Parse(base + "/ajax-module-connector.php")
	tok := c.Jar.GetSpecific(ajaxURL, "wikidot_token7")
	if tok == nil {
		t.Fatalf("wikidot_token7 cookie was not captured")
	}
	t.Logf("token=%s domain=%s path=%s", tok.Value, tok.Domain, tok.Path)

	form := url.Values{}
	form.Set("options", `{"all": true}`)
	form.Set("page", "1")
	form.Set("perpage", "20")
	form.Set("moduleName", "changes/SiteChangesListModule")
	form.Set("wikidot_token7", tok.Value)

	body, err := c.Post(base+"/ajax-module-connector.php", &Options{
		Body: []byte(form.Encode()),
		Headers: map[string]string{
			"Content-Type": "application/x-www-form-urlencoded",
			"Referer":      base + "/system:recent-changes",
		},
		FollowRedirects: &noFollow,
	})
	if err != nil {
		t.Fatalf("ajax post failed: %v", err)
	}
	s := string(body)
	if !strings.Contains(s, `"status"`) {
		t.Fatalf("unexpected ajax response (first 200 bytes): %s", s[:min(200, len(s))])
	}
	if !strings.Contains(s, `"ok"`) {
		t.Fatalf("ajax status not ok: %s", s[:min(300, len(s))])
	}
	t.Logf("ajax ok, body %d bytes", len(body))
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
