package main

import (
	"flag"
	"fmt"
	"os"
	"strconv"
	"strings"

	"wikit/internal/config"
)

// backupOpts holds command-line overrides. A nil pointer means "not specified",
// so the config.json value (or its default) is left untouched.
type backupOpts struct {
	configPath string
	configSet  bool // true if the user explicitly passed -c/--config

	baseDir       *string
	bucketSize    *int
	refillSeconds *int
	delayMs       *int
	maxJobs       *int
	userCache     *int
	httpProxy     *string
	socksProxy    *string

	noUpdateCheck bool
}

func parseBackupArgs(args []string) (backupOpts, []string, error) {
	var opts backupOpts

	defaultConfig := os.Getenv("WIKICOMMA_CONFIG")
	if defaultConfig == "" {
		defaultConfig = "config.json"
	}

	fs := flag.NewFlagSet("backup", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)

	fs.StringVar(&opts.configPath, "config", defaultConfig, "config file path")
	fs.StringVar(&opts.configPath, "c", defaultConfig, "config file path (shorthand)")

	baseDir := fs.String("base-dir", "", "override base_directory")
	bucketSize := fs.Int("bucket-size", -1, "ratelimit bucket size")
	refillSeconds := fs.Int("refill-seconds", -1, "ratelimit refill seconds")
	delayMs := fs.Int("delay-ms", -1, "delay between jobs (ms)")
	maxJobs := fs.Int("max-jobs", -1, "maximum simultaneous jobs")
	userCache := fs.Int("user-cache", -1, "user info cache freshness (seconds)")
	httpProxy := fs.String("http-proxy", "", "http proxy host:port[:user:password]")
	socksProxy := fs.String("socks-proxy", "", "socks proxy host:port")
	noUpdateCheck := fs.Bool("no-update-check", false, "do not check for a newer wikit release")

	// Allow flags and positional targets to be interleaved.
	var targets []string
	rest := args
	for len(rest) > 0 {
		if err := fs.Parse(rest); err != nil {
			return opts, nil, err
		}
		rest = fs.Args()
		if len(rest) > 0 {
			targets = append(targets, rest[0])
			rest = rest[1:]
		}
	}

	// Record only the flags the user actually set.
	setFlags := map[string]bool{}
	fs.Visit(func(f *flag.Flag) { setFlags[f.Name] = true })

	if setFlags["base-dir"] {
		opts.baseDir = baseDir
	}
	if setFlags["bucket-size"] {
		opts.bucketSize = bucketSize
	}
	if setFlags["refill-seconds"] {
		opts.refillSeconds = refillSeconds
	}
	if setFlags["delay-ms"] {
		opts.delayMs = delayMs
	}
	if setFlags["max-jobs"] {
		opts.maxJobs = maxJobs
	}
	if setFlags["user-cache"] {
		opts.userCache = userCache
	}
	if setFlags["http-proxy"] {
		opts.httpProxy = httpProxy
	}
	if setFlags["socks-proxy"] {
		opts.socksProxy = socksProxy
	}
	opts.noUpdateCheck = *noUpdateCheck
	opts.configSet = setFlags["config"] || setFlags["c"]

	return opts, targets, nil
}

// apply mutates cfg in place with any overrides the user supplied.
func (o backupOpts) apply(cfg *config.Config) {
	if o.baseDir != nil {
		cfg.BaseDirectory = *o.baseDir
	}
	if o.bucketSize != nil || o.refillSeconds != nil {
		if cfg.Ratelimit == nil {
			cfg.Ratelimit = &config.Ratelimit{BucketSize: 60, RefillSeconds: 60}
		}
		if o.bucketSize != nil {
			cfg.Ratelimit.BucketSize = *o.bucketSize
		}
		if o.refillSeconds != nil {
			cfg.Ratelimit.RefillSeconds = *o.refillSeconds
		}
	}
	if o.delayMs != nil {
		cfg.DelayMs = o.delayMs
	}
	if o.maxJobs != nil {
		cfg.MaximumJobs = o.maxJobs
	}
	if o.userCache != nil {
		cfg.UserListCacheFreshness = o.userCache
	}
	if o.httpProxy != nil {
		cfg.HTTPProxy = parseHTTPProxy(*o.httpProxy)
	}
	if o.socksProxy != nil {
		cfg.SocksProxy = parseSocksProxy(*o.socksProxy)
	}
}

func parseHTTPProxy(s string) *config.HTTPProxy {
	if s == "" {
		return nil
	}
	parts := strings.Split(s, ":")
	if len(parts) < 2 {
		return nil
	}
	port, _ := strconv.Atoi(parts[1])
	p := &config.HTTPProxy{Address: parts[0], Port: port}
	if len(parts) >= 4 {
		p.User = parts[2]
		p.Password = parts[3]
	}
	return p
}

func parseSocksProxy(s string) *config.SocksProxy {
	if s == "" {
		return nil
	}
	parts := strings.Split(s, ":")
	if len(parts) < 2 {
		return nil
	}
	port, _ := strconv.Atoi(parts[1])
	return &config.SocksProxy{Address: parts[0], Port: port}
}

var _ = fmt.Sprintf
