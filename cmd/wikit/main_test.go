package main

import "testing"

func TestWithScheme(t *testing.T) {
	cases := []struct {
		url, scheme, want string
	}{
		{"https://scp-wiki.wikidot.com", "http", "http://scp-wiki.wikidot.com"},
		{"http://example.com", "https", "https://example.com"},
		{"https://x.wikidot.com", "https", "https://x.wikidot.com"},
		{"example.com", "http", "http://example.com"}, // no scheme -> prepend
	}
	for _, c := range cases {
		if got := withScheme(c.url, c.scheme); got != c.want {
			t.Errorf("withScheme(%q, %q) = %q, want %q", c.url, c.scheme, got, c.want)
		}
	}
}
