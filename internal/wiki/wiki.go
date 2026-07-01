// Package wiki implements the per-wiki archiving work loop: it fetches the
// sitemap, page metadata, revisions, files, forums and users, and writes them to
// disk in WikiComma's exact layout and byte format.
package wiki

import (
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/PuerkitoBio/goquery"

	"wikit/internal/htmlx"
	"wikit/internal/httpc"
	"wikit/internal/userlist"
)

const (
	pageMetadataVersion          = 18
	fileMetadataVersion          = 1
	forumThreadMetadataVersion   = 1
	forumCategoryMetadataVersion = 1
	defaultPagination            = 100
)

// WikiDot archives a single wiki into workDir.
type WikiDot struct {
	name    string
	url     string
	workDir string

	client    *httpc.Client
	users     *userlist.List
	state     *state
	ajaxURL   *url.URL
	delayMs      int
	ultraFast    bool
	refreshVotes bool
	keepRemoved  bool

	tokenFetched bool

	// tokenMu serializes wikidot_token7 refreshes. The work loop runs many
	// goroutines against one shared cookie jar; without this, a single
	// wrong_token7 makes every goroutine invalidate the jar at once, each
	// wiping the token another just fetched — a livelock that never recovers.
	tokenMu sync.Mutex
}

func New(name, wikiURL, workDir string, client *httpc.Client, users *userlist.List, delayMs int, ultraFast, refreshVotes, keepRemoved bool) *WikiDot {
	au, _ := url.Parse(wikiURL + "/ajax-module-connector.php")
	w := &WikiDot{
		name:         name,
		url:          wikiURL,
		workDir:      workDir,
		client:       client,
		users:        users,
		state:        newState(workDir + "/meta"),
		ajaxURL:      au,
		delayMs:      delayMs,
		ultraFast:    ultraFast,
		refreshVotes: refreshVotes,
		keepRemoved:  keepRemoved,
	}
	return w
}

// logf/errf emit a per-wiki log line as "HH:MM:SS LEVEL  <wiki>  <message>".
// INFO goes to stdout, WARN to stderr.
func (w *WikiDot) logf(format string, a ...any) {
	emitLog(os.Stdout, "INFO", w.name, fmt.Sprintf(format, a...))
}

func (w *WikiDot) errf(format string, a ...any) {
	emitLog(os.Stderr, "WARN", w.name, fmt.Sprintf(format, a...))
}

// emitLog writes one timestamped, level-tagged log line. Kept package-level so
// other phases (e.g. the per-wiki failure notice) can share the exact format.
func emitLog(out *os.File, level, wiki, msg string) {
	fmt.Fprintf(out, "%s %-5s %s  %s\n", time.Now().Format("15:04:05"), level, wiki, msg)
}

// LogError emits an ERROR-level line in the shared per-wiki log format, for
// callers outside this package (such as the backup orchestrator).
func LogError(wiki, msg string) {
	emitLog(os.Stderr, "ERROR", wiki, msg)
}

func (w *WikiDot) delay() {
	if w.delayMs > 0 {
		time.Sleep(time.Duration(w.delayMs) * time.Millisecond)
	}
}

// initialize loads persisted state and cookies.
func (w *WikiDot) initialize() {
	w.state.load()
	w.loadCookies()
}

func (w *WikiDot) cookiePath() string { return w.workDir + "/http_cookies.json" }

func (w *WikiDot) loadCookies() {
	data, err := os.ReadFile(w.cookiePath())
	if err != nil {
		return
	}
	_ = w.client.Jar.Load(data)
}

func (w *WikiDot) saveCookies() {
	data, err := w.client.Jar.Save()
	if err != nil {
		return
	}
	_ = os.MkdirAll(w.workDir, 0o755)
	_ = os.WriteFile(w.cookiePath(), data, 0o644)
}

// FetchToken ensures we hold a wikidot_token7 cookie.
func (w *WikiDot) FetchToken(force bool) error {
	if !force && w.client.Jar.GetSpecific(w.ajaxURL, "wikidot_token7") != nil {
		return nil
	}
	noFollow := false
	_, err := w.client.Get(w.url+"/system:recent-changes", &httpc.Options{FollowRedirects: &noFollow})
	if err != nil {
		return err
	}
	w.saveCookies()
	return nil
}

// ajaxBody posts an ajax module request and returns the raw response bytes.
func (w *WikiDot) ajaxBody(params map[string]string, headers map[string]string) ([]byte, error) {
	tok := w.client.Jar.GetSpecific(w.ajaxURL, "wikidot_token7")
	if tok == nil {
		if err := w.FetchToken(false); err != nil {
			return nil, err
		}
		tok = w.client.Jar.GetSpecific(w.ajaxURL, "wikidot_token7")
	}
	form := url.Values{}
	for k, v := range params {
		form.Set(k, v)
	}
	if tok != nil {
		form.Set("wikidot_token7", tok.Value)
	}
	h := map[string]string{
		"Content-Type": "application/x-www-form-urlencoded",
		"Referer":      w.url,
	}
	for k, v := range headers {
		h[k] = v
	}
	noFollow := false
	return w.client.Post(w.ajaxURL.String(), &httpc.Options{
		Body:            []byte(form.Encode()),
		Headers:         h,
		FollowRedirects: &noFollow,
	})
}

type ajaxEnvelope struct {
	Status  string `json:"status"`
	Message string `json:"message"`
	Body    string `json:"body"`
	Content string `json:"content"`
	Title   string `json:"title"`
}

// maxTokenRetries bounds how many times a single ajax call will refresh the
// token on wrong_token7 before giving up, so a wiki that persistently rejects
// our token fails loudly instead of looping forever.
const maxTokenRetries = 4

// ajaxJSON posts an ajax request and parses the JSON envelope, refreshing the
// token a bounded number of times on wrong_token7. Refreshes are serialized and
// deduplicated via refreshToken so concurrent workers don't invalidate each
// other's freshly-fetched token (which otherwise livelocks the whole run).
func (w *WikiDot) ajaxJSON(params, headers map[string]string, custom bool) (*ajaxEnvelope, error) {
	for attempt := 0; ; attempt++ {
		// Record the token we are about to use so refreshToken can tell whether
		// another goroutine has already replaced it by the time we get rejected.
		usedToken := ""
		if tok := w.client.Jar.GetSpecific(w.ajaxURL, "wikidot_token7"); tok != nil {
			usedToken = tok.Value
		}

		raw, err := w.ajaxBody(params, headers)
		if err != nil {
			return nil, err
		}
		var env ajaxEnvelope
		if err := json.Unmarshal(raw, &env); err != nil {
			return nil, fmt.Errorf("invalid ajax json: %w", err)
		}

		if env.Status == "wrong_token7" {
			if attempt >= maxTokenRetries {
				return nil, fmt.Errorf("wikidot kept rejecting our token (wrong_token7) after %d attempts", maxTokenRetries)
			}
			if err := w.refreshToken(usedToken); err != nil {
				return nil, err
			}
			continue
		}

		if env.Status != "ok" && !custom {
			return nil, fmt.Errorf("server returned %s, message: %s", env.Status, env.Message)
		}
		return &env, nil
	}
}

// refreshToken obtains a fresh wikidot_token7, but only once per "generation":
// if another goroutine has already replaced the token we used, it returns
// immediately so the caller simply retries with the new token. Otherwise it
// performs the one back-off + invalidate + fetch, holding tokenMu so concurrent
// callers can't clobber each other.
func (w *WikiDot) refreshToken(usedToken string) error {
	w.tokenMu.Lock()
	defer w.tokenMu.Unlock()

	cur := ""
	if tok := w.client.Jar.GetSpecific(w.ajaxURL, "wikidot_token7"); tok != nil {
		cur = tok.Value
	}
	// Someone else already refreshed since we sent our request — reuse theirs.
	if cur != "" && cur != usedToken {
		return nil
	}

	w.errf("token rejected by wikidot - refreshing in 10s")
	time.Sleep(10 * time.Second)
	w.client.Jar.Invalidate()
	if err := w.FetchToken(true); err != nil {
		return err
	}
	w.logf("token refreshed, resuming")
	return nil
}

// get performs a plain GET with the wiki referer, following redirects (the
// original's default for sitemap, forum and file downloads).
func (w *WikiDot) get(rawURL string, headers map[string]string) ([]byte, error) {
	h := map[string]string{"Referer": w.url}
	for k, v := range headers {
		h[k] = v
	}
	return w.client.Get(rawURL, &httpc.Options{Headers: h})
}

// getNoRedirect performs a GET that does not follow redirects (used for the
// generic page fetch, which relies on /noredirect/true).
func (w *WikiDot) getNoRedirect(rawURL string, headers map[string]string) ([]byte, error) {
	h := map[string]string{"Referer": w.url}
	for k, v := range headers {
		h[k] = v
	}
	noFollow := false
	return w.client.Get(rawURL, &httpc.Options{Headers: h, FollowRedirects: &noFollow})
}

// matchAndFetchUser extracts a user id (and triggers a background user fetch)
// from a printuser element, returning the id as a NullInt (null when absent).
func (w *WikiDot) matchAndFetchUser(sel *goquery.Selection) NullInt {
	id, username := htmlx.ExtractUser(sel)
	if id == nil {
		return nullUser()
	}
	if username != "" && w.users != nil {
		w.users.FetchOptionalAsync(*id, username)
	}
	return num(*id)
}

// normalizeName replaces ':' with '_' for on-disk page directory/file names.
func normalizeName(name string) string {
	return strings.ReplaceAll(name, ":", "_")
}
