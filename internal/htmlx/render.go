package htmlx

import (
	"strings"

	"github.com/PuerkitoBio/goquery"
	"golang.org/x/net/html"
)

// voidElements are serialized without a closing slash (<br>, not <br/>), matching
// node-html-parser and Wikidot's own HTML — the one way goquery's renderer
// differs from the original.
var voidElements = map[string]bool{
	"area": true, "base": true, "br": true, "col": true, "embed": true,
	"hr": true, "img": true, "input": true, "link": true, "meta": true,
	"param": true, "source": true, "track": true, "wbr": true,
}

// rawTextElements have their contents emitted verbatim (no escaping).
var rawTextElements = map[string]bool{
	"script": true, "style": true, "noscript": true, "iframe": true,
	"noembed": true, "noframes": true, "plaintext": true, "textarea": true,
	"title": true, "xmp": true,
}

// InnerHTML returns the innerHTML of the first node in sel, serialized the way
// the original WikiComma stored forum post bodies. Text and attribute escaping
// follow golang.org/x/net/html (verified against reference output); only void
// element serialization is adjusted to the slash-less form.
func InnerHTML(sel *goquery.Selection) string {
	if sel.Length() == 0 {
		return ""
	}
	var b strings.Builder
	for c := sel.Nodes[0].FirstChild; c != nil; c = c.NextSibling {
		renderNode(&b, c)
	}
	return b.String()
}

func renderNode(b *strings.Builder, n *html.Node) {
	switch n.Type {
	case html.TextNode:
		if n.Parent != nil && n.Parent.Type == html.ElementNode && rawTextElements[n.Parent.Data] {
			b.WriteString(n.Data)
		} else {
			escape(b, n.Data)
		}
	case html.ElementNode:
		b.WriteByte('<')
		b.WriteString(n.Data)
		for _, a := range n.Attr {
			b.WriteByte(' ')
			if a.Namespace != "" {
				b.WriteString(a.Namespace)
				b.WriteByte(':')
			}
			b.WriteString(a.Key)
			b.WriteString(`="`)
			escape(b, a.Val)
			b.WriteByte('"')
		}
		b.WriteByte('>')
		if voidElements[n.Data] {
			return
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			renderNode(b, c)
		}
		b.WriteString("</")
		b.WriteString(n.Data)
		b.WriteByte('>')
	case html.CommentNode:
		b.WriteString("<!--")
		b.WriteString(n.Data)
		b.WriteString("-->")
	case html.DoctypeNode:
		b.WriteString("<!DOCTYPE ")
		b.WriteString(n.Data)
		b.WriteByte('>')
	}
}

// escape replicates golang.org/x/net/html's render-time escaping exactly, so the
// matching cases stay matching: & ' < > " \r are escaped, nothing else.
func escape(b *strings.Builder, s string) {
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case '&':
			b.WriteString("&amp;")
		case '\'':
			b.WriteString("&#39;")
		case '<':
			b.WriteString("&lt;")
		case '>':
			b.WriteString("&gt;")
		case '"':
			b.WriteString("&#34;")
		case '\r':
			b.WriteString("&#13;")
		default:
			b.WriteByte(s[i])
		}
	}
}
