// Package backup orchestrates archiving of one or more wikis, mirroring the
// top-level flow of the original index.ts: build a shared user list, then run
// each wiki's work loop with bounded parallelism.
package backup

import (
	"fmt"
	"net/url"
	"sync"

	"wikit/internal/config"
	"wikit/internal/httpc"
	"wikit/internal/userlist"
	"wikit/internal/wiki"
)

// Run archives the given wikis using settings from cfg.
func Run(cfg *config.Config, wikis []config.WikiEntry) error {
	if cfg.BaseDirectory == "" {
		return fmt.Errorf("base_directory is not set")
	}

	delayMs := 0
	if cfg.DelayMs != nil {
		delayMs = *cfg.DelayMs
	}
	var proxyURL *url.URL
	if cfg.HTTPProxy != nil {
		proxyURL = &url.URL{Scheme: "http", Host: fmt.Sprintf("%s:%d", cfg.HTTPProxy.Address, cfg.HTTPProxy.Port)}
		if cfg.HTTPProxy.User != "" {
			proxyURL.User = url.UserPassword(cfg.HTTPProxy.User, cfg.HTTPProxy.Password)
		}
	}

	newClient := func() *httpc.Client {
		c := httpc.New(8, proxyURL)
		if cfg.Ratelimit != nil {
			c.Ratelimit = httpc.NewRatelimit(cfg.Ratelimit.BucketSize, cfg.Ratelimit.RefillSeconds)
		}
		return c
	}

	// Shared user list (one bucket directory under base_directory/_users).
	users := userlist.New(cfg.BaseDirectory+"/_users", newClient(), cfg.UserCacheMillis())
	if err := users.Initialize(); err != nil {
		return fmt.Errorf("initialize user list: %w", err)
	}

	var sitemapLock sync.Mutex
	maxParallel := 3
	if cfg.MaximumJobs != nil && *cfg.MaximumJobs > 0 && *cfg.MaximumJobs < maxParallel {
		maxParallel = *cfg.MaximumJobs
	}

	sem := make(chan struct{}, maxParallel)
	var wg sync.WaitGroup
	for _, entry := range wikis {
		wg.Add(1)
		go func(entry config.WikiEntry) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			client := newClient()
			defer client.Ratelimit.Stop()

			w := wiki.New(entry.Name, entry.URL, cfg.BaseDirectory+"/"+entry.Name, client, users, delayMs, cfg.UltraFastIncremental, cfg.RefreshVotes, cfg.KeepRemoved)
			for i := 0; i < 40; i++ {
				if err := w.FetchToken(false); err != nil {
					if he, ok := err.(*httpc.HTTPError); ok && he.Status == 500 {
						client.ChangeFingerprint()
						continue
					}
				}
				break
			}
			if err := w.WorkLoop(&sitemapLock); err != nil {
				wiki.LogError(entry.Name, fmt.Sprintf("backup failed: %v", err))
			}
		}(entry)
	}
	wg.Wait()

	users.Wait()
	return nil
}
