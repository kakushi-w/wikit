// Package config loads the WikiComma-compatible config.json and applies any
// command-line overrides. The on-disk format is intentionally identical to the
// original tool's so an existing config can be reused unchanged.
package config

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

type WikiEntry struct {
	Name string `json:"name"`
	// URL is optional. Omit it for a standard wiki to derive
	// <scheme>://<name>.wikidot.com; set it for a custom domain or to pin this
	// wiki's protocol (an explicit http://…/https://… is honored as written).
	URL string `json:"url,omitempty"`
}

type Ratelimit struct {
	BucketSize    int `json:"bucket_size"`
	RefillSeconds int `json:"refill_seconds"`
}

type HTTPProxy struct {
	Address  string `json:"address"`
	Port     int    `json:"port"`
	User     string `json:"user,omitempty"`
	Password string `json:"password,omitempty"`
}

type SocksProxy struct {
	Address string `json:"address"`
	Port    int    `json:"port"`
}

// Config mirrors the original IDaemonConfig. Optional sections are pointers so
// "absent" and "present" are distinguishable, matching the original semantics.
type Config struct {
	BaseDirectory string      `json:"base_directory"`
	Wikis         []WikiEntry `json:"wikis"`

	UserListCacheFreshness *int `json:"user_list_cache_freshness,omitempty"`

	Ratelimit              *Ratelimit  `json:"ratelimit,omitempty"`
	DelayMs                *int        `json:"delay_ms,omitempty"`
	MaximumJobs            *int        `json:"maximum_jobs,omitempty"`
	HTTPProxy              *HTTPProxy  `json:"http_proxy,omitempty"`
	SocksProxy             *SocksProxy `json:"socks_proxy,omitempty"`
	UltraFastIncremental   bool        `json:"ultra_fast_incremental_scan,omitempty"`

	// RefreshVotes enables a bulk ListPages rating/vote refresh after the normal
	// backup. Settable in config.json or via --refresh-votes.
	RefreshVotes bool `json:"refresh_votes,omitempty"`

	// Scheme is the default URL scheme ("http" or "https") applied only to wikis
	// whose own url omits a scheme (and to bare command-line names). A wiki whose
	// url already includes http:// or https:// keeps that, so per-wiki protocols
	// are possible. Empty means https. Settable in config.json or via --scheme.
	Scheme string `json:"scheme,omitempty"`

	// KeepRemoved keeps locally-archived pages that have disappeared from the
	// sitemap instead of deleting them. Settable in config.json or via
	// --keep-removed.
	KeepRemoved bool `json:"keep_removed,omitempty"`
}

// Default returns the built-in configuration used when no config.json is
// present. Its values match the documented defaults in the README, so running
// without a config behaves identically to running with the example config. It
// lists no wikis — the caller names the target wiki on the command line.
func Default() *Config {
	delay := 200
	cache := 86400
	return &Config{
		// Relative "wikit_data" so a config-less run archives into a
		// ./wikit_data folder under the current working directory (where the
		// command was launched), rather than the absolute /data the example
		// config uses.
		BaseDirectory:          "wikit_data",
		Ratelimit:              &Ratelimit{BucketSize: 60, RefillSeconds: 60},
		DelayMs:                &delay,
		UserListCacheFreshness: &cache,
	}
}

// Load reads config from path. The original trims a trailing slash from each
// wiki URL because some Wikidot endpoints dislike the resulting double slash.
func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var c Config
	if err := json.Unmarshal(data, &c); err != nil {
		return nil, fmt.Errorf("config %s is invalid: %w", path, err)
	}
	for i := range c.Wikis {
		c.Wikis[i].URL = strings.TrimSuffix(c.Wikis[i].URL, "/")
	}
	return &c, nil
}

// FindWiki returns the configured entry with the given name, if any.
func (c *Config) FindWiki(name string) (WikiEntry, bool) {
	for _, w := range c.Wikis {
		if w.Name == name {
			return w, true
		}
	}
	return WikiEntry{}, false
}

// UserCacheMillis returns the user-info cache lifetime in milliseconds, applying
// the original default of 86400 seconds when unset.
func (c *Config) UserCacheMillis() int64 {
	secs := 86400
	if c.UserListCacheFreshness != nil {
		secs = *c.UserListCacheFreshness
	}
	return int64(secs) * 1000
}
