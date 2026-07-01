// Command wikit is a cross-platform Wikidot archiving tool whose backup output is
// compatible with the WikiComma archive format (same layout, byte-identical
// files), so existing WikiComma backups can be continued interchangeably.
//
// Usage:
//
//	wikit backup all                 back up every wiki listed in config.json
//	wikit backup <name> [name...]    back up specific wikis (a name not in the
//	                                 config is fetched from https://<name>.wikidot.com)
//
// Override flags (defaults come from config.json):
//
//	-c, --config        path to config.json (default ./config.json or $WIKICOMMA_CONFIG)
//	    --base-dir      override base_directory
//	    --bucket-size   ratelimit bucket size
//	    --refill-seconds ratelimit refill seconds
//	    --delay-ms      delay between jobs
//	    --max-jobs      maximum simultaneous jobs
//	    --user-cache    user info cache freshness (seconds)
//	    --http-proxy    http proxy as host:port or host:port:user:password
//	    --socks-proxy   socks proxy as host:port
//	    --refresh-votes after backup, bulk-refresh page ratings/votes via ListPages
//	    --scheme        wiki URL scheme: http or https (default https)
//	    --keep-removed  keep pages that disappeared from the sitemap
package main

import (
	"fmt"
	"os"
	"strings"

	"wikit/internal/backup"
	"wikit/internal/config"
	"wikit/internal/installer"
	"wikit/internal/selfupdate"
)

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}

	// Clean up a leftover binary from a previous self-update (Windows can't
	// delete the running exe, so it is removed on the next run).
	selfupdate.CleanupOld()

	switch os.Args[1] {
	case "backup":
		if err := runBackup(os.Args[2:]); err != nil {
			fmt.Fprintf(os.Stderr, "wikit: %v\n", err)
			os.Exit(1)
		}
	case "update", "upgrade":
		if err := runUpdate(os.Args[2:]); err != nil {
			fmt.Fprintf(os.Stderr, "wikit: %v\n", err)
			os.Exit(1)
		}
	case "install":
		if err := installer.Install(); err != nil {
			fmt.Fprintf(os.Stderr, "wikit: %v\n", err)
			os.Exit(1)
		}
	case "uninstall":
		if err := installer.Uninstall(); err != nil {
			fmt.Fprintf(os.Stderr, "wikit: %v\n", err)
			os.Exit(1)
		}
	case "-h", "--help", "help":
		usage()
	case "version", "--version":
		fmt.Println("wikit " + version)
	default:
		fmt.Fprintf(os.Stderr, "wikit: unknown command %q\n", os.Args[1])
		usage()
		os.Exit(2)
	}
}

// version is the release version, overridable at build time with
// -ldflags "-X main.version=v1.2.3". updateRepo is the GitHub "owner/repo" used
// for self-update, overridable with -ldflags "-X main.updateRepo=owner/repo" or
// the WIKIT_UPDATE_REPO environment variable.
var (
	version    = "0.1.0"
	updateRepo = "kakushi-w/wikit"
)

// repo resolves the update repository, preferring the env override.
func repo() string {
	if r := os.Getenv("WIKIT_UPDATE_REPO"); r != "" {
		return r
	}
	return updateRepo
}

func runUpdate(args []string) error {
	checkOnly := false
	for _, a := range args {
		if a == "--check" {
			checkOnly = true
		}
	}
	return selfupdate.Run(repo(), version, checkOnly)
}

func runBackup(args []string) error {
	opts, targets, err := parseBackupArgs(args)
	if err != nil {
		return err
	}
	if len(targets) == 0 {
		return fmt.Errorf("nothing to back up: specify 'all' or one or more wiki names")
	}

	cfg, err := loadConfigOrDefault(opts, targets)
	if err != nil {
		return err
	}
	opts.apply(cfg)

	wikis, err := resolveTargets(cfg, targets)
	if err != nil {
		return err
	}
	if err := backup.Run(cfg, wikis); err != nil {
		return err
	}

	// Non-intrusive, cached check for a newer wikit release.
	if !opts.noUpdateCheck && os.Getenv("WIKIT_NO_UPDATE_CHECK") == "" {
		selfupdate.NotifyIfNewer(repo(), version)
	}
	return nil
}

// loadConfigOrDefault loads the config file, falling back to the built-in
// defaults (config.Default) when the file is simply absent. The fallback only
// applies when the user named explicit wikis and did not point at a specific
// config with -c/--config: "backup all" needs the config's wiki list, and an
// explicitly-requested config that is missing is treated as an error.
func loadConfigOrDefault(opts backupOpts, targets []string) (*config.Config, error) {
	cfg, err := config.Load(opts.configPath)
	if err == nil {
		return cfg, nil
	}
	if !os.IsNotExist(err) {
		return nil, err // e.g. invalid JSON — surface it
	}
	if opts.configSet {
		return nil, err // user named a config that does not exist
	}
	if wantsAll(targets) {
		return nil, fmt.Errorf("no config file at %s: 'backup all' needs a config listing the wikis", opts.configPath)
	}
	return config.Default(), nil
}

// wantsAll reports whether the targets contain the special "all" keyword.
func wantsAll(targets []string) bool {
	for _, t := range targets {
		if t == "all" {
			return true
		}
	}
	return false
}

// resolveTargets turns the positional arguments into the concrete list of wikis
// to back up. "all" expands to config.wikis; any other name is looked up in the
// config and, if absent, synthesized from the default Wikidot URL.
func resolveTargets(cfg *config.Config, targets []string) ([]config.WikiEntry, error) {
	scheme := "https"
	if cfg.Scheme == "http" {
		scheme = "http"
	}

	for _, t := range targets {
		if t == "all" {
			if len(targets) != 1 {
				return nil, fmt.Errorf("'all' cannot be combined with explicit wiki names")
			}
			if len(cfg.Wikis) == 0 {
				return nil, fmt.Errorf("config lists no wikis")
			}
			out := make([]config.WikiEntry, len(cfg.Wikis))
			for i, w := range cfg.Wikis {
				out[i] = config.WikiEntry{Name: w.Name, URL: withScheme(w.URL, scheme)}
			}
			return out, nil
		}
	}

	var out []config.WikiEntry
	for _, t := range targets {
		if w, ok := cfg.FindWiki(t); ok {
			out = append(out, config.WikiEntry{Name: w.Name, URL: withScheme(w.URL, scheme)})
		} else {
			out = append(out, config.WikiEntry{
				Name: t,
				URL:  scheme + "://" + t + ".wikidot.com",
			})
		}
	}
	return out, nil
}

// withScheme forces rawURL to use the given scheme, so a single --scheme/config
// setting governs the protocol for every target wiki (synthesized or from
// config). A URL without a scheme gets one prepended.
func withScheme(rawURL, scheme string) string {
	rest := rawURL
	if i := strings.Index(rest, "://"); i != -1 {
		rest = rest[i+3:]
	}
	return scheme + "://" + rest
}

func usage() {
	fmt.Fprint(os.Stderr, `wikit - cross-platform Wikidot archiver

Usage:
  wikit backup all                  back up every wiki in config.json
  wikit backup <name> [name...]     back up specific wikis
  wikit install                     install wikit to your PATH (per-user)
  wikit uninstall                   remove the installed wikit
  wikit update                      upgrade to the latest release
  wikit version                     print the version

Flags:
  -c, --config <path>     config file (default ./config.json or $WIKICOMMA_CONFIG)
      --base-dir <path>   override base_directory
      --bucket-size <n>   ratelimit bucket size
      --refill-seconds <n> ratelimit refill seconds
      --delay-ms <n>      delay between jobs in milliseconds
      --max-jobs <n>      maximum simultaneous jobs
      --user-cache <n>    user info cache freshness in seconds
      --http-proxy <s>    http proxy: host:port or host:port:user:password
      --socks-proxy <s>   socks proxy: host:port
      --refresh-votes     after backup, bulk-refresh page ratings/votes via ListPages
      --scheme <s>        wiki URL scheme: http or https (default https)
      --keep-removed      keep pages that disappeared from the sitemap
`)
}
