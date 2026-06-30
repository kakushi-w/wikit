# wikit CLI

[![English](https://img.shields.io/badge/Lang-English-2563eb?style=for-the-badge)](README.md)&nbsp;[![中文](https://img.shields.io/badge/语言-中文-9ca3af?style=for-the-badge)](README.zh-CN.md)

A cross-platform (Windows / Linux / macOS) Wikidot farm-wiki archiving tool, powered by Go. It
produces backups that are **compatible with the WikiComma archive format** — the
same directory layout and byte-for-byte the same file contents. The backup can be directly imported for use with the [ProjectWikit Engine](https://github.com/WikitTeam/ProjectWikit).


## What gets archived

`wikit` captures essentially everything a Wikidot wiki exposes:

### Wiki metadata (`meta/site.json`)
- Domain
- Global site ID
- Slug (lowercase unix name)
- Home page
- Language

### Page metadata (`meta/pages/<name>.json`)
- Global page ID
- Name
- Title
- Rating
- Tags
- Parent page
- Forum discussion thread ID
- Lock status
- Last-updated timestamp
- Full revision list
- Votes and voters (per-user up/down)
- Attached files list

### File metadata & contents
- Global file ID
- Name
- Original URL
- Size (human-readable and in bytes)
- MIME type and reported content type
- Author (numeric ID)
- Upload timestamp
- The **raw file bytes** (`files/<page>/<file_id>`)

### Page revisions
- Global revision ID
- Per-page revision number
- Timestamp
- Author (numeric ID)
- Change flags
- Commentary
- The **full wiki source text** of every revision (compressed in
  `pages/<name>.7z`)

### Forum — categories (`meta/forum/category/<id>.json`)
- Title, description, global ID
- Time of last post
- Total posts and threads
- Last poster (numeric ID)

### Forum — threads (`meta/forum/<cat>/<thread>.json`)
- Global ID, title, description
- Created when / by whom
- Last post time / last poster
- Post count, sticky flag, lock status
- Full nested post tree

### Forum — posts
- Global ID, title, author, timestamp
- Last edit time / last editor
- Every post revision's **HTML content** (compressed in
  `forum/<cat>/<thread>.7z`)
- Nested replies (recursive tree)

### Users (`_users/<bucket>.json`)
- Display name and username slug
- Account creation date
- Account type (e.g. Pro)
- Activity / karma level
- (Users are bucketed by `id >> 13`.)

## Install

**Linux / macOS**
```
curl -fsSL https://raw.githubusercontent.com/kakushi-w/wikit/main/install.sh | sh
```

**Windows (PowerShell)**
```
irm https://raw.githubusercontent.com/kakushi-w/wikit/main/install.ps1 | iex
```

Open a new terminal, then run `wikit`.

Prefer to do it by hand? Download a binary from the
[Releases](https://github.com/kakushi-w/wikit/releases) page and run
`wikit install` once.

## Usage

```
wikit backup all                 # back up every wiki in config.json
wikit backup <name> [name...]    # back up specific wikis, Separate multiple wikis with spaces
                                 # a name not in the config is fetched from
                                 # https://<name>.wikidot.com
```

### Flags (override config.json values)

```
-c, --config <path>      config file (default ./config.json or $WIKICOMMA_CONFIG)
    --base-dir <path>    override base_directory
    --bucket-size <n>    ratelimit bucket size
    --refill-seconds <n> ratelimit refill seconds
    --delay-ms <n>       delay between jobs (ms)
    --max-jobs <n>       maximum simultaneous wikis
    --user-cache <n>     user-info cache freshness (seconds)
    --http-proxy <s>     http proxy: host:port or host:port:user:password
    --socks-proxy <s>    socks proxy: host:port
    --no-update-check    do not check for a newer wikit release
```

## Updating

```
wikit update            # download and install the latest release
wikit update --check    # only report whether a newer version exists
wikit version           # print the installed version
```

After each `backup` run, wikit also does a cached (once-a-day) check and prints a
one-line notice if a newer version is available — disable with
`--no-update-check` or `WIKIT_NO_UPDATE_CHECK=1`.

## Config

The same `config.json` format as WikiComma:

```json
{
  "base_directory": "/data",
  "wikis": [ { "name": "scp-wiki", "url": "https://scp-wiki.wikidot.com" } ],
  "ratelimit": { "bucket_size": 60, "refill_seconds": 60 },
  "delay_ms": 200,
  "user_list_cache_freshness": 86400,
  "http_proxy": null,
  "socks_proxy": null
}
```

A config file is only required for `backup all`, which reads the wiki list from
it. When you name wikis explicitly (`wikit backup <name> ...`) and there is no
`config.json`, wikit runs with these same defaults built in — archiving into a
`wikit_data` folder created in the current working directory (where you launched
the command). Use `--base-dir` (and the other override flags) to adjust them
without writing a config, or pass `-c <path>` to point at one. Naming a config
with `-c` that does not exist is an error.

## Output layout

```
<base_directory>/
  _users/<bucket>.json            users bucketed by id >> 13
  _users/pending.json
  <wiki>/
    http_cookies.json
    meta/site.json
    meta/sitemap.json
    meta/pages/<name>.json        page metadata (":" -> "_")
    meta/file_map.json  meta/page_id_map.json  meta/pending_*.json
    meta/forum/category/<id>.json
    meta/forum/<cat>/<thread>.json
    pages/<name>.7z               compressed page revisions (<rev>.txt)
    files/<page>/<file_id>        raw attachments
    forum/<cat>/<thread>.7z       compressed post html (<post>/<rev|latest>.html)
```

## Building

```
go build -o wikit ./cmd/wikit
```

Cross-compile (the matching 7-Zip binary is embedded per platform):

```
GOOS=windows GOARCH=amd64 go build -o wikit.exe ./cmd/wikit
GOOS=linux   GOARCH=amd64 go build -o wikit     ./cmd/wikit
```


## Tests

```
go test ./...
```
