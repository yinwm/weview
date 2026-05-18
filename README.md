# Wxview

Wxview 是一个把本机微信数据变成可访问数据源的工具。

这个项目的目标不是做另一个微信客户端，而是把属于自己的微信数据整理成可以被程序读取、检索和管理的数据源，用来管理自己的数字资产。这里的数字资产包括联系人、群、消息、媒体、文件、链接、关系网络，以及后续可以从这些数据中提取出来的知识、记录和上下文。

V1 当前支持 macOS 微信 4.x 和 Windows 微信 4.x：自动获取支持库的数据库 key，解密联系人、会话、消息和若干可选数据数据库，维护本地缓存，并通过 CLI 输出联系人、群、最近会话、未读/增量消息、按明确 username 查询的聊天记录、正文搜索，以及收藏、公众号 appmsg 和朋友圈 SNS 的本地缓存数据。

## 平台支持

当前支持：

- macOS 微信 4.x
- Windows 微信 4.x（`xwechat_files\<account>\db_storage` 数据布局）

当前不支持：

- Linux
- macOS 微信 3.x

运行依赖：

- 普通 SQLite 读写内置在 `wxview` 二进制中，不要求用户安装系统 `sqlite` 工具。
- `wxview init` 需要读取正在运行的微信进程内存：macOS 通常需要 `sudo`，Windows 需要管理员终端。

## 设计目标

- 本地优先：微信数据、key、解密缓存都保存在本机。
- 面向自动化：CLI 的 JSON、JSONL、CSV 输出方便其他 AI、脚本或系统调用。
- 数据层复用：CLI 只是一个 controller。后续如果增加 Web API，也应该复用同一套 service 和缓存数据。
- 渐进实现：V1 先做联系人和明确 username 的消息读取闭环，再扩展群成员、收藏、公众号 appmsg、朋友圈等高价值本地数据面。
- 明确边界：V1 不做 WAL patch，不做公开 Web API；消息能力覆盖记录读取、常见 XML 摘要和图片/视频/文件/语音本地解析。消息索引是可删除、可重建的派生加速层，不改变源库和解密缓存。

## 当前能力

- 自动检测 macOS 微信账号目录：
  `~/Library/Containers/com.tencent.xinWeChat/Data/Documents/xwechat_files/<account>/db_storage`
- 自动检测 Windows 微信账号目录：
  `%APPDATA%\Tencent\xwechat\config\*.ini` 指向的数据根目录下的
  `xwechat_files\<account>\db_storage`。如果自动检测失败，可以设置
  `WXVIEW_WECHAT_DB_STORAGE` 为明确的 `db_storage` 目录，或设置
  `WXVIEW_WECHAT_DATA_ROOT` 为包含 `xwechat_files` 的数据根目录。
- 自动获取并验证当前支持数据库的 SQLCipher raw key：必需库包括
  `contact/contact.db` 和 `message/message_[number].db`；辅助库
  `message/message_fts.db`、`message/message_resource.db`、
  `message/message_revoke.db`、`message/biz_message_[number].db`、
  `message/media_[number].db`、`session/session.db`、`head_image/head_image.db`、
  `favorite/favorite.db`、`sns/sns.db` 按 best-effort 准备，失败时只输出 warning。
- 解密主数据库 `contact/contact.db` 到本地缓存：
  `~/.wxview/cache/<account>/contact/contact.db`
- 解密消息相关数据库到本地缓存：
  `~/.wxview/cache/<account>/message/`
- 运行本地 daemon，后台维护联系人、会话、消息/媒体、头像、收藏和 SNS 缓存；`~/.wxview/wxview.sock`
  只作为内部控制通道。
- 通过 CLI 查看本地解密缓存的 fresh/stale 状态、源 DB 与缓存 mtime、以及各 DB key 是否可用。
- 通过 CLI 查看和维护派生消息索引 `~/.wxview/cache/<account>/index/messages.db`，用于加速 `messages` 和 `timeline`。
- 通过 CLI 查询联系人、普通私聊联系人、群和其他联系人表记录。
- 通过 CLI 查询单个联系人的更完整详情和本地头像，以及群成员和群主。
- 通过 CLI 查询最近会话、未读会话和基于账号 checkpoint 的增量新消息。
- 通过 CLI 按 username 查询 `message/message_*.db` 的聊天记录，支持时间范围、分页和机器可读输出。
- 通过 CLI 在一个或多个会话内搜索消息正文。
- 通过 CLI 按时间范围查询一批会话，并合并成跨会话时间线。
- 通过 CLI 查询收藏、公众号 / 订阅号文章、视频号 appmsg，以及朋友圈 SNS feed / 搜索 / 通知。
- 支持 `table`、`json`、`jsonl`、`csv` 输出。
- 支持搜索、精确 username 查询、分页、排序和计数。
- `messages` 和 `timeline` 的 `json` 输出使用 `{meta, items}` envelope；
  `meta.schema_version` 标识 envelope 契约版本，`meta.timezone` 标识本地时间解释；
  `jsonl`、`csv`、`table` 仍然只输出记录行。

## 本地构建

```sh
task build
```

构建产物在：

```sh
./bin/wxview
```

Windows 下构建产物通常是：

```powershell
.\bin\wxview.exe
```

常用检查：

```sh
task check   # go test ./... + go vet ./...
task clean   # 删除 bin/
```

## 支持命令

### `wxview init`

首次使用时运行，一般只需要执行一次。

```sh
./bin/wxview init
```

macOS 通常需要用 `sudo ./bin/wxview init` 读取微信进程内存；Windows 需要在管理员终端中运行。

它会检测微信账号目录，枚举必需数据库、消息辅助数据库和可选数据数据库，读取每个数据库 page 1 salt，扫描正在运行的微信进程来获取缺失的数据库 key，验证成功后保存到：

```sh
~/.wxview/cache/<account>/keys.json
```

`keys.json` 绑定账号保存，使用 `version: 1`，内容是该账号的 `account`、`data_dir` 和按 DB 相对路径索引的 key。

`init` 默认只输出账号、路径、key 数量和 warning 摘要。需要逐库查看 fingerprint 和 reused/scanned 状态时，再加 `--verbose`。

如果某个辅助库或可选数据库（例如 `session/session.db`、`message/media_0.db`、`head_image/head_image.db`、`message/message_revoke.db`、`favorite/favorite.db`、`sns/sns.db`）当前没有在微信进程内找到 key，`init` 会输出 warning 并继续保留其他已准备好的 key。联系人和数字消息分片 key 仍然是必需项，缺失时需要带进程内存读取权限重新运行 `wxview init`。

### `wxview daemon`

管理本地联系人缓存维护进程。

```sh
./bin/wxview daemon
```

当前 daemon 只支持这 4 种形式：

```sh
./bin/wxview daemon          # 显示 daemon 帮助
./bin/wxview daemon start    # 后台启动 daemon
./bin/wxview daemon stop     # 停止后台 daemon
./bin/wxview daemon status   # 检查 daemon 是否正在响应 health
```

`wxview daemon` 和 `wxview daemon --help` 行为一样，只显示帮助，不启动进程。

`wxview daemon start` 会后台启动 daemon。如果 daemon 已经在运行，会给出
`status: already running`，不会重复启动。后台 daemon 会先确保 key 可用，把
当前 WeChat 进程正在打开的账号对应的 `contact/contact.db`、`session/session.db`、消息/媒体相关数据库、`head_image/head_image.db`、`favorite/favorite.db` 和 `sns/sns.db` 解密到本地缓存，然后监听主数据库文件变化；发现变化后，会重新刷新缓存。daemon 运行中如果微信切换账号，会重新解析当前 WeChat 打开的 `db_storage` 路径并切到对应账号目录。刷新时会读取
`~/.wxview/cache/<account>/mtime.json`，源 DB 的 size、mtime 和 salt 没变且 cache 已存在时会跳过解密。daemon socket 进入 healthy 后会在后台刷新派生消息索引；消息缓存 watcher 发现变化并成功刷新 `message/message_*.db` 后，也会后台刷新索引。索引失败只写日志，不会让 daemon 退出。daemon 日志写入 `~/.wxview/wxview.log`。

`wxview daemon stop` 会停止后台 daemon；如果 daemon 没有运行，会输出
`status: stopped`。

daemon 当前没有其他 flags；除了 `-h` / `--help` / `help` 外，多余参数都会报错。

V1 不处理 `.db-wal`，所以这是准实时刷新：微信把变化写回主数据库后，缓存才会刷新。

daemon 使用本地 Unix socket：

```sh
~/.wxview/wxview.sock
```

这个 socket 是内部控制通道，不是公开 Web API，也不是联系人查询接口。daemon
只负责维护缓存、响应 `health`、`refresh_contacts`、`refresh_sessions`、`refresh_messages`、`refresh_index`、`refresh_avatars`、`refresh_favorites`、`refresh_sns` 这类内部请求，不负责 `contacts`、`sessions` 或 `messages` 查询。

`wxview contacts ...` 始终直接读取本地解密缓存；daemon 是否运行只影响缓存是否
会被常驻进程自动维护。

### `wxview cache`

查看本地缓存状态。

```sh
./bin/wxview cache status
./bin/wxview cache status --group messages --format json
```

`cache status` 是只读观测命令：不刷新缓存、不解密数据库、不扫描微信进程内存，也不通过 daemon 查询。它读取当前发现的源 DB、`~/.wxview/cache/<account>/mtime.json` 和 `~/.wxview/cache/<account>/keys.json`，判断每个本地缓存是 `fresh`、`stale`、`missing_cache` 还是 `missing_source`，同时给出 `key_status`：

- `available`：`keys.json` 里有匹配当前 source salt 且可验证的 key。
- `missing`：没有匹配这个 DB 当前 salt 的 key。
- `invalid`：保存过 key，但 page 1 HMAC 验证失败。
- `unknown`：key store 或源 DB salt 无法检查。

常用分组：

```sh
./bin/wxview cache status --group contacts
./bin/wxview cache status --group messages
./bin/wxview cache status --group media
./bin/wxview cache status --group sessions
./bin/wxview cache status --group favorites
./bin/wxview cache status --group sns
```

完整参数说明请运行：

```sh
./bin/wxview cache --help
```

### `wxview index`

查看和维护派生消息索引。

```sh
./bin/wxview index status
./bin/wxview index refresh
./bin/wxview index rebuild --reset
./bin/wxview index clean-tmp
./bin/wxview cache clean-tmp
```

索引库位置：

```sh
~/.wxview/cache/<account>/index/messages.db
```

这个库是派生缓存：可以删除，可以重建，不存数据库 key，不存 CDN AES key，也不替代
`wxview cache status`。索引库会保存 row refs 和为后续搜索加速准备的可搜索文本，但当前 `search` 命令仍保持本地扫描语义。`cache status` 观察的是源 DB、解密缓存、mtime metadata 和 key 覆盖；`index status` 观察的是派生索引是否完整、是否正在构建、是否可续跑，以及它和当前 message cache 的滞后关系。

命令语义：

- `index status`：只读检查索引状态，不刷新缓存、不解密数据库、不请求 daemon。
- `index refresh`：普通维护入口。没有索引时从 0 构建；上次中断时从 checkpoint / watermark 续跑；已有完整索引时只追加新增 rows。daemon 正在运行时优先请求 daemon 的内部 `refresh_index`。
- `index rebuild --reset`：明确删除当前索引、job 和 index tmp 后从 0 重建。构建完成前，查询不会使用半成品索引。
- `index clean-tmp`：清理 `index/` 下旧实现遗留的 `.messages-*.db` 等临时文件，不删除正式 `messages.db`。
- `cache clean-tmp`：清理解密 cache 层过期 `*.tmp`，例如 `message_fts.db.*.tmp`；不会处理 `index/messages.db`。

输出字段包括 `status`、`query_mode`、`schema_version`、`source_db_count`、`indexed_rows`、`source_rows`、`covered_chats`、`job_state`、`lag_seconds`、`lag_policy`、`reason` 和 `path`。状态值：

- `missing`：索引库不存在。
- `building`：正在从 0 构建，还没有完整可用基线，查询走原始扫描。
- `interrupted`：上次构建中断，下一次 `index refresh` 可以续跑，查询走原始扫描。
- `ready`：至少完成过一次完整构建，可以作为查询基线。
- `refreshing`：已有完整索引，正在追加新增 rows，已提交部分仍可查询。
- `stale`：索引本身存在，但落后当前 message cache 太久，默认查询走原始扫描。
- `schema_mismatch`：索引 schema version 与当前程序不匹配，`index refresh` 会重建，或可显式执行 `rebuild --reset`。

滞后策略固定为 60 秒：`lag_seconds <= 60` 表示准实时延迟，`messages` 和 `timeline` 可以继续走索引；`lag_seconds > 60` 表示索引追赶太慢或正在重建，默认查询会回落到原来的本地扫描路径。即使整体 `stale`，如果查询时间范围完全落在已索引 watermark 之前，历史查询仍可以走索引。两个命令都支持 `--no-index` 强制走扫描路径，便于排查性能和结果差异。`search` 继续保持本地扫描语义，避免 FTS 分词和包含匹配的结果不一致。

### `wxview contacts`

查询联系人。

```sh
./bin/wxview contacts --format table
./bin/wxview contacts --format json
./bin/wxview contacts --format jsonl
./bin/wxview contacts --format csv
```

`wxview contacts` 不带参数时只显示帮助，不直接查询数据。要查询数据，需要显式传入 `--format`、`--kind`、`--count` 等参数。

`wxview contacts` 始终直接读取本地解密缓存。使用 `--refresh` 时，如果 daemon 正在运行，会先请求 daemon 刷新缓存；如果 daemon 没运行，就在当前进程里刷新一次缓存。

常用查询：

```sh
./bin/wxview contacts --kind friend --format table
./bin/wxview contacts --kind chatroom --format table
./bin/wxview contacts --kind friend --query AI --limit 20 --format json
./bin/wxview contacts --username wxid_xxx --format json
./bin/wxview contacts --detail --username wxid_xxx --format json
./bin/wxview contacts --kind friend --count
./bin/wxview contacts --refresh --format json
```

面向 AI 或其他系统调用时，优先使用：

```sh
./bin/wxview contacts --kind friend --limit 100 --offset 0 --sort username --format json
./bin/wxview contacts --kind chatroom --format jsonl
./bin/wxview contacts --kind friend --format csv
```

完整参数说明请运行：

```sh
./bin/wxview contacts --help
```

#### 输出字段

- `username`：微信内部 username，例如 `wxid_*` 或 `*@chatroom`。
- `alias`：微信号或别名。
- `remark`：你给联系人设置的备注。
- `nick_name`：联系人昵称。
- `head_url`：头像图片 URL。
- `kind`：`friend`、`chatroom` 或 `other`。

使用 `--detail --username USERNAME` 时会返回单个联系人详情对象，额外包含
`small_head_url`、`big_head_url`、`description`、`verify_flag`、`local_type`、
`is_chatroom`、`is_official`、`avatar_status`、`avatar_path` 和 `avatar_reason`。
本地头像来自 `head_image/head_image.db`；没有对应 key 或本地头像时，详情仍会返回远程头像 URL。

`kind` 的含义：

- `friend`：普通私聊联系人。
- `chatroom`：群。
- `other`：公众号、企业联系人、非好友群成员、系统联系人等其他联系人表记录。
- `all`：查询参数，表示不过滤。

### `wxview members`

查询群成员和群主。数据来自本地解密的 `contact/contact.db` 缓存，当前不通过 daemon
做查询服务。

```sh
./bin/wxview members --username 123@chatroom --format json
./bin/wxview members --query "AI交流群" --format table
./bin/wxview members --username 123@chatroom --refresh --format csv
```

选择方式：

- `--username`：精确指定群 username，例如 `123@chatroom`。
- `--query`：按群的 `username`、`alias`、`remark`、`nick_name` 查找；必须只命中一个群，否则需要改用 `--username`。

输出包含群 `username`、`display_name`、`owner`、`owner_display_name`、成员数量和成员列表。成员行包含 `username`、`display_name`、`alias`、`remark`、`nick_name`、`kind` 和 `is_owner`。

### `wxview sessions`

查询最近会话，数据来自本地解密的 `session/session.db` 缓存。

```sh
./bin/wxview sessions --limit 20 --format table
./bin/wxview sessions --kind chatroom --query AI --format json
./bin/wxview sessions --refresh --format jsonl
```

输出字段包含 `username`、`chat_kind`、`chat_display_name`、`unread_count`、`summary`、`last_timestamp`、`time`、`last_msg_type`、`last_msg_sub_type`、`last_sender` 和 `last_sender_display_name`。

### `wxview unread`

查询未读会话，是 `sessions` 的未读过滤入口。

```sh
./bin/wxview unread --format json
./bin/wxview unread --kind chatroom --limit 20 --format table
```

### `wxview new-messages`

查询自上次检查以来的新消息。状态按账号保存在
`~/.wxview/cache/<account>/state/new_messages.json`。

```sh
./bin/wxview new-messages --limit 100 --format json
./bin/wxview new-messages --reset --format table
```

首次运行或 `--reset` 会用最近 24 小时作为起点，避免把历史全量消息一次性涌出来。
输出 item 使用和 `messages` / `timeline` 相同的消息 schema。

### `wxview messages`

按明确 username 查询聊天记录。

```sh
./bin/wxview messages --username wxid_xxx --format table
./bin/wxview messages --username wxid_xxx --date today --limit 100 --format json
./bin/wxview messages --username wxid_xxx --start "2026-05-01" --end "2026-05-14" --format json
./bin/wxview messages --username wxid_xxx --after-seq 1773421286000 --limit 100 --format jsonl
./bin/wxview messages --username 123@chatroom --limit 100 --offset 0 --format jsonl
./bin/wxview messages --username wxid_xxx --source --refresh --format json
./bin/wxview messages --username wxid_xxx --date today --no-index --format json
```

`wxview messages` 不带参数时只显示帮助。查询时必须显式传入 `--username`。
`--username` 始终按普通会话目标处理；即使这个值等于当前登录账号 username，也不会被特殊拦截。

时间范围参数：

- `--date today|yesterday|YYYY-MM-DD`：选择一个本地自然日，不能和
  `--start` / `--end` 同时使用。
- `--start`：包含起点。
- `--end`：包含终点；日期格式如 `2026-05-14` 会包含当天 23:59:59。
- 支持 Unix 秒、`YYYY-MM-DD`、`YYYY-MM-DD HH:MM`、`YYYY-MM-DD HH:MM:SS` 和 RFC3339。

分页参数：

- `--after-seq`：只返回 `seq` 大于该值的消息，用来做“从上一条之后继续读”的游标。
- `--limit`：按全局时间正序合并所有分片后最多返回多少条；`0` 表示不限制。
- `--offset`：按全局时间正序合并所有分片后跳过多少条。
- `--no-index`：强制走旧的本地扫描路径，便于和索引快路径对照。

机器调用建议使用 `--format json`，并在有下一页时直接执行
`meta.next_args`。`--offset` 适合人工临时查看；消息持续变化或做可靠分页时，
优先使用 `next_after_seq` / `next_args`。

输出字段：

- 默认输出聊天历史需要的字段：`id`、`chat_username`、`from_username`、`direction`、`is_self`、`is_chatroom`、`seq`、`server_id`、`create_time`、`time`、`type`、`sub_type`、`content`、`content_detail`、`content_encoding`。
- `messages` 和 `timeline` 的消息行字段一致，并额外包含会话元信息：
  `chat_kind`、`chat_display_name`、`chat_alias`、`chat_remark`、
  `chat_nick_name`。`chat_display_name` 按 `remark > nick_name > alias >
  username` 生成；联系人缓存里找不到时回退为 `chat_username`，
  `chat_kind` 为 `unknown`。
- `chat_username` 是会话用户名，也就是 `--username` 查询对象；`from_username` 是实际发消息的人。群消息会从正文前缀里解析真实发送者。
- `direction` 是 `out`、`in` 或 `unknown`；`is_self` 表示这条消息是否来自本机账号。
- `content` 是 zstd 解码后的原始正文，是保留给程序使用的原始内容；XML 仍然是 XML，群消息前缀也会保留，不会被改成摘要。
- `content_detail` 是解析后的便利字段，用来让人和程序更容易消费常见消息类型；它不能替代 `content`。
- 对普通文本，`content_detail` 通常包含 `type: "text"` 和 `text`。
- 对链接、文件、小程序、视频号等 appmsg XML，`content_detail` 会解析 `type`、`text`、`title`、`desc`、`url`、`source_username`、`source_display_name` 等字段。
- 对图片消息，`content_detail` 会解析 `type: "image"`、`text: "[图片]"`、`md5`、`origin_md5`、`length`、`cdn_thumb_url`、`cdn_thumb_length`、`cdn_thumb_width`、`cdn_thumb_height`、`cdn_mid_img_url`、`cdn_mid_width`、`cdn_mid_height`、`cdn_hd_width`、`cdn_hd_height`、`hevc_mid_size` 等存在且非零的 XML 元数据。
- 对视频消息，`content_detail` 会解析 `type: "video"`、`text: "[视频]"`、`md5`、`new_md5`、`raw_md5`、`origin_source_md5`、`length`、`raw_length`、`play_length`、`cdn_video_url`、`cdn_raw_video_url`、`cdn_thumb_url`、`cdn_thumb_length`、`cdn_thumb_width`、`cdn_thumb_height` 等存在且非零的 XML 元数据；CDN AES key 不会复制到便利字段里。
- 对语音通话消息，`content_detail` 会解析 `type: "voip"`、`text`、`call_text`、`duration_text`、`duration_seconds`、`voip_type`、`msg_type`、`business` 等便利字段；`roomid`、`identity`、`inviteid` 等内部标识不会复制到便利字段里。
- 图片和视频消息会自动解析本地媒体路径；普通图片/视频直接返回本地路径，微信 `.dat` 图片会解密到 `~/.wxview/cache/<account>/media/`，可识别的 `.dat` 视频也会规范化到同一目录。解析结果也放在 `content_detail`：`media_status`、`path`、`source_path`、`decoded`、`thumbnail`、`thumbnail_path`、`thumbnail_source_path`、`thumbnail_decoded`、`width`、`height`、`media_reason`。`text` 只保留语义摘要 `[图片]` 或 `[视频]`；`media_status=resolved` 且 `path` 存在时，`path` 才是可以直接打开的本地媒体路径。
- `--source` 才输出 DB/table/local row 等来源字段，正常看聊天记录不需要。

`--format json` 返回 envelope：

```json
{
  "meta": {
    "schema_version": 1,
    "timezone": "Asia/Shanghai",
    "mode": "messages",
    "username": "wxid_xxx",
    "limit": 100,
    "returned": 100,
    "has_more": true,
    "next_after_seq": 1773421988000,
    "next_args": ["messages", "--username", "wxid_xxx", "--after-seq", "1773421988000", "--limit", "100", "--format", "json"]
  },
  "items": []
}
```

运行行为：

- 命令读取本地解密缓存里的 `message/message_*.db`，不会扫描微信进程内存。
- 如果 `~/.wxview/cache/<account>/index/messages.db` 是 ready 且准实时，命令会先从索引查当前页 row refs，再回原 decrypted cache 读取正文并复用同一套 normalize / media 解析逻辑。
- 如果索引 missing、stale、schema_mismatch 或快路径失败，命令会自动回落到旧扫描逻辑。
- 默认排序是 `create_time` 正序，再按 `seq`、本地行 id、分片名稳定排序。机器分页优先使用 `--after-seq` 或 `meta.next_args`，避免用大 `--offset` 反复扫描。
- 默认查询只读取现有完整缓存；自动刷新由 daemon 负责维护。
- `--refresh` 会显式刷新消息正文分片和消息辅助库，优先请求 daemon；daemon 不在时在当前进程刷新。
- 如果缺少对应 message DB 的 key，命令会提示先带进程内存读取权限运行 `wxview init`，不会在查询路径临时扫描 key。
- 当前支持图片、视频、文件和语音的本地路径解析。图片 `.dat` 会自动解密；视频会先查 `message_resource.db` 的 hardlink 元数据，再回退到 WeChat 本地目录 best-effort 查找，普通视频直接返回路径，可识别的 `.dat` 视频会规范化到媒体缓存；文件会优先查 hardlink 元数据并回退到本地文件目录；语音会从 `message/media_*.db` 的 `VoiceInfo.voice_data` 导出为 `.silk` 到媒体缓存。

### `wxview search`

搜索消息正文。当前保持本地扫描 selected conversations 的语义：对解码后的 `content` 和已解析的 `content_detail` 做包含匹配，避免 FTS 分词导致中文或半截关键词漏搜。

```sh
./bin/wxview search --query "AI" --date today --format json
./bin/wxview search --query "开会" --kind chatroom --chat-query 项目 --limit 50 --format table
./bin/wxview search --query "合同" --username wxid_xxx --start "2026-05-01" --format json
```

`--query` 搜索解码后的 `content` 和已解析的 `content_detail`。会话范围可以用
`--username` 精确指定，也可以用 `--kind` / `--chat-query` 从联系人缓存中筛选。
JSON 输出同样是 `{meta, items}` envelope，`items` 使用和 `messages` / `timeline`
相同的消息 schema。搜索结果按 `create_time` 倒序返回。

### `wxview timeline`

按时间范围查询一批会话，并合并成一条全局时间线。这个命令适合给 AI
或脚本读取“今天所有 AI 群聊内容”“昨天某类客户群发生了什么”这类跨会话上下文。

性能注意：索引 ready、准实时且覆盖全部选中会话时，`timeline` 会用普通 B-tree 索引按全局时间顺序取候选 row refs，再回源 cache 读取完整消息。索引不可用或没有覆盖全部会话时，会回落到按会话 fan-out 扫描本地 `message/message_*.db` 分片，合并后再应用全局 `--limit`。仍然建议使用小时间窗口和明确筛选，例如 `--username wxid_xxx --date today` 或 `--kind chatroom --query AI --date today`；避免对 `--kind all` 使用宽时间范围。

```sh
./bin/wxview timeline --kind chatroom --query AI --date today --limit 200 --format json
./bin/wxview timeline --kind chatroom --query AI --start "2026-05-14" --end "2026-05-14" --format json
./bin/wxview timeline --kind chatroom --query AI --start "2026-05-14 00:00:00" --end "2026-05-14 23:59:59" --limit 200 --cursor TOKEN --format json
./bin/wxview timeline --username wxid_xxx --date yesterday --format jsonl
./bin/wxview timeline --kind chatroom --query AI --date today --no-index --format json
```

会话选择参数：

- `--kind all|friend|chatroom|other`：按联系人类型选择会话。
- `--query TEXT`：筛选会话的 `username`、`alias`、`remark`、`nick_name`；
  不搜索消息正文。
- `--username TEXT`：精确指定一个会话，不能和 `--kind` / `--query` 同时使用。

时间范围参数：

- `--date today|yesterday|YYYY-MM-DD`：选择一个本地自然日。
- 或同时使用 `--start` 和 `--end`；`--date` 不能与它们混用。

分页参数：

- `--limit`：默认 `200`，最大 `1000`。
- `--cursor`：使用上一次 `meta.next_cursor` 继续读取。继续分页时保留相同查询条件，只追加 `--cursor`。

机器调用建议直接执行 `meta.next_args`，不需要理解 cursor 内部结构。
`matched_chats` 表示筛选命中的会话数量，不表示本页或本时间范围内实际有消息的会话数量。

`timeline --format json` 返回 envelope：

```json
{
  "meta": {
    "schema_version": 1,
    "timezone": "Asia/Shanghai",
    "mode": "timeline",
    "kind": "chatroom",
    "query": "AI",
    "start": "2026-05-14 00:00:00",
    "end": "2026-05-14 23:59:59",
    "limit": 200,
    "returned": 200,
    "matched_chats": 12,
    "has_more": true,
    "next_cursor": "TOKEN",
    "next_args": ["timeline", "--kind", "chatroom", "--query", "AI", "--start", "2026-05-14 00:00:00", "--end", "2026-05-14 23:59:59", "--limit", "200", "--cursor", "TOKEN", "--format", "json"]
  },
  "items": []
}
```

可靠分页建议使用 `--format json` 并读取 `meta.next_args`。`jsonl`、`csv`
和 `table` 只输出 `items`，便于管道处理。

### `wxview favorites`

查询微信收藏本地缓存 `favorite/favorite.db`。这个库是可选数据面；如果当前账号没有拿到对应 key，先带进程内存读取权限运行 `wxview init`，如果仍缺失则等微信打开过收藏相关数据后再重试。

```sh
./bin/wxview favorites --type article --limit 20 --format json
./bin/wxview favorites --query "AI" --format table
./bin/wxview favorites --refresh --format jsonl
```

常用参数：

- `--type text|image|voice|video|article|location|file|chat_history|note|card|video_channel`：按收藏类型过滤。
- `--query TEXT`：搜索收藏 XML / 正文内容。
- `--limit` / `--offset`：分页。
- `--refresh`：刷新 `favorite/favorite.db` 本地缓存后再查；daemon 正在运行时优先请求 daemon 刷新。

JSON 输出字段包括 `id`、`type`、`type_code`、`time`、`timestamp`、`summary`、`content`、`url`、`content_detail`、`content_items`、`from_username` 和 `source_chat_username`。`summary` 是从收藏 XML 中提取的简要标题/描述；`content` 是可读正文，笔记/聊天记录类收藏会把文本子项按顺序合并到这里；`content_detail` / `content_items` 会展开安全的载体信息，例如文件格式、大小、本地源路径、缩略图路径、普通 `http(s)` URL、只有微信 CDN 元数据时的 `remote_locator`、md5、message UUID 和 `media_status`。`url` 只表示可直接当 URL 使用的地址；微信 CDN 的十六进制 locator 会放在 `remote_locator` / `thumb_remote_locator`。CDN key 不会复制到输出里。

### `wxview articles`

查询公众号 / 订阅号账号，以及它们在消息缓存中的 appmsg 文章、视频号、小程序等分享记录。账号列表来自 `contact/contact.db`，文章记录来自 `message/message_*.db` 和 best-effort 的 `message/biz_message_*.db` 缓存。

```sh
./bin/wxview articles --list-accounts --format json
./bin/wxview articles --username gh_xxx --limit 20 --format json
./bin/wxview articles --query "AI" --date today --format table
```

选择方式：

- `--list-accounts`：只列出联系人表里的公众号 / 订阅号候选账号。
- `--username`：精确指定一个公众号 username。
- `--query TEXT`：筛选账号的 `username`、`alias`、`remark`、`nick_name`、`description`，再扫描命中的账号消息。

时间参数与 `messages` 类似，支持 `--date` 或 `--start` / `--end`。输出字段包括 `account_username`、`account_display_name`、`time`、`timestamp`、`type`、`text`、`title`、`desc`、`url`、`source_username`、`source_display_name`、`nickname` 和 `message_id`。

性能注意：这个命令会按账号 fan-out 读取消息缓存。查询所有公众号或很宽时间范围时可能较慢，优先用 `--username`、`--query` 和小时间窗口。

### `wxview sns`

查询朋友圈 SNS 本地缓存 `sns/sns.db`，当前支持 feed、正文搜索和通知。

```sh
./bin/wxview sns feed --date today --limit 20 --format json
./bin/wxview sns search "AI" --start "2026-05-01" --format table
./bin/wxview sns notifications --include-read --limit 20 --format json
```

子命令：

- `feed`：列出本地缓存里的朋友圈动态。
- `search KEYWORD`：搜索朋友圈动态正文。
- `notifications`：列出点赞/评论通知，默认只返回未读；加 `--include-read` 返回已读通知。

feed / search 支持 `--username`、`--date`、`--start`、`--end`、`--limit`、`--offset`、`--refresh`；daemon 正在运行时优先请求 daemon 刷新。输出媒体字段包括 URL、缩略图、md5、尺寸、大小、视频时长等可用元数据；不会把 CDN key/token 类字段复制到输出里。

## 未来性能 TODO

下面这些是后续可能的性能优化方向，不改变当前 item schema。

- `messages` 查询下推分页：在保持跨 `message_*.db` 分片全局排序正确的前提下，
  按分片使用 `limit + offset` 或游标上界减少读取量，避免大群/长时间范围全量加载后再分页。
- `messages` 优先使用 `sort_seq` / `create_time` 的可用索引：查询最新页、继续页时尽量走
  SQLite 已有索引，减少临时排序和大结果集传输。
- `timeline` 查询预估：增加类似 `--explain` / `--dry-run` 的模式，只返回命中的会话数量、
  时间范围和可能扫描的消息缓存，帮助 AI/脚本在执行昂贵查询前缩小范围。
- `timeline` 分批 fan-out：对多会话查询做更细的批处理和早停策略，优先读取最可能落入当前页的候选行，
  但必须保持 cursor 不重复、不漏数据。
- meta 标记快慢路径：有索引后，`messages` 和 `timeline` 的 JSON meta 可以加入
  `indexed: true|false`，让调用方知道当前结果来自索引快路径还是 V1 慢路径。
- 更细粒度的增量索引：当前 `index refresh` 按已索引到的 `local_id` 补新增 rows；后续可以继续做分片级调度和更精确的变更估计。

## 本地状态

- 配置目录：`~/.wxview/`
- key 文件：`~/.wxview/cache/<account>/keys.json`
- 联系人缓存：`~/.wxview/cache/<account>/contact/contact.db`
- 会话缓存：`~/.wxview/cache/<account>/session/session.db`
- 消息缓存：`~/.wxview/cache/<account>/message/message_*.db`
- 消息索引：`~/.wxview/cache/<account>/index/messages.db`
- 可选 biz 消息缓存：`~/.wxview/cache/<account>/message/biz_message_*.db`
- 可选媒体缓存 DB：`~/.wxview/cache/<account>/message/media_*.db`
- 头像缓存 DB：`~/.wxview/cache/<account>/head_image/head_image.db`
- 收藏缓存：`~/.wxview/cache/<account>/favorite/favorite.db`
- 朋友圈缓存：`~/.wxview/cache/<account>/sns/sns.db`
- 媒体导出/解密缓存：`~/.wxview/cache/<account>/media/`
- 增量消息状态：`~/.wxview/cache/<account>/state/new_messages.json`
- 缓存刷新元数据：`~/.wxview/cache/<account>/mtime.json`
- daemon socket：`~/.wxview/wxview.sock`

CLI 不会打印完整数据库 key。`wxview init --verbose` 只打印短 fingerprint。

如果 macOS 上之前用 `sudo` 产生过 root-owned 状态，可以修复一次权限：

```sh
sudo chown -R "$USER":staff ~/.wxview
```

## 权限说明

获取 key 需要读取正在运行的微信进程内存。失败时通常需要确认：

- 微信正在运行。
- macOS 使用了 `sudo`，且允许当前终端读取目标进程。
- Windows 在管理员终端中运行，且微信进程名是 `Weixin.exe` 或 `WeChat.exe`。
- 某些 macOS Hardened Runtime 构建可能需要本地研究环境下重新签名微信并开启 `get-task-allow`。

## 致谢

Wxview 的实现过程中参考并受益于这些开源项目：

- 特别感谢 [wechat-decrypt](https://github.com/ylytdeng/wechat-decrypt)：这个项目提供了 macOS / Windows 微信数据库解密方向上的关键参考，是 Wxview 能够落地本地解密与缓存流程的重要基础。
- 感谢 [wechat-cli](https://github.com/huohuoer/wechat-cli)：它在本地微信数据 CLI 化、查询体验和使用场景上提供了有价值的参考。
- 感谢 [wx-cli](https://github.com/jackwener/wx-cli)：它在命令设计、媒体解析和面向自动化使用的数据输出上提供了有价值的参考。

## V1 边界

当前版本只支持：

- macOS 微信 4.x
- Windows 微信 4.x（`xwechat_files\<account>\db_storage` 数据布局）
- `contact/contact.db`
- 联系人和群查询
- `session/session.db`
- 最近会话、未读会话和增量新消息
- `message/message_*.db`
- 按明确 username 查询聊天记录、消息正文搜索、常见 XML 摘要和图片/视频/文件/语音本地路径解析
- `message/media_*.db` 语音数据导出
- `head_image/head_image.db` 本地头像导出
- 群成员和群主
- `favorite/favorite.db`
- `message/biz_message_*.db`
- 公众号 / 订阅号文章和视频号 appmsg
- `sns/sns.db` 朋友圈 feed / 搜索 / 通知

当前版本不支持：

- Linux
- macOS 微信 3.x
- SILK 转 MP3/WAV 或语音转写
- WAL patch
- 公开 Web API

这些能力后续可以在同一套 key、decrypt、cache、service 架构上继续扩展。
