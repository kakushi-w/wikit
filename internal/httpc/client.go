// Package httpc is a small HTTP client tailored to Wikidot scraping: a custom
// cookie jar, browser-like headers, a token-bucket rate limiter, a bounded
// connection pool, manual redirect control, and transparent decompression.
package httpc

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sync"
	"time"
)

// HTTPError represents a non-success HTTP response.
type HTTPError struct {
	Status int
	Body   []byte
}

func (e *HTTPError) Error() string {
	return fmt.Sprintf("server returned %d", e.Status)
}

// Options configures a single request.
type Options struct {
	Headers         map[string]string
	Body            []byte // POST body
	FollowRedirects *bool  // nil => follow (the original's default)
}

// Client is a WikiComma-style HTTP client.
type Client struct {
	Jar       *CookieJar
	Ratelimit *Ratelimit

	hc   *http.Client
	sem  chan struct{} // bounds concurrent in-flight requests
	mu   sync.Mutex
	ua   string
	uaIx int
}

// userAgents are rotated when the server starts rejecting us (the original
// regenerates a browser fingerprint in that situation).
var userAgents = []string{
	"Mozilla/5.0 (Windows NT 10.0; Win64; x64; rv:100.0) Gecko/20100101 Firefox/100.0",
	"Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36",
	"Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/17.0 Safari/605.1.15",
	"Mozilla/5.0 (X11; Linux x86_64; rv:121.0) Gecko/20100101 Firefox/121.0",
}

// New builds a client with the given concurrency limit and optional http proxy.
func New(connections int, proxyURL *url.URL) *Client {
	if connections <= 0 {
		connections = 8
	}
	tr := &http.Transport{
		MaxIdleConns:        connections * 2,
		MaxIdleConnsPerHost: connections,
		MaxConnsPerHost:     connections,
		IdleConnTimeout:     30 * time.Second,
		ForceAttemptHTTP2:   true,
	}
	if proxyURL != nil {
		tr.Proxy = http.ProxyURL(proxyURL)
	}
	c := &Client{
		Jar: NewCookieJar(),
		hc: &http.Client{
			Transport: tr,
			Timeout:   60 * time.Second,
			// Never auto-follow; we follow manually to capture cookies and
			// to honour per-request FollowRedirects.
			CheckRedirect: func(req *http.Request, via []*http.Request) error {
				return http.ErrUseLastResponse
			},
		},
		sem: make(chan struct{}, connections),
		ua:  userAgents[0],
	}
	return c
}

// ChangeFingerprint rotates the User-Agent, used on repeated server errors.
func (c *Client) ChangeFingerprint() {
	c.mu.Lock()
	c.uaIx = (c.uaIx + 1) % len(userAgents)
	c.ua = userAgents[c.uaIx]
	c.mu.Unlock()
}

func (c *Client) Get(rawURL string, opts *Options) ([]byte, error) {
	return c.do("GET", rawURL, opts)
}

func (c *Client) Post(rawURL string, opts *Options) ([]byte, error) {
	return c.do("POST", rawURL, opts)
}

func (c *Client) do(method, rawURL string, opts *Options) ([]byte, error) {
	if opts == nil {
		opts = &Options{}
	}
	follow := true
	if opts.FollowRedirects != nil {
		follow = *opts.FollowRedirects
	}

	u, err := url.Parse(rawURL)
	if err != nil {
		return nil, err
	}

	const maxRedirects = 20
	for redirects := 0; ; redirects++ {
		if c.Ratelimit != nil {
			c.Ratelimit.Wait()
		}
		c.sem <- struct{}{}
		body, status, location, err := c.once(method, u, opts)
		<-c.sem
		if err != nil {
			return nil, err
		}

		if status == 301 || status == 302 {
			if follow && location != "" && redirects < maxRedirects {
				next, perr := resolveLocation(u, location)
				if perr != nil {
					return nil, &HTTPError{Status: status}
				}
				u = next
				// After the first hop a redirect target is fetched with GET.
				method = "GET"
				opts = &Options{Headers: opts.Headers}
				continue
			}
			return nil, &HTTPError{Status: status}
		}

		if status == 200 || status == 206 {
			return body, nil
		}
		return body, &HTTPError{Status: status, Body: body}
	}
}

// once performs a single HTTP exchange (no redirect following) and returns the
// decompressed body, status, and any Location header.
func (c *Client) once(method string, u *url.URL, opts *Options) (body []byte, status int, location string, err error) {
	var bodyReader io.Reader
	if opts.Body != nil {
		bodyReader = bytes.NewReader(opts.Body)
	}
	req, err := http.NewRequest(method, u.String(), bodyReader)
	if err != nil {
		return nil, 0, "", err
	}

	c.mu.Lock()
	ua := c.ua
	c.mu.Unlock()

	req.Header.Set("User-Agent", ua)
	req.Header.Set("Accept", "*/*")
	// Let net/http negotiate and transparently decompress gzip.
	req.Header.Set("Connection", "keep-alive")

	if cookie := c.Jar.Build(u); cookie != "" {
		req.Header.Set("Cookie", cookie)
	}
	for k, v := range opts.Headers {
		req.Header.Set(k, v)
	}

	resp, err := c.hc.Do(req)
	if err != nil {
		return nil, 0, "", err
	}
	defer resp.Body.Close()

	for _, sc := range resp.Header["Set-Cookie"] {
		c.Jar.Put(sc, u.Host)
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, 0, "", err
	}
	return data, resp.StatusCode, resp.Header.Get("Location"), nil
}

// resolveLocation resolves a possibly-relative redirect target, handling the
// protocol-relative ("//host/path") and absolute-path ("/path") forms.
func resolveLocation(base *url.URL, location string) (*url.URL, error) {
	ref, err := url.Parse(location)
	if err != nil {
		return nil, err
	}
	return base.ResolveReference(ref), nil
}
