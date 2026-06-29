package htmlx

import (
	"strconv"
	"strings"
)

// ForumContentInnerHTML reproduces, byte-for-byte, what the original stored as a
// forum post's latest.html: the innerHTML of that post's div.content.
//
// The original obtained this via node-html-parser's innerHTML getter, whose
// serialization is essentially the raw source with exactly these transforms:
//   - a start tag's self-closing slash is dropped (<br/> -> <br>, " />" -> " >");
//   - the first whitespace char after the tag name is normalized to one space;
//   - text, attributes and entities are preserved verbatim;
//   - comments are dropped (node-html-parser's default is comment:false).
//
// Reproducing it from the raw bytes (rather than re-serializing a parse tree) is
// the only way to match, because the parse tree loses incidental whitespace and
// the original entity forms.
func ForumContentInnerHTML(rawBody string, postID int) (string, bool) {
	anchor := `id="post-content-` + strconv.Itoa(postID) + `"`
	idx := strings.Index(rawBody, anchor)
	if idx < 0 {
		return "", false
	}
	gt := strings.IndexByte(rawBody[idx:], '>')
	if gt < 0 {
		return "", false
	}
	start := idx + gt + 1

	end := matchCloseDiv(rawBody, start)
	if end < 0 {
		return "", false
	}
	return serializeInner(rawBody[start:end]), true
}

// matchCloseDiv scans from pos (already inside one open div, depth 1) and returns
// the index of the </div> that closes it, or -1.
func matchCloseDiv(s string, pos int) int {
	depth := 1
	i := pos
	for i < len(s) {
		if s[i] != '<' {
			i++
			continue
		}
		tok, next := scanTag(s, i)
		switch tok.kind {
		case tokStart:
			if tok.name == "div" {
				depth++
			}
		case tokEnd:
			if tok.name == "div" {
				depth--
				if depth == 0 {
					return i
				}
			}
		}
		i = next
	}
	return -1
}

// serializeInner applies the node-html-parser innerHTML transforms over a raw
// inner-HTML slice.
func serializeInner(s string) string {
	var b strings.Builder
	i := 0
	for i < len(s) {
		if s[i] != '<' {
			j := strings.IndexByte(s[i:], '<')
			if j < 0 {
				b.WriteString(s[i:])
				break
			}
			b.WriteString(s[i : i+j])
			i += j
			continue
		}
		tok, next := scanTag(s, i)
		switch tok.kind {
		case tokText:
			b.WriteString(s[i:next]) // a stray '<' that is not a tag
		case tokComment:
			// dropped (comment:false)
		case tokEnd:
			b.WriteString(tok.raw)
		case tokStart:
			b.WriteString(rewriteStartTag(tok.name, tok.raw))
		}
		i = next
	}
	return b.String()
}

// rewriteStartTag turns a raw start tag into node-html-parser's serialized form.
func rewriteStartTag(name, raw string) string {
	// raw == "<" + name + between + ">"
	between := raw[1+len(name) : len(raw)-1]
	if strings.HasSuffix(between, "/") {
		between = between[:len(between)-1]
	}
	rawAttrs := ""
	if len(between) > 0 {
		rawAttrs = between[1:] // .slice(1): drop the first (whitespace) char
	}
	if rawAttrs == "" {
		return "<" + name + ">"
	}
	return "<" + name + " " + rawAttrs + ">"
}

type tokKind int

const (
	tokText tokKind = iota
	tokStart
	tokEnd
	tokComment
)

type token struct {
	kind tokKind
	raw  string
	name string
}

// scanTag parses the construct starting at s[i] (which is '<') and returns the
// token plus the index just past it. A '<' that does not begin a tag/comment is
// returned as a one-byte text token.
func scanTag(s string, i int) (token, int) {
	rest := s[i:]
	if strings.HasPrefix(rest, "<!--") {
		if k := strings.Index(rest, "-->"); k >= 0 {
			end := i + k + 3
			return token{kind: tokComment, raw: s[i:end]}, end
		}
		return token{kind: tokComment, raw: rest}, len(s)
	}

	closing := false
	p := i + 1
	if p < len(s) && s[p] == '/' {
		closing = true
		p++
	}
	if p >= len(s) || !isTagNameStart(s[p]) {
		// not a tag: emit the '<' as text
		return token{kind: tokText, raw: "<"}, i + 1
	}
	nameStart := p
	for p < len(s) && isTagNameChar(s[p]) {
		p++
	}
	name := s[nameStart:p]

	// advance to the closing '>', respecting quoted attribute values
	for p < len(s) {
		switch s[p] {
		case '"', '\'':
			q := s[p]
			p++
			for p < len(s) && s[p] != q {
				p++
			}
			if p < len(s) {
				p++ // consume closing quote
			}
		case '>':
			end := p + 1
			kind := tokStart
			if closing {
				kind = tokEnd
			}
			return token{kind: kind, raw: s[i:end], name: name}, end
		default:
			p++
		}
	}
	// unterminated tag: treat remainder as text
	return token{kind: tokText, raw: s[i:]}, len(s)
}

func isTagNameStart(c byte) bool {
	return (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z')
}

func isTagNameChar(c byte) bool {
	return (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') ||
		(c >= '0' && c <= '9') || c == '-' || c == '.' || c == ':' || c == '_'
}
