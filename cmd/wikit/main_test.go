package main

import "testing"

func TestResolveWikiURL(t *testing.T) {
	cases := []struct {
		name, url, defScheme, want string
	}{
		// url with an explicit scheme is honored verbatim (per-wiki control)
		{"a", "https://a.wikidot.com", "http", "https://a.wikidot.com"},
		{"b", "http://b.example.com", "https", "http://b.example.com"},
		// scheme-less url gets the default scheme
		{"c", "c.example.com", "http", "http://c.example.com"},
		{"d", "//d.example.com", "https", "https://d.example.com"},
		// empty url is derived from the name using the default scheme
		{"scp-wiki", "", "https", "https://scp-wiki.wikidot.com"},
		{"old", "", "http", "http://old.wikidot.com"},
	}
	for _, c := range cases {
		if got := resolveWikiURL(c.name, c.url, c.defScheme); got != c.want {
			t.Errorf("resolveWikiURL(%q, %q, %q) = %q, want %q", c.name, c.url, c.defScheme, got, c.want)
		}
	}
}
