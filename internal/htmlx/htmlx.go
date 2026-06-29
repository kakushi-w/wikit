// Package htmlx provides the HTML parsing and extraction used to turn Wikidot
// ajax responses into the exact text/markup the original WikiComma stored. It is
// built on goquery (cascadia selectors over golang.org/x/net/html), which mirrors
// the querySelector/textContent semantics the original relied on.
package htmlx

import (
	"strings"

	"github.com/PuerkitoBio/goquery"
)

// Parse parses an HTML fragment (such as an ajax module body) into a document.
func Parse(html string) (*goquery.Document, error) {
	return goquery.NewDocumentFromReader(strings.NewReader(html))
}

// PageSourceText returns the page wikitext exactly as the original wrote it to
// <revision>.txt: the textContent of div.page-source (entities decoded, <br/>
// contributing nothing). Returns "" if the source div is absent, matching the
// original's fallback.
func PageSourceText(doc *goquery.Document) string {
	sel := doc.Find("div.page-source")
	if sel.Length() == 0 {
		return ""
	}
	return sel.First().Text()
}
