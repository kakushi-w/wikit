# wikit

[![English](https://img.shields.io/badge/Lang-English-9ca3af?style=for-the-badge)](README.md)&nbsp;[![中文](https://img.shields.io/badge/语言-中文-2563eb?style=for-the-badge)](README.zh-CN.md)

由Go语言驱动的跨平台（Windows / Linux / macOS）的 Wikidot 维基备份工具。它产出的备份**兼容
WikiComma 归档格式**：相同的目录结构、逐字节相同的文件内容。因此已有的
WikiComma 备份和它的 `config.json` 可以原样沿用，`wikit` 会在其基础上继续增量备份。备份可直接导入[ProjectWikit维基迁移引擎](https://github.com/WikitTeam/ProjectWikit)使用。


## 能备份哪些数据

`wikit` 几乎抓取 Wikidot 维基对外暴露的全部内容：

### 站点元数据（`meta/site.json`）
- 域名
- 全局站点 ID
- Slug（小写 unix 名）
- 主页
- 语言

### 页面元数据（`meta/pages/<名字>.json`）
- 全局页面 ID
- 页面名
- 标题
- 评分
- 标签
- 父页面
- 关联的论坛讨论串 ID
- 是否锁定
- 最后更新时间
- 完整修订列表
- 投票与投票者（每个用户的赞/踩）
- 附件列表

### 附件元数据与内容
- 全局文件 ID
- 文件名
- 原始 URL
- 大小（可读格式与字节数）
- MIME 类型与服务器报告的内容类型
- 上传者（数字 ID）
- 上传时间
- **附件原始字节**（`files/<页名>/<file_id>`）

### 页面修订
- 全局修订 ID
- 页内修订编号
- 时间戳
- 作者（数字 ID）
- 变更标志（flags）
- 修订注释
- 每个修订的**完整维基源码文本**（压缩在 `pages/<名字>.7z` 中）

### 论坛 — 分类（`meta/forum/category/<id>.json`）
- 标题、描述、全局 ID
- 最后发帖时间
- 帖子总数、讨论串总数
- 最后发帖人（数字 ID）

### 论坛 — 讨论串（`meta/forum/<分类>/<串>.json`）
- 全局 ID、标题、描述
- 创建时间 / 创建者
- 最后发帖时间 / 最后发帖人
- 帖子数、是否置顶、是否锁定
- 完整的嵌套帖子树

### 论坛 — 帖子
- 全局 ID、标题、作者、时间戳
- 最后编辑时间 / 最后编辑者
- 每个帖子修订的 **HTML 内容**（压缩在 `forum/<分类>/<串>.7z` 中）
- 嵌套回复（递归树结构）

### 用户（`_users/<bucket>.json`）
- 显示名与用户名 slug
- 注册日期
- 账户类型（如 Pro）
- 活跃度 / karma 等级
- （用户按 `id >> 13` 分桶存储。）

## 安装

**Linux / macOS**
```
curl -fsSL https://raw.githubusercontent.com/kakushi-w/wikit/main/install.sh | sh
```

**Windows（PowerShell）**
```
irm https://raw.githubusercontent.com/kakushi-w/wikit/main/install.ps1 | iex
```

装完新开一个终端，直接运行 `wikit`。

手动安装：前往[Releases](https://github.com/kakushi-w/wikit/releases) 页面下载对应
二进制，运行一次 `wikit install` 即可。

## 用法

```
wikit backup all                 # 备份 config.json 里的所有 wiki
wikit backup <名字> [名字...]     # 备份指定的 wiki
                                 # 不在 config 里的名字会按
                                 # https://<名字>.wikidot.com 抓取
```

### 命令行参数（覆盖 config.json 的值）

```
-c, --config <路径>      配置文件（默认 ./config.json 或 $WIKICOMMA_CONFIG）
    --base-dir <路径>    覆盖 base_directory
    --bucket-size <n>    限速令牌桶容量
    --refill-seconds <n> 限速令牌桶填满秒数
    --delay-ms <n>       任务之间的延迟（毫秒）
    --max-jobs <n>       同时备份的最大 wiki 数
    --user-cache <n>     用户信息缓存有效期（秒）
    --http-proxy <s>     http 代理：host:port 或 host:port:user:password
    --socks-proxy <s>    socks 代理：host:port
    --no-update-check    不检查是否有新版本
```

## 更新

```
wikit update            # 下载并安装最新版
wikit update --check    # 只检查有没有新版，不安装
wikit version           # 显示当前版本
```

每次 `backup` 跑完后，wikit会做一次版本检查，有新版就打印一行提示。可用 `--no-update-check` 或
`WIKIT_NO_UPDATE_CHECK=1` 关闭。

## 配置

与 WikiComma 相同的 `config.json` 格式：

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

只有 `backup all` 必须要有配置文件（它从中读取 wiki 列表）。当你显式指定 wiki 名字
（`wikit backup <名字> ...`）且本地没有 `config.json` 时，wikit 会用上面这套相同的
默认值内置运行，在你**运行命令的当前目录**下创建一个 `wikit_data` 文件夹来存放备份，其余参数同上。可以用 `--base-dir`（以及其他覆盖参数）
在不写配置文件的情况下调整，或用 `-c <路径>` 指向一个配置文件。用 `-c` 指定一个不存在
的配置文件会报错。

## 输出目录结构

```
<base_directory>/
  _users/<bucket>.json            按 id >> 13 分桶的用户
  _users/pending.json
  <wiki>/
    http_cookies.json
    meta/site.json
    meta/sitemap.json
    meta/pages/<名字>.json         页面元数据（":" -> "_"）
    meta/file_map.json  meta/page_id_map.json  meta/pending_*.json
    meta/forum/category/<id>.json
    meta/forum/<分类>/<串>.json
    pages/<名字>.7z                压缩后的页面修订（<rev>.txt）
    files/<页名>/<file_id>         附件原始文件
    forum/<分类>/<串>.7z           压缩后的帖子 HTML（<post>/<rev|latest>.html）
```

## 构建

```
go build -o wikit ./cmd/wikit
```

交叉编译（对应平台的 7-Zip 二进制会按平台内嵌）：

```
GOOS=windows GOARCH=amd64 go build -o wikit.exe ./cmd/wikit
GOOS=linux   GOARCH=amd64 go build -o wikit     ./cmd/wikit
```


## 测试

```
go test ./...
```
