#!/usr/bin/env python3
"""X Console 资讯小接口（B241）。

跑在虚拟机上，只读 ai-news 的 SQLite 库，再把 x-assistant 写下的 tweets.jsonl 导入自己的 tweets.db。
面板经过自己的代理访问这里。接口格式见 docs/specs/B241.md。

只用 Python 标准库。正文是 zstd 压缩的，解压要 zstandard（ai-news 的 venv 里已经装了）。
令牌从环境变量 XC_FEEDS_TOKEN 读。只监听 127.0.0.1。
"""
from __future__ import annotations

import argparse
import hmac
import io
import json
import logging
import os
import re
import sqlite3
import sys
import threading
from collections import Counter
from datetime import datetime, timedelta, timezone
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
from urllib.parse import parse_qs, quote, urlsplit

try:
    import zstandard
except ImportError:  # 没装也能跑，只是拿不到正文
    zstandard = None

log = logging.getLogger("x-console-feeds")

DEFAULT_LISTEN = "127.0.0.1:8790"
DEFAULT_NEWS_DB = "/root/.hermes/scripts/ai-news/ai_intel.db"
DEFAULT_TWEETS_JSONL = "/root/.hermes/remote-chrome/tweets.jsonl"
DEFAULT_TWEETS_DB = "/root/.hermes/remote-chrome/tweets.db"

DEFAULT_LIMIT = 50
MAX_LIMIT = 100
MAX_TERMS = 8  # 关键词最多取前 8 个词
MAX_QUERY_LEN = 200
MAX_FILTER_LEN = 100
LIST_SUMMARY_CHARS = 2000
DETAIL_SUMMARY_CHARS = 20000
TWEET_TEXT_CHARS = 20000
# 单个响应不超过 2 MB。正文和译文各最多 800000 字节，其余字段加起来远小于 400 KB。
MAX_RESPONSE_BYTES = 2 << 20
MAX_CONTENT_BYTES = 800_000
MAX_INT64 = (1 << 63) - 1

# 中日韩文字没有空格分词，FTS5 默认分词器会把一整段连着的字当成一个词，
# 搜“模型”找不到“大模型发布”。含这些字的词改用 LIKE。
NO_SPACE_SCRIPTS = re.compile(
    "[\u3040-\u30ff\u3400-\u4dbf\u4e00-\u9fff\uac00-\ud7af\uf900-\ufaff\uff66-\uff9f]"
)
# 只由字母和数字组成的词才走 FTS。带标点的词（gpt-5、100%、a"b）的分词结果和用户看到的不一样，改用 LIKE 按原样找。
PLAIN_WORD = re.compile(r"[^\W_]+")
CONTROL_CHARS = re.compile(r"[\x00-\x1f\x7f]")
TAG_SPLIT = re.compile(r"[,，/|\s]+")

NEWS_LIKE_COLUMNS = ("title", "title_cn", "summary", "summary_cn", "tags", "source", "category")
TWEET_LIKE_COLUMNS = ("text", "author")

TWEETS_SCHEMA = """
CREATE TABLE IF NOT EXISTS tweets (
    id TEXT PRIMARY KEY,
    idn INTEGER NOT NULL,
    author TEXT NOT NULL DEFAULT '',
    time TEXT NOT NULL DEFAULT '',
    url TEXT NOT NULL DEFAULT '',
    text TEXT NOT NULL DEFAULT '',
    collected_at TEXT
);
CREATE INDEX IF NOT EXISTS idx_tweets_idn ON tweets(idn DESC);
CREATE TABLE IF NOT EXISTS meta (key TEXT PRIMARY KEY, value TEXT NOT NULL);
"""
# 没有 FTS5 的 SQLite 建这张表会报错，这时搜索退回 LIKE。测试里改这个变量来模拟。
TWEETS_FTS_DDL = "CREATE VIRTUAL TABLE tweets_fts USING fts5(tweet_id UNINDEXED, text, author)"


class HTTPError(Exception):
    def __init__(self, status: int, message: str):
        super().__init__(message)
        self.status = status
        self.message = message


def bad(message: str) -> HTTPError:
    return HTTPError(400, message)


# ---------- 搜索词 ----------


def split_terms(q: str) -> list[str]:
    q = CONTROL_CHARS.sub(" ", q)
    return q.split()[:MAX_TERMS]


def like_pattern(term: str) -> str:
    esc = term.replace("\\", "\\\\").replace("%", "\\%").replace("_", "\\_")
    return f"%{esc}%"


def fts_phrase(term: str) -> str:
    """把一个词变成 FTS5 里的字面短语：整个包进双引号，里面的双引号写两遍，结尾加 * 做前缀匹配。
    引号里的 AND、OR、NOT、NEAR、列名: 和 ^ 都只是普通文字，用户改不了查询语法。"""
    return '"' + term.replace('"', '""') + '"*'


def needs_like(term: str) -> bool:
    return bool(NO_SPACE_SCRIPTS.search(term)) or not PLAIN_WORD.fullmatch(term)


def search_clause(
    q: str | None, like_columns: tuple[str, ...], fts_subquery: str | None
) -> tuple[list[str], list]:
    """返回 (条件列表, 参数列表)。fts_subquery 为空表示没有 FTS 表，所有词都用 LIKE。
    fts_subquery 形如 'id IN (SELECT rowid FROM items_fts WHERE items_fts MATCH ?)'。"""
    if not q:
        return [], []
    where: list[str] = []
    args: list = []
    fts_terms: list[str] = []
    for term in split_terms(q):
        if fts_subquery and not needs_like(term):
            fts_terms.append(fts_phrase(term))
            continue
        cols = " OR ".join(f"{c} LIKE ? ESCAPE '\\'" for c in like_columns)
        where.append(f"({cols})")
        args.extend([like_pattern(term)] * len(like_columns))
    if fts_terms:
        where.append(fts_subquery)
        args.append(" ".join(fts_terms))
    return where, args


# ---------- 参数 ----------


def first(params: dict, name: str) -> str | None:
    v = params.get(name)
    if not v:
        return None
    return v[0]


def int_param(params: dict, name: str, lo: int, hi: int, clamp: bool = False) -> int | None:
    raw = first(params, name)
    if raw is None or raw.strip() == "":
        return None
    try:
        n = int(raw)
    except ValueError:
        raise bad(f"{name} 要是整数")
    if n < lo:
        raise bad(f"{name} 不能小于 {lo}")
    if n > hi:
        if clamp:
            return hi
        raise bad(f"{name} 不能大于 {hi}")
    return n


def bool_param(params: dict, name: str) -> bool | None:
    raw = first(params, name)
    if raw is None or raw == "":
        return None
    if raw.lower() in ("1", "true"):
        return True
    if raw.lower() in ("0", "false"):
        return False
    raise bad(f"{name} 要是 true 或 false")


def text_param(params: dict, name: str, max_len: int) -> str | None:
    raw = first(params, name)
    if raw is None:
        return None
    raw = CONTROL_CHARS.sub(" ", raw).strip()
    if not raw:
        return None
    if len(raw) > max_len:
        raise bad(f"{name} 太长")
    return raw


def cursor_param(params: dict) -> int | None:
    raw = first(params, "before")
    if raw is None or raw == "":
        return None
    if not re.fullmatch(r"[0-9]{1,19}", raw) or int(raw) > MAX_INT64:
        raise bad("before 格式不对")
    return int(raw)


def clip(value, max_chars: int):
    if value is None:
        return None
    s = str(value)
    return s if len(s) <= max_chars else s[:max_chars]


# ---------- 新闻 ----------


def parse_tags(raw) -> list[str]:
    if not raw:
        return []
    if isinstance(raw, str):
        try:
            parsed = json.loads(raw)
        except ValueError:
            parsed = [t for t in TAG_SPLIT.split(raw) if t]
        raw = parsed
    if not isinstance(raw, list):
        return []
    return [str(t) for t in raw if str(t).strip()]


class NewsDB:
    ITEM_COLUMNS = (
        "id", "title", "title_cn", "summary", "summary_cn", "url", "source", "category",
        "tags", "importance", "pushed", "published_at", "fetched_at",
    )

    def __init__(self, path: str):
        self.path = path

    def connect(self) -> sqlite3.Connection:
        # 每个请求单独开一个只读连接。路径里的特殊字符要转义成 URI。
        uri = "file:" + quote(os.path.abspath(self.path)) + "?mode=ro"
        conn = sqlite3.connect(uri, uri=True, timeout=5)
        conn.execute("PRAGMA busy_timeout = 5000")
        return conn

    @staticmethod
    def tables(conn: sqlite3.Connection) -> set[str]:
        return {r[0] for r in conn.execute("SELECT name FROM sqlite_master WHERE type IN ('table', 'view')")}

    def select_items(self, conn: sqlite3.Connection) -> str:
        """老库可能还没有 AI 补的那几列，缺的列按空值处理。"""
        have = {r[1] for r in conn.execute("PRAGMA table_info(items)")}
        if not have:
            raise sqlite3.OperationalError("no such table: items")
        return ", ".join(c if c in have else f"NULL AS {c}" for c in self.ITEM_COLUMNS)

    @staticmethod
    def row_to_item(row, summary_chars: int) -> dict:
        (id_, title, title_cn, summary, summary_cn, url, source, category, tags, importance, pushed,
         published_at, fetched_at) = row
        try:
            importance = int(importance) if importance is not None else None
        except (TypeError, ValueError):
            importance = None
        return {
            "id": id_,
            "title": title or "",
            "titleCn": title_cn or None,
            "summary": clip(summary, summary_chars) or None,
            "summaryCn": clip(summary_cn, summary_chars) or None,
            "url": url or "",
            "source": source or "",
            "category": category or "",
            "tags": parse_tags(tags),
            "importance": importance,
            "pushed": bool(pushed),
            "publishedAt": published_at or None,
            "fetchedAt": fetched_at or None,
        }

    def health(self, conn: sqlite3.Connection) -> dict:
        count, last = conn.execute("SELECT COUNT(*), MAX(fetched_at) FROM items").fetchone()
        return {"count": count, "lastFetchedAt": last}

    def list(self, params: dict, now: datetime) -> dict:
        limit = int_param(params, "limit", 1, MAX_LIMIT, clamp=True) or DEFAULT_LIMIT
        q = text_param(params, "q", MAX_QUERY_LEN)
        tag = text_param(params, "tag", MAX_FILTER_LEN)
        category = text_param(params, "category", MAX_FILTER_LEN)
        days = int_param(params, "days", 1, 3650)
        min_importance = int_param(params, "minImportance", 1, 5)
        pushed = bool_param(params, "pushed")
        before = cursor_param(params)

        conn = self.connect()
        try:
            cols = self.select_items(conn)
            has_fts = "items_fts" in self.tables(conn)
            for use_fts in ((True, False) if has_fts else (False,)):
                where: list[str] = []
                args: list = []
                if before is not None:
                    where.append("id < ?")
                    args.append(before)
                if category:
                    where.append("category = ?")
                    args.append(category)
                if tag:
                    # tags 是 JSON 数组文本。带引号匹配，所以 ai 不会匹配到 train。
                    where.append("tags LIKE ? ESCAPE '\\'")
                    args.append(like_pattern(json.dumps(tag, ensure_ascii=False)))
                if days:
                    where.append("fetched_at >= ?")
                    args.append((now - timedelta(days=days)).isoformat())
                if min_importance:
                    where.append("importance >= ?")
                    args.append(min_importance)
                if pushed is True:
                    where.append("pushed = 1")
                elif pushed is False:
                    where.append("(pushed IS NULL OR pushed = 0)")
                sw, sa = search_clause(
                    q, NEWS_LIKE_COLUMNS,
                    "id IN (SELECT rowid FROM items_fts WHERE items_fts MATCH ?)" if use_fts else None,
                )
                where += sw
                args += sa
                sql = f"SELECT {cols} FROM items"
                if where:
                    sql += " WHERE " + " AND ".join(where)
                sql += " ORDER BY id DESC LIMIT ?"
                args.append(limit + 1)
                try:
                    rows = conn.execute(sql, args).fetchall()
                except sqlite3.OperationalError:
                    if use_fts:  # FTS 查询出错，退回 LIKE 再试一次
                        log.warning("FTS 查询失败，退回 LIKE", exc_info=True)
                        continue
                    raise
                break
        finally:
            conn.close()
        more = len(rows) > limit
        items = [self.row_to_item(r, LIST_SUMMARY_CHARS) for r in rows[:limit]]
        return {"items": items, "next": str(items[-1]["id"]) if more and items else None}

    def facets(self, now: datetime) -> dict:
        conn = self.connect()
        try:
            self.select_items(conn)
            have = {r[1] for r in conn.execute("PRAGMA table_info(items)")}
            categories = [
                r[0] for r in conn.execute(
                    "SELECT DISTINCT category FROM items WHERE category IS NOT NULL AND category != '' ORDER BY category"
                )
            ]
            since = (now - timedelta(days=30)).isoformat()
            counter: Counter = Counter()
            if "tags" in have:  # 老库可能还没有 tags 列
                for (raw,) in conn.execute(
                    "SELECT tags FROM items WHERE fetched_at >= ? AND tags IS NOT NULL AND tags != ''", (since,)
                ):
                    counter.update(set(parse_tags(raw)))
        finally:
            conn.close()
        top = sorted(counter.items(), key=lambda kv: (-kv[1], kv[0]))[:50]
        return {"categories": categories, "tags": [{"tag": t, "count": n} for t, n in top]}

    def detail(self, item_id: int) -> dict:
        conn = self.connect()
        try:
            cols = self.select_items(conn)
            row = conn.execute(f"SELECT {cols} FROM items WHERE id = ?", (item_id,)).fetchone()
            if row is None:
                raise HTTPError(404, "没有这条新闻")
            out = self.row_to_item(row, DETAIL_SUMMARY_CHARS)
            tables = self.tables(conn)
            content = {"status": "missing", "markdown": None, "markdownCn": None}
            if "article_contents" in tables:
                c = conn.execute(
                    "SELECT status, markdown_zstd, content_cn_zstd FROM article_contents WHERE item_id = ?",
                    (item_id,),
                ).fetchone()
                if c is not None:
                    md, cut1 = unzstd(c[1], MAX_CONTENT_BYTES)
                    md_cn, cut2 = unzstd(c[2], MAX_CONTENT_BYTES)
                    content = {"status": c[0] or "pending", "markdown": md, "markdownCn": md_cn}
                    if cut1 or cut2:
                        content["truncated"] = True
            media = []
            if "article_media" in tables:
                for url, kind, width, height in conn.execute(
                    "SELECT public_url, media_type, width, height FROM article_media "
                    "WHERE item_id = ? AND public_url IS NOT NULL AND public_url != '' "
                    "AND (error IS NULL OR error = '') ORDER BY id",
                    (item_id,),
                ):
                    media.append({"url": url, "type": kind or "", "width": width, "height": height})
        finally:
            conn.close()
        out["content"] = content
        out["media"] = media
        return out


def unzstd(blob, limit: int) -> tuple[str | None, bool]:
    """解压 zstd 文本，最多取 limit 字节。返回 (文本, 是否被截断)。"""
    if not blob:
        return None, False
    if zstandard is None:
        log.warning("没有装 zstandard，取不到正文。请用 ai-news 的 .venv/bin/python3 运行")
        return None, False
    try:
        data = bytearray()
        with zstandard.ZstdDecompressor().stream_reader(io.BytesIO(bytes(blob))) as reader:
            while len(data) <= limit:
                chunk = reader.read(min(1 << 16, limit + 1 - len(data)))
                if not chunk:
                    break
                data += chunk
    except zstandard.ZstdError:
        log.warning("正文解压失败", exc_info=True)
        return None, False
    truncated = len(data) > limit
    # 截在字节上可能切断最后一个字，ignore 会丢掉这半个字
    text = bytes(data[:limit]).decode("utf-8", "ignore")
    return (text or None), truncated


# ---------- 推文 ----------


class TweetsDB:
    def __init__(self, db_path: str, jsonl_path: str):
        self.db_path = db_path
        self.jsonl_path = jsonl_path
        self.lock = threading.Lock()

    def connect(self) -> sqlite3.Connection:
        conn = sqlite3.connect(self.db_path, timeout=5)
        conn.execute("PRAGMA busy_timeout = 5000")
        try:
            conn.execute("PRAGMA journal_mode = WAL")
        except sqlite3.OperationalError:
            pass
        conn.executescript(TWEETS_SCHEMA)
        self.ensure_fts(conn)
        return conn

    @staticmethod
    def has_fts(conn: sqlite3.Connection) -> bool:
        return conn.execute("SELECT 1 FROM sqlite_master WHERE name = 'tweets_fts'").fetchone() is not None

    def ensure_fts(self, conn: sqlite3.Connection) -> None:
        if self.has_fts(conn):
            return
        try:
            conn.execute(TWEETS_FTS_DDL)
        except sqlite3.OperationalError:
            return  # 这个 SQLite 没有 FTS5，搜索用 LIKE
        # 库先建好、后来才有 FTS5 时，把已有的推文补进索引
        conn.execute("INSERT INTO tweets_fts(tweet_id, text, author) SELECT id, text, author FROM tweets")
        conn.commit()

    @staticmethod
    def parse_line(raw: bytes) -> dict:
        obj = json.loads(raw.decode("utf-8"))
        if not isinstance(obj, dict):
            raise ValueError("不是 JSON 对象")
        tid = obj.get("id")
        if isinstance(tid, int) and not isinstance(tid, bool):
            tid = str(tid)
        if not isinstance(tid, str) or not re.fullmatch(r"[0-9]{1,19}", tid) or int(tid) > MAX_INT64 or int(tid) < 1:
            raise ValueError("id 不对")

        def s(key: str) -> str:
            v = obj.get(key)
            return v if isinstance(v, str) else ""

        collected = obj.get("collectedAt", obj.get("collected_at"))
        return {
            "id": tid,
            "idn": int(tid),
            "author": s("author"),
            "time": s("time"),
            "url": s("url"),
            "text": s("text"),
            "collected_at": collected if isinstance(collected, str) else None,
        }

    def import_new(self) -> int:
        """从上次读到的位置接着导入 tweets.jsonl 里新写的行。返回新增条数。"""
        with self.lock:
            conn = self.connect()
            try:
                return self._import(conn)
            finally:
                conn.close()

    def _import(self, conn: sqlite3.Connection) -> int:
        row = conn.execute("SELECT value FROM meta WHERE key = 'offset'").fetchone()
        offset = int(row[0]) if row else 0
        try:
            size = os.path.getsize(self.jsonl_path)
        except OSError:
            return 0  # collect.js 还没写过
        if size < offset:
            log.warning("tweets.jsonl 变小了（%d < %d），从头重新读，重复的推文会被跳过", size, offset)
            offset = 0
        if size == offset:
            return 0
        added = skipped = 0
        has_fts = self.has_fts(conn)
        conn.execute("BEGIN IMMEDIATE")
        try:
            with open(self.jsonl_path, "rb") as f:
                f.seek(offset)
                for raw in f:
                    if not raw.endswith(b"\n"):
                        break  # 最后一行还没写完，下次再读
                    start = offset
                    offset += len(raw)
                    if not raw.strip():
                        continue
                    try:
                        t = self.parse_line(raw)
                        cur = conn.execute(
                            "INSERT OR IGNORE INTO tweets (id, idn, author, time, url, text, collected_at) "
                            "VALUES (:id, :idn, :author, :time, :url, :text, :collected_at)", t,
                        )
                        if cur.rowcount == 1 and has_fts:
                            conn.execute(
                                "INSERT INTO tweets_fts (tweet_id, text, author) VALUES (?, ?, ?)",
                                (t["id"], t["text"], t["author"]),
                            )
                        added += cur.rowcount
                    except (ValueError, UnicodeError, sqlite3.InterfaceError, sqlite3.DataError) as e:
                        skipped += 1
                        log.warning("tweets.jsonl 第 %d 字节处的行有问题，已跳过：%s", start, e)
            conn.execute("INSERT OR REPLACE INTO meta (key, value) VALUES ('offset', ?)", (str(offset),))
            conn.execute("COMMIT")
        except BaseException:
            conn.execute("ROLLBACK")
            raise
        if added or skipped:
            log.info("导入推文：新增 %d 条，跳过 %d 行", added, skipped)
        return added

    @staticmethod
    def row_to_tweet(row) -> dict:
        id_, author, time_, url, text, collected = row
        return {
            "id": id_, "author": author, "time": time_, "url": url,
            "text": clip(text, TWEET_TEXT_CHARS), "collectedAt": collected,
        }

    def health(self) -> dict:
        conn = self.connect()
        try:
            count, last = conn.execute("SELECT COUNT(*), MAX(NULLIF(time, '')) FROM tweets").fetchone()
        finally:
            conn.close()
        return {"count": count, "lastAt": last}

    def list(self, params: dict) -> dict:
        limit = int_param(params, "limit", 1, MAX_LIMIT, clamp=True) or DEFAULT_LIMIT
        q = text_param(params, "q", MAX_QUERY_LEN)
        author = text_param(params, "author", MAX_FILTER_LEN)
        before = cursor_param(params)
        conn = self.connect()
        try:
            has_fts = self.has_fts(conn)
            for use_fts in ((True, False) if has_fts else (False,)):
                where: list[str] = []
                args: list = []
                if before is not None:
                    where.append("idn < ?")
                    args.append(before)
                if author:
                    where.append("author LIKE ? ESCAPE '\\'")
                    args.append(like_pattern(author))
                sw, sa = search_clause(
                    q, TWEET_LIKE_COLUMNS,
                    "id IN (SELECT tweet_id FROM tweets_fts WHERE tweets_fts MATCH ?)" if use_fts else None,
                )
                where += sw
                args += sa
                sql = "SELECT id, author, time, url, text, collected_at FROM tweets"
                if where:
                    sql += " WHERE " + " AND ".join(where)
                sql += " ORDER BY idn DESC LIMIT ?"
                args.append(limit + 1)
                try:
                    rows = conn.execute(sql, args).fetchall()
                except sqlite3.OperationalError:
                    if use_fts:
                        log.warning("FTS 查询失败，退回 LIKE", exc_info=True)
                        continue
                    raise
                break
        finally:
            conn.close()
        more = len(rows) > limit
        items = [self.row_to_tweet(r) for r in rows[:limit]]
        return {"items": items, "next": items[-1]["id"] if more and items else None}


# ---------- HTTP ----------


class Service:
    def __init__(self, token: str, news_db: str, tweets_db: str, tweets_jsonl: str, now=None):
        if not token:
            raise ValueError("令牌不能为空")
        self.token = token.encode("utf-8")
        self.news = NewsDB(news_db)
        self.tweets = TweetsDB(tweets_db, tweets_jsonl)
        self.now = now or (lambda: datetime.now(timezone.utc))

    def authorized(self, header: str) -> bool:
        scheme, _, cred = (header or "").partition(" ")
        if scheme.lower() != "bearer":
            return False
        return hmac.compare_digest(cred.strip().encode("utf-8"), self.token)

    def import_tweets(self) -> None:
        try:
            self.tweets.import_new()
        except Exception:
            log.exception("导入 tweets.jsonl 失败，先用已有的数据")

    def route(self, path: str, params: dict) -> dict:
        if path == "/v1/health":
            self.import_tweets()
            conn = self.news.connect()
            try:
                news = self.news.health(conn)
            finally:
                conn.close()
            return {"news": news, "tweets": self.tweets.health()}
        if path == "/v1/news":
            return self.news.list(params, self.now())
        if path == "/v1/news/facets":
            return self.news.facets(self.now())
        m = re.fullmatch(r"/v1/news/([0-9]{1,18})", path)
        if m:
            return self.news.detail(int(m.group(1)))
        if path == "/v1/tweets":
            self.import_tweets()
            return self.tweets.list(params)
        raise HTTPError(404, "没有这个地址")


class Handler(BaseHTTPRequestHandler):
    server_version = "x-console-feeds"
    timeout = 10  # 对方半天不发完请求就断开

    def log_message(self, fmt, *args):  # 日志里不记查询参数
        log.info("%s %s %s", self.command, urlsplit(self.path).path, args[1] if len(args) > 1 else "")

    def send_json(self, status: int, obj: dict) -> None:
        body = json.dumps(obj, ensure_ascii=False, separators=(",", ":")).encode("utf-8")
        if len(body) > MAX_RESPONSE_BYTES:
            status, body = 500, b'{"error":"response too large"}'
        self.send_response(status)
        self.send_header("Content-Type", "application/json; charset=utf-8")
        self.send_header("Content-Length", str(len(body)))
        self.send_header("Cache-Control", "no-store")
        self.end_headers()
        if self.command != "HEAD":
            self.wfile.write(body)

    def handle_any(self) -> None:
        svc: Service = self.server.service  # type: ignore[attr-defined]
        try:
            if not svc.authorized(self.headers.get("Authorization", "")):
                self.send_response(401)
                self.send_header("WWW-Authenticate", "Bearer")
                body = b'{"error":"unauthorized"}'
                self.send_header("Content-Type", "application/json; charset=utf-8")
                self.send_header("Content-Length", str(len(body)))
                self.end_headers()
                self.wfile.write(body)
                return
            if self.command != "GET":
                raise HTTPError(405, "只支持 GET")
            parts = urlsplit(self.path)
            try:
                params = parse_qs(parts.query, keep_blank_values=True, max_num_fields=50)
            except ValueError:
                raise bad("参数太多")
            self.send_json(200, svc.route(parts.path, params))
        except HTTPError as e:
            self.send_json(e.status, {"error": e.message})
        except sqlite3.Error:
            log.exception("读数据库出错")
            self.send_json(503, {"error": "database unavailable"})
        except Exception:
            log.exception("处理请求出错")
            self.send_json(500, {"error": "internal error"})

    do_GET = do_POST = do_PUT = do_PATCH = do_DELETE = do_HEAD = do_OPTIONS = handle_any


class Server(ThreadingHTTPServer):
    daemon_threads = True
    allow_reuse_address = True

    def __init__(self, addr, service: Service):
        self.service = service
        super().__init__(addr, Handler)


def parse_listen(value: str) -> tuple[str, int]:
    host, sep, port = value.rpartition(":")
    if not sep:
        host, port = "127.0.0.1", value
    host = host or "127.0.0.1"
    if host not in ("127.0.0.1", "localhost"):
        raise argparse.ArgumentTypeError("只能监听 127.0.0.1，别的机器要访问请走面板代理")
    try:
        n = int(port)
    except ValueError:
        raise argparse.ArgumentTypeError("端口要是数字")
    if not 0 <= n <= 65535:
        raise argparse.ArgumentTypeError("端口超出范围")
    return "127.0.0.1", n


def main(argv: list[str] | None = None) -> int:
    ap = argparse.ArgumentParser(description="X Console 资讯小接口")
    ap.add_argument("--listen", type=parse_listen, default=parse_listen(DEFAULT_LISTEN),
                    help=f"监听地址，只接受 127.0.0.1。默认 {DEFAULT_LISTEN}，只写端口也行")
    ap.add_argument("--news-db", default=DEFAULT_NEWS_DB, help="ai-news 的 ai_intel.db，只读")
    ap.add_argument("--tweets-jsonl", default=DEFAULT_TWEETS_JSONL, help="collect.js 追加写的 tweets.jsonl")
    ap.add_argument("--tweets-db", default=DEFAULT_TWEETS_DB, help="推文自己的库，不存在会新建")
    args = ap.parse_args(argv)
    logging.basicConfig(level=logging.INFO, format="%(levelname)s %(message)s")
    token = os.environ.get("XC_FEEDS_TOKEN", "")
    if not token:
        print("缺少环境变量 XC_FEEDS_TOKEN", file=sys.stderr)
        return 2
    if zstandard is None:
        log.warning("没有装 zstandard，新闻详情里不会有正文。请用 ai-news 的 .venv/bin/python3 运行")
    service = Service(token, args.news_db, args.tweets_db, args.tweets_jsonl)
    server = Server(args.listen, service)
    log.info("监听 %s:%d", *args.listen)
    try:
        server.serve_forever()
    except KeyboardInterrupt:
        pass
    return 0


if __name__ == "__main__":
    sys.exit(main())
