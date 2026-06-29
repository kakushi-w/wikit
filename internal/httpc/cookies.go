package httpc

import (
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"wikit/internal/jsonx"
)

// Cookie mirrors the original HTTPCookie. Expires is nil for session cookies.
type Cookie struct {
	Name    string
	Value   string
	Path    string // "" means "matches any path"
	Domain  string // "" means "matches any domain"
	Expires *time.Time
	Secure  bool
}

// CookieJar is a minimal, WikiComma-compatible cookie store. It deliberately
// does not implement public-suffix rules (Go's net/http/cookiejar does), to
// match the original's plain endsWith domain matching.
type CookieJar struct {
	mu      sync.Mutex
	cookies []*Cookie
}

func NewCookieJar() *CookieJar { return &CookieJar{} }

// get returns cookies applicable to u (caller holds no lock).
func (j *CookieJar) get(u *url.URL) []*Cookie {
	secure := u.Scheme == "https"
	path := u.Path
	if path == "" {
		path = "/"
	}
	host := u.Host
	now := time.Now()

	var out []*Cookie
	for _, c := range j.cookies {
		if (!c.Secure || secure) &&
			(c.Path == "" || strings.HasPrefix(path, c.Path)) &&
			(c.Domain == "" || strings.HasSuffix(host, c.Domain)) &&
			(c.Expires == nil || !c.Expires.Before(now)) {
			out = append(out, c)
		}
	}
	return out
}

// GetSpecific returns the named cookie applicable to u, or nil.
func (j *CookieJar) GetSpecific(u *url.URL, name string) *Cookie {
	j.mu.Lock()
	defer j.mu.Unlock()
	for _, c := range j.get(u) {
		if c.Name == name {
			return c
		}
	}
	return nil
}

// Build renders the Cookie request header value for u.
func (j *CookieJar) Build(u *url.URL) string {
	j.mu.Lock()
	defer j.mu.Unlock()
	var parts []string
	for _, c := range j.get(u) {
		parts = append(parts, c.Name+"="+c.Value)
	}
	return strings.Join(parts, "; ")
}

// Put parses a single Set-Cookie header value and stores it, defaulting the
// domain to defaultDomain. Returns true if a cookie was stored.
func (j *CookieJar) Put(setCookie, defaultDomain string) bool {
	split := strings.Split(setCookie, ";")

	var name, value string
	var path string
	var domain = defaultDomain
	var expires *time.Time
	secure := false
	firstPair := true

	for _, token := range split {
		trim := strings.TrimSpace(token)
		lower := strings.ToLower(trim)

		switch {
		case lower == "secure":
			secure = true
		case lower == "httponly":
			// no meaningful value
		case strings.Contains(trim, "="):
			eq := strings.IndexByte(trim, '=')
			k := trim[:eq]
			v := trim[eq+1:]
			if firstPair {
				firstPair = false
				name = k
				value = v
			} else {
				switch strings.ToLower(k) {
				case "expires":
					if t, err := parseCookieDate(v); err == nil {
						expires = &t
					}
				case "domain":
					domain = v
				case "path":
					path = v
				case "max-age":
					if d, err := strconv.Atoi(v); err == nil {
						var t time.Time
						if d == 0 {
							t = time.Unix(0, 0)
						} else {
							t = time.Now().Add(time.Duration(d) * time.Millisecond)
						}
						expires = &t
					}
				}
			}
		}
	}

	if name == "" {
		return false
	}

	c := &Cookie{Name: name, Value: value, Path: path, Domain: domain, Expires: expires, Secure: secure}

	j.mu.Lock()
	defer j.mu.Unlock()
	// Replace any existing cookie with the same domain+name+path.
	kept := j.cookies[:0]
	for _, ex := range j.cookies {
		if ex.Domain == c.Domain && ex.Name == c.Name && ex.Path == c.Path {
			continue
		}
		kept = append(kept, ex)
	}
	j.cookies = append(kept, c)
	return true
}

// Invalidate clears all cookies (used when the server rejects our token).
func (j *CookieJar) Invalidate() {
	j.mu.Lock()
	defer j.mu.Unlock()
	j.cookies = nil
}

// Save renders the jar in the exact shape the original writes to
// http_cookies.json: an array of {value,name,path,domain,expires,secure} with
// expires as an ISO-8601 string (or null), via JSON.stringify(x, null, 4).
func (j *CookieJar) Save() ([]byte, error) {
	j.mu.Lock()
	defer j.mu.Unlock()

	arr := make([]any, 0, len(j.cookies))
	for _, c := range j.cookies {
		o := jsonx.NewObject()
		o.Set("value", c.Value)
		o.Set("name", c.Name)
		o.Set("path", nullable(c.Path))
		o.Set("domain", nullable(c.Domain))
		if c.Expires == nil {
			o.Set("expires", nil)
		} else {
			o.Set("expires", isoDate(*c.Expires))
		}
		o.Set("secure", c.Secure)
		arr = append(arr, o)
	}
	return jsonx.Marshal(arr)
}

// Load replaces the jar contents from previously saved JSON.
func (j *CookieJar) Load(data []byte) error {
	v, err := jsonx.Decode(data)
	if err != nil {
		return err
	}
	arr, ok := v.([]any)
	if !ok {
		return nil
	}
	var cookies []*Cookie
	for _, el := range arr {
		o, ok := el.(*jsonx.Object)
		if !ok {
			continue
		}
		c := &Cookie{}
		if s, ok := getString(o, "value"); ok {
			c.Value = s
		}
		if s, ok := getString(o, "name"); ok {
			c.Name = s
		}
		if s, ok := getString(o, "path"); ok {
			c.Path = s
		}
		if s, ok := getString(o, "domain"); ok {
			c.Domain = s
		}
		if raw, ok := o.Get("expires"); ok {
			if s, isStr := raw.(string); isStr {
				if t, err := time.Parse(time.RFC3339, s); err == nil {
					c.Expires = &t
				}
			}
		}
		if raw, ok := o.Get("secure"); ok {
			if b, isB := raw.(bool); isB {
				c.Secure = b
			}
		}
		cookies = append(cookies, c)
	}
	j.mu.Lock()
	j.cookies = cookies
	j.mu.Unlock()
	return nil
}

func nullable(s string) any {
	if s == "" {
		return nil
	}
	return s
}

func getString(o *jsonx.Object, key string) (string, bool) {
	if raw, ok := o.Get(key); ok {
		if s, isStr := raw.(string); isStr {
			return s, true
		}
	}
	return "", false
}

// isoDate formats t like JavaScript's Date.prototype.toISOString():
// always UTC, millisecond precision, e.g. 2026-06-11T17:49:20.135Z.
func isoDate(t time.Time) string {
	return t.UTC().Format("2006-01-02T15:04:05.000Z07:00")
}

// parseCookieDate parses the date formats Wikidot uses in Set-Cookie headers.
func parseCookieDate(s string) (time.Time, error) {
	layouts := []string{
		time.RFC1123,
		"Mon, 02-Jan-2006 15:04:05 MST",
		"Mon, 02 Jan 2006 15:04:05 MST",
		"Mon, 02-Jan-06 15:04:05 MST",
	}
	var lastErr error
	for _, l := range layouts {
		if t, err := time.Parse(l, s); err == nil {
			return t, nil
		} else {
			lastErr = err
		}
	}
	return time.Time{}, lastErr
}
