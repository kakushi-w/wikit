package userlist

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/PuerkitoBio/goquery"

	"wikit/internal/httpc"
	"wikit/internal/jsonx"
)

// List is the shared, cross-wiki user database. Users are stored in buckets
// (id >> 13) under workFolder, matching the original WikiDotUserList.
type List struct {
	workFolder string
	client     *httpc.Client
	cacheValid int64 // milliseconds

	mu       sync.Mutex
	loaded   bool
	byID     map[int64]User
	byName   map[string]User
	fetched  map[string]bool
	failures map[string]bool
	pending  []pendingEntry

	wg  sync.WaitGroup
	sem chan struct{}
}

type pendingEntry struct {
	hasID bool
	id    int64
	name  string
}

func New(workFolder string, client *httpc.Client, cacheValidMillis int64) *List {
	if cacheValidMillis <= 0 {
		cacheValidMillis = 86400000
	}
	return &List{
		workFolder: workFolder,
		client:     client,
		cacheValid: cacheValidMillis,
		byID:       map[int64]User{},
		byName:     map[string]User{},
		fetched:    map[string]bool{},
		failures:   map[string]bool{},
		sem:        make(chan struct{}, 8),
	}
}

// Initialize loads existing buckets and the pending queue into memory.
func (l *List) Initialize() error {
	if err := os.MkdirAll(l.workFolder, 0o755); err != nil {
		return err
	}
	l.loadMapping()
	l.loadPending()
	return nil
}

func (l *List) loadMapping() {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.loaded {
		return
	}
	entries, err := os.ReadDir(l.workFolder)
	if err == nil {
		for _, e := range entries {
			name := e.Name()
			if e.IsDir() || !strings.HasSuffix(name, ".json") || name == "pending.json" {
				continue
			}
			if !isAllDigits(strings.TrimSuffix(name, ".json")) {
				continue
			}
			data, err := os.ReadFile(filepath.Join(l.workFolder, name))
			if err != nil {
				continue
			}
			v, err := jsonx.Decode(data)
			if err != nil {
				continue
			}
			o, ok := v.(*jsonx.Object)
			if !ok {
				continue
			}
			for _, k := range o.Keys() {
				uv, _ := o.Get(k)
				uo, ok := uv.(*jsonx.Object)
				if !ok {
					continue
				}
				u := readUser(uo)
				l.byID[u.UserID] = u
				l.byName[u.Username] = u
			}
		}
	}
	l.loaded = true
}

func (l *List) loadPending() {
	data, err := os.ReadFile(filepath.Join(l.workFolder, "pending.json"))
	if err != nil {
		return
	}
	v, err := jsonx.Decode(data)
	if err != nil {
		return
	}
	arr, ok := v.([]any)
	if !ok {
		return
	}
	for _, e := range arr {
		pair, ok := e.([]any)
		if !ok || len(pair) != 2 {
			continue
		}
		pe := pendingEntry{}
		if pair[0] != nil {
			pe.hasID = true
			pe.id = toInt(pair[0])
		}
		if s, ok := pair[1].(string); ok {
			pe.name = s
		}
		l.pending = append(l.pending, pe)
	}
	// Re-attempt previously pending users.
	for _, pe := range l.pending {
		id := int64(0)
		if pe.hasID {
			id = pe.id
		}
		l.FetchOptionalAsync(id, pe.name)
	}
}

// FetchOptionalAsync fetches a user in the background if not already known/fresh.
func (l *List) FetchOptionalAsync(id int64, username string) {
	l.wg.Add(1)
	go func() {
		defer l.wg.Done()
		l.sem <- struct{}{}
		defer func() { <-l.sem }()
		if err := l.fetchOptional(id, username); err != nil {
			// errors are non-fatal; logged sparsely
		}
	}()
}

// Wait blocks until all in-flight user fetches complete, then flushes pending.
func (l *List) Wait() {
	l.wg.Wait()
	l.writePending()
}

func (l *List) fetchOptional(id int64, username string) error {
	l.mu.Lock()
	if l.fetched[username] || l.failures[username] {
		l.mu.Unlock()
		return nil
	}
	if existing, ok := l.byName[username]; ok {
		if existing.FetchedAt+l.cacheValid >= nowMillis() {
			l.fetched[username] = true
			l.removePending(id, username)
			l.mu.Unlock()
			return nil
		}
	}
	l.mu.Unlock()

	path := "https://www.wikidot.com/user:info/" + username
	body, err := l.client.Get(path, &httpc.Options{Headers: map[string]string{
		"User-Agent": "Mozilla/5.0 (Windows NT 10.0; Win64; x64; rv:100.0) Gecko/20100101 Firefox/100.0",
	}})
	if err != nil {
		l.mu.Lock()
		l.failures[username] = true
		l.mu.Unlock()
		return err
	}
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(string(body)))
	if err != nil {
		return err
	}
	u, notFound, err := parseUserInfo(doc, username)
	if err != nil {
		l.mu.Lock()
		l.failures[username] = true
		if notFound {
			l.removePending(id, username)
		}
		l.mu.Unlock()
		return err
	}

	storeID := u.UserID
	if id != 0 {
		storeID = id
	}
	if err := l.write(storeID, u); err != nil {
		return err
	}
	l.mu.Lock()
	l.fetched[username] = true
	l.removePending(id, username)
	l.mu.Unlock()
	return nil
}

// write persists a user into its bucket file (read-modify-write) and updates the
// in-memory maps.
func (l *List) write(id int64, u User) error {
	l.mu.Lock()
	defer l.mu.Unlock()

	bucket := bucketOf(id)
	bucketPath := filepath.Join(l.workFolder, strconv.FormatInt(bucket, 10)+".json")

	o := jsonx.NewObject()
	if data, err := os.ReadFile(bucketPath); err == nil {
		if v, err := jsonx.Decode(data); err == nil {
			if existing, ok := v.(*jsonx.Object); ok {
				o = existing
			}
		}
	}
	o.Set(strconv.FormatInt(id, 10), u.object())

	data, err := jsonx.Marshal(o)
	if err != nil {
		return err
	}
	if err := os.WriteFile(bucketPath, data, 0o644); err != nil {
		return err
	}
	l.byID[id] = u
	l.byName[u.Username] = u
	return nil
}

// removePending drops a (id,name) entry from the pending queue. Caller holds mu.
func (l *List) removePending(id int64, username string) {
	for i, pe := range l.pending {
		if (id != 0 && pe.hasID && pe.id == id) || (!pe.hasID && pe.name == username) || pe.name == username {
			l.pending = append(l.pending[:i], l.pending[i+1:]...)
			return
		}
	}
}

func (l *List) writePending() {
	l.mu.Lock()
	defer l.mu.Unlock()
	arr := make([]any, 0, len(l.pending))
	for _, pe := range l.pending {
		pair := make([]any, 0, 2)
		if pe.hasID {
			pair = append(pair, pe.id)
		} else {
			pair = append(pair, nil)
		}
		pair = append(pair, pe.name)
		arr = append(arr, pair)
	}
	data, err := jsonx.Marshal(arr)
	if err != nil {
		return
	}
	_ = os.WriteFile(filepath.Join(l.workFolder, "pending.json"), data, 0o644)
}

func nowMillis() int64 { return time.Now().UnixMilli() }

func isAllDigits(s string) bool {
	if s == "" {
		return false
	}
	for i := 0; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return false
		}
	}
	return true
}

var _ = fmt.Sprintf
