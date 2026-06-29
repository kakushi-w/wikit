package htmlx

import (
	"regexp"

	"github.com/PuerkitoBio/goquery"
)

var (
	useridMatcher   = regexp.MustCompile(`WIKIDOT\.page\.listeners\.userInfo\((\d+)\)`)
	usernameMatcher = regexp.MustCompile(`user:info/(.*)`)
)

// ExtractUser mirrors the original WikiDot.extractUser: it pulls a numeric user
// id and slug from a printuser element, falling back to span.deleted[data-id]
// and the element's own data-id. Returns (nil, "") when no user is present.
func ExtractUser(sel *goquery.Selection) (*int64, string) {
	if sel == nil || sel.Length() == 0 {
		return nil, ""
	}
	a := sel.Find("a").First()

	var id *int64
	if onclick, ok := a.Attr("onclick"); ok {
		if m := useridMatcher.FindStringSubmatch(onclick); m != nil {
			id = parseI64(m[1])
		}
	}
	username := ""
	if href, ok := a.Attr("href"); ok {
		if m := usernameMatcher.FindStringSubmatch(href); m != nil {
			username = m[1]
		}
	}

	if id == nil {
		if v, ok := sel.Find("span.deleted").First().Attr("data-id"); ok {
			id = parseI64(v)
		}
	}
	if id == nil {
		if v, ok := sel.Attr("data-id"); ok {
			id = parseI64(v)
		}
	}
	if id == nil {
		return nil, ""
	}
	return id, username
}

func parseI64(s string) *int64 {
	var n int64
	neg := false
	i := 0
	if i < len(s) && (s[i] == '-' || s[i] == '+') {
		neg = s[i] == '-'
		i++
	}
	if i >= len(s) {
		return nil
	}
	for ; i < len(s); i++ {
		c := s[i]
		if c < '0' || c > '9' {
			return nil
		}
		n = n*10 + int64(c-'0')
	}
	if neg {
		n = -n
	}
	return &n
}
