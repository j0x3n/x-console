# 资讯小接口

面板的“资讯”页面（`/feeds`）读的数据在虚拟机上。这个目录是虚拟机上要装的小接口。需求和接口格式见 `docs/specs/B241.md`。

数据流是这样的：面板的服务端通过虚拟机上的面板代理，请求本机的 `127.0.0.1:8790`。小接口只读，不改任何数据。

## 文件

| 文件 | 作用 |
| --- | --- |
| `feeds_api.py` | 小接口本体，只用 Python 标准库，解压正文要 `zstandard` |
| `test_feeds_api.py` | 测试，用标准库 `unittest` |
| `x-console-feeds.service` | systemd 单元 |
| `collect.js.patch` | 给 x-assistant 的补丁，让它把新推文存下来 |

## 安装

在虚拟机上用 root 操作。

1. 装面板代理，用设置页生成的配对码配对，步骤见 `docs/04-agent-protocol.md` 的“配对”一节。代理要在线，设置页的机器下拉框里才选得到它。
2. 复制小接口。

   ```sh
   mkdir -p /opt/x-console-feeds
   cp feeds_api.py /opt/x-console-feeds/
   ```

3. 生成令牌，写进环境文件，权限设成 0600。

   ```sh
   umask 077
   echo "XC_FEEDS_TOKEN=$(openssl rand -hex 32)" > /etc/x-console-feeds.env
   cat /etc/x-console-feeds.env
   ```

   最后一条命令会显示令牌，记下来，后面填进面板。
4. 装服务。先看一下 `x-console-feeds.service` 里的路径和你的机器是否一致，不一致就改。

   ```sh
   cp x-console-feeds.service /etc/systemd/system/
   systemctl daemon-reload
   systemctl enable --now x-console-feeds
   ```

5. 在虚拟机上验证。

   ```sh
   . /etc/x-console-feeds.env
   curl -s -H "Authorization: Bearer $XC_FEEDS_TOKEN" http://127.0.0.1:8790/v1/health
   ```

   正常会返回新闻和推文的条数。`tweets` 的条数现在是 0，打完下面的补丁以后才会增长。
6. 在面板里打开“设置”，进“资讯”标签：选这台机器，接口地址用默认的 `http://127.0.0.1:8790`，填上令牌，保存，点“测试连接”。改设置需要再验证一次两步验证码。

## 给 x-assistant 打补丁

补丁只改 `collect.js` 两处。

1. 有新推文时，每条加上 `collectedAt`，追加写进同目录的 `tweets.jsonl`。
2. 没有新推文时输出 `NO_NEW_TWEETS`。原来输出的是上一批推文，Hermes 如果不自己去重，会重复推送。

```sh
cd /root/.hermes/remote-chrome
patch -p1 --dry-run < /path/to/collect.js.patch   # 先试一下
patch -p1 < /path/to/collect.js.patch
./x-list-b-scrape.sh
wc -l tweets.jsonl
```

打完补丁后，跑一次 `x-list-b-scrape.sh`，`tweets.jsonl` 应该多出新行。补丁之前已经见过的推文不会补进来，页面里的推文从打补丁以后开始积累。

这个补丁 2026-10-17 已经提交到 x-assistant 的 `main`（`2b148c5`）。虚拟机上的目录如果是这个仓库的 git 副本，`git pull` 就行，不用再打补丁。ai-news 不用改。

## 接口

都是 GET，返回 JSON。每个请求都要带 `Authorization: Bearer <令牌>`，不对回 401。

| 地址 | 参数 | 返回 |
| --- | --- | --- |
| `/v1/health` | 无 | `{news: {count, lastFetchedAt}, tweets: {count, lastAt}}` |
| `/v1/news` | `q`、`tag`、`category`、`days`、`minImportance`、`pushed`、`before`、`limit` | `{items, next}` |
| `/v1/news/facets` | 无 | `{categories, tags: [{tag, count}]}` |
| `/v1/news/{id}` | 无 | 新闻加 `content` 和 `media` |
| `/v1/tweets` | `q`、`author`、`before`、`limit` | `{items, next}` |

- 新闻按 `id` 倒序，也就是抓取顺序。推文按推文编号倒序，也就是发布顺序。
- `before` 填上一页返回的 `next`。`next` 为空表示没有更多。
- `limit` 默认 50，最大 100。
- `days=1` 表示最近 24 小时。`pushed=true` 只看推过 Telegram 的，`pushed=false` 只看没推过的。
- 正文和译文各最多 800000 字节。超过时截断，`content.truncated` 为 `true`。整个响应不超过 2 MB。
- `content.status` 是 ai-news 里的抓取状态。还没抓取时是 `missing`。

### 搜索

关键词用空格分开，每个词都要命中，最多取前 8 个词。

- 由字母和数字组成的词，有 `items_fts` 时用 FTS5 按词首匹配。例如 `agen` 能找到 `agents`。
- 中文、日文、韩文的词，以及带标点的词（`gpt-5`、`100%`），用 `LIKE` 按原样找子串。因为 FTS5 默认分词器不会切开连着的汉字，搜“模型”找不到“大模型”。
- 没有 `items_fts`，或者 FTS 查询出错时，所有词都用 `LIKE`。
- 关键词永远当作普通文字。FTS 的词会整个包进双引号，里面的双引号写两遍。`LIKE` 里的 `%`、`_`、`\` 会转义。`OR`、`NOT`、`title:` 这些都不会改变查询。

## 数据从哪里来

- 新闻：只读打开 ai-news 的 `ai_intel.db`（`mode=ro`，`busy_timeout` 5 秒，每个请求单独开连接）。用到 `items`、`items_fts`、`article_contents`、`article_media` 四张表。缺的表和列会按空处理。
- 推文：每次收到推文请求，小接口从上次读到的位置接着读 `tweets.jsonl`，导入自己的 `tweets.db`。主键是推文编号，重复的跳过。坏行跳过，并在日志里记下位置。最后一行没写完时先不读。`tweets.jsonl` 变小（被替换了）时从头重读。
- `tweets.db` 放在 x-assistant 目录下。可以随时删掉，下次请求会从 `tweets.jsonl` 重建。

## 参数

| 参数 | 默认 |
| --- | --- |
| `--listen` | `127.0.0.1:8790`。只接受 127.0.0.1，只写端口也行 |
| `--news-db` | `/root/.hermes/scripts/ai-news/ai_intel.db` |
| `--tweets-jsonl` | `/root/.hermes/remote-chrome/tweets.jsonl` |
| `--tweets-db` | `/root/.hermes/remote-chrome/tweets.db` |

令牌从环境变量 `XC_FEEDS_TOKEN` 读，没有就不启动。令牌用常数时间比较。

## 出问题时

- 面板提示“接口没响应”：在虚拟机上跑 `systemctl status x-console-feeds` 和 `journalctl -u x-console-feeds -n 50`。再用上面第 5 步的 `curl` 看本机能不能访问。
- 面板提示“令牌不对”：环境文件里的令牌和面板设置里的不一样。改完环境文件要 `systemctl restart x-console-feeds`。
- 面板提示“虚拟机不在线”：面板代理没连上。在虚拟机上看代理服务有没有在跑。
- 新闻详情没有正文：日志里有“没装 zstandard”时，说明服务不是用 ai-news 的 `.venv/bin/python3` 启动的。也可能是 ai-news 还没抓这篇的正文，页面会显示“正文还没抓取”。
- 推文一直是 0 条：看 `tweets.jsonl` 有没有新行。没有就是补丁没打上，或者 `collect.js` 没有新推文。

## 测试

在仓库根目录运行：

```sh
python3 -m unittest discover -s deploy/feeds-api -v
```

测试会造一个和 ai-news 结构一样的临时库，有 FTS5 和没有 FTS5 都测。没装 `zstandard` 时，需要压缩正文的用例会跳过。想跑全部用例，用 ai-news 的 Python：

```sh
/root/.hermes/scripts/ai-news/.venv/bin/python3 -m unittest discover -s deploy/feeds-api -v
```

CI 里有一步跑这些测试。
