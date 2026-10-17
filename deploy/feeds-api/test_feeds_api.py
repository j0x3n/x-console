"""feeds_api.py 的测试。只用标准库 unittest。

造一个和 ai-news 结构一样的临时库来测，覆盖有 FTS5 和没有 FTS5 两种情况。
没装 zstandard 时，需要压缩正文的用例会跳过。
运行：python3 -m unittest discover -s deploy/feeds-api
"""
from __future__ import annotations

import json
import logging
import os
import sqlite3
import tempfile
import threading
import unittest
import urllib.error
import urllib.request
from datetime import datetime, timedelta, timezone
from unittest import mock

import feeds_api

try:
    import zstandard
except ImportError:
    zstandard = None

TOKEN = "test-token-123"
NOW = datetime(2026, 10, 17, 12, 0, 0, tzinfo=timezone.utc)

# 和 ai-news 的 fetch_ai_intel.py、ai_common.py、migrate_content_schema.py 里的建表语句一致
ITEMS_DDL = """
CREATE TABLE items (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    dedupe_key TEXT UNIQUE,
    title TEXT, url TEXT, source TEXT, category TEXT,
    published_at TEXT, fetched_at TEXT, summary TEXT, raw_text_snippet TEXT,
    relevance_score REAL DEFAULT 0, opportunity_score REAL DEFAULT 0, popularity_score REAL DEFAULT 0,
    pushed INTEGER DEFAULT 0,
    title_cn TEXT, summary_cn TEXT, tags TEXT, importance INTEGER, ai_enriched_at TEXT, search_text TEXT
)
"""
OLD_ITEMS_DDL = """
CREATE TABLE items (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    dedupe_key TEXT UNIQUE,
    title TEXT, url TEXT, source TEXT, category TEXT,
    published_at TEXT, fetched_at TEXT, summary TEXT, raw_text_snippet TEXT
)
"""
FTS_DDL = """
CREATE VIRTUAL TABLE items_fts USING fts5(
    item_id UNINDEXED, title, title_cn, summary, summary_cn, tags, source, category, url UNINDEXED
)
"""
CONTENT_DDL = """
CREATE TABLE article_contents (
    item_id INTEGER PRIMARY KEY, url TEXT NOT NULL, fetched_at TEXT NOT NULL,
    status TEXT NOT NULL DEFAULT 'pending', http_status INTEGER, content_type TEXT, final_url TEXT,
    title_extracted TEXT, raw_html_zstd BLOB, extracted_text_zstd BLOB, markdown_zstd BLOB,
    title_cn TEXT, summary_cn TEXT, content_cn_zstd BLOB, word_count INTEGER DEFAULT 0, lang TEXT, error TEXT
);
CREATE TABLE article_media (
    id INTEGER PRIMARY KEY AUTOINCREMENT, item_id INTEGER NOT NULL, media_type TEXT NOT NULL,
    original_url TEXT NOT NULL, s3_bucket TEXT NOT NULL, s3_key TEXT NOT NULL, public_url TEXT,
    content_type TEXT, bytes INTEGER DEFAULT 0, width INTEGER, height INTEGER, sha256 TEXT,
    fetched_at TEXT NOT NULL, error TEXT, UNIQUE(item_id, original_url)
);
"""


def zpack(text: str) -> bytes:
    return zstandard.ZstdCompressor(level=3).compress(text.encode("utf-8"))


def iso(days_ago: float = 0, hours_ago: float = 0) -> str:
    return (NOW - timedelta(days=days_ago, hours=hours_ago)).isoformat()


class Env:
    """一套临时文件加一个跑在随机端口上的服务。"""

    def __init__(self, fts: bool = True, old_schema: bool = False, db_dir: str | None = None):
        self.tmp = tempfile.TemporaryDirectory()
        base = db_dir or self.tmp.name
        os.makedirs(base, exist_ok=True)
        self.news_path = os.path.join(base, "ai_intel.db")
        self.jsonl = os.path.join(self.tmp.name, "tweets.jsonl")
        self.tweets_path = os.path.join(self.tmp.name, "tweets.db")
        self.fts = fts
        conn = sqlite3.connect(self.news_path)
        conn.execute("PRAGMA journal_mode = WAL")
        conn.execute(OLD_ITEMS_DDL if old_schema else ITEMS_DDL)
        conn.executescript(CONTENT_DDL)
        if fts:
            conn.execute(FTS_DDL)
        conn.commit()
        self.conn = conn
        self.server = None

    def add(self, title="t", **kw) -> int:
        row = {
            "title": title, "url": "https://example.com/" + title.replace(" ", "-"), "source": "src",
            "category": "ai", "published_at": iso(), "fetched_at": iso(), "summary": None,
            "pushed": 0, "title_cn": None, "summary_cn": None, "tags": None, "importance": None,
        }
        row.update(kw)
        if isinstance(row["tags"], list):
            row["tags"] = json.dumps(row["tags"], ensure_ascii=False, separators=(",", ":"))
        row["dedupe_key"] = f"{title}-{row['url']}-{self.conn.execute('SELECT COUNT(*) FROM items').fetchone()[0]}"
        cols = [r[1] for r in self.conn.execute("PRAGMA table_info(items)")]
        row = {k: v for k, v in row.items() if k in cols or k == "dedupe_key"}
        names = ", ".join(row)
        cur = self.conn.execute(f"INSERT INTO items ({names}) VALUES ({', '.join(':' + k for k in row)})", row)
        item_id = cur.lastrowid
        if self.fts:
            full = {k: row.get(k) or "" for k in ("title", "title_cn", "summary", "summary_cn", "tags", "source", "category", "url")}
            self.conn.execute(
                "INSERT INTO items_fts (rowid, item_id, title, title_cn, summary, summary_cn, tags, source, category, url) "
                "VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)",
                (item_id, item_id, full["title"], full["title_cn"], full["summary"], full["summary_cn"],
                 full["tags"], full["source"], full["category"], full["url"]),
            )
        self.conn.commit()
        return item_id

    def content(self, item_id, status="ok", md=None, md_cn=None):
        self.conn.execute(
            "INSERT INTO article_contents (item_id, url, fetched_at, status, markdown_zstd, content_cn_zstd) VALUES (?, 'u', ?, ?, ?, ?)",
            (item_id, iso(), status, md, md_cn),
        )
        self.conn.commit()

    def media(self, item_id, url, kind="image", width=None, height=None, error=None):
        self.conn.execute(
            "INSERT INTO article_media (item_id, media_type, original_url, s3_bucket, s3_key, public_url, width, height, fetched_at, error) "
            "VALUES (?, ?, ?, 'b', 'k', ?, ?, ?, ?, ?)",
            (item_id, kind, "orig-" + str(url), url, width, height, iso(), error),
        )
        self.conn.commit()

    def tweet_lines(self, *tweets, raw: bytes | None = None):
        with open(self.jsonl, "ab") as f:
            for t in tweets:
                f.write((json.dumps(t, ensure_ascii=False) + "\n").encode("utf-8"))
            if raw:
                f.write(raw)

    def start(self):
        service = feeds_api.Service(TOKEN, self.news_path, self.tweets_path, self.jsonl, now=lambda: NOW)
        self.server = feeds_api.Server(("127.0.0.1", 0), service)
        threading.Thread(target=self.server.serve_forever, kwargs={"poll_interval": 0.01}, daemon=True).start()
        self.port = self.server.server_address[1]
        return self

    def get(self, path, token=TOKEN, method="GET", headers=None):
        req = urllib.request.Request(f"http://127.0.0.1:{self.port}{path}", method=method)
        if token is not None:
            req.add_header("Authorization", "Bearer " + token)
        for k, v in (headers or {}).items():
            req.add_header(k, v)
        try:
            with urllib.request.urlopen(req, timeout=10) as r:
                return r.status, json.loads(r.read().decode("utf-8")), r.headers
        except urllib.error.HTTPError as e:
            body = e.read().decode("utf-8")
            return e.code, json.loads(body) if body else None, e.headers

    def ok(self, path):
        status, body, _ = self.get(path)
        assert status == 200, (path, status, body)
        return body

    def close(self):
        if self.server:
            self.server.shutdown()
            self.server.server_close()
        self.conn.close()
        self.tmp.cleanup()


class Base(unittest.TestCase):
    FTS = True

    def setUp(self):
        self.env = Env(fts=self.FTS)
        self.addCleanup(self.env.close)

    def titles(self, path):
        return [i["title"] for i in self.env.ok(path)["items"]]


class AuthTest(Base):
    def setUp(self):
        super().setUp()
        self.env.start()

    def test_token_required_everywhere(self):
        for path in ("/v1/health", "/v1/news", "/v1/news/facets", "/v1/news/1", "/v1/tweets", "/nope"):
            for token in (None, "", "wrong", TOKEN + "x", TOKEN[:-1]):
                status, body, headers = self.env.get(path, token=token)
                self.assertEqual(status, 401, (path, token))
                self.assertEqual(body, {"error": "unauthorized"})
                self.assertEqual(headers["WWW-Authenticate"], "Bearer")

    def test_other_schemes_rejected(self):
        status, _, _ = self.env.get("/v1/health", token=None, headers={"Authorization": "Basic " + TOKEN})
        self.assertEqual(status, 401)
        status, _, _ = self.env.get("/v1/health", token=None, headers={"Authorization": TOKEN})
        self.assertEqual(status, 401)

    def test_non_ascii_token_header_does_not_crash(self):
        status, _, _ = self.env.get("/v1/health", token=None, headers={"Authorization": "Bearer café"})
        self.assertEqual(status, 401)
        status, _, _ = self.env.get("/v1/health", token=TOKEN)
        self.assertEqual(status, 200)

    def test_uses_constant_time_compare(self):
        with mock.patch.object(feeds_api.hmac, "compare_digest", wraps=feeds_api.hmac.compare_digest) as m:
            self.env.get("/v1/health", token="wrong")
            self.env.get("/v1/health", token=TOKEN)
        self.assertEqual(m.call_count, 2)

    def test_only_get(self):
        for method in ("POST", "PUT", "DELETE", "PATCH"):
            req = urllib.request.Request(f"http://127.0.0.1:{self.env.port}/v1/news", method=method, data=b"{}")
            req.add_header("Authorization", "Bearer " + TOKEN)
            with self.assertRaises(urllib.error.HTTPError) as cm:
                urllib.request.urlopen(req, timeout=5)
            self.assertEqual(cm.exception.code, 405, method)

    def test_unknown_paths(self):
        for path in ("/", "/v1", "/v1/news/", "/v1/news/abc", "/v1/news/1/2", "/v1/news/-1", "/v1/tweets/1",
                     "/v1/news/1%2f..%2fhealth", "/v1/news/../health", "/v1/health/"):
            status, _, _ = self.env.get(path)
            self.assertEqual(status, 404, path)

    def test_listens_on_loopback_only(self):
        self.assertEqual(self.env.server.server_address[0], "127.0.0.1")

    def test_listen_argument(self):
        self.assertEqual(feeds_api.parse_listen("8790"), ("127.0.0.1", 8790))
        self.assertEqual(feeds_api.parse_listen("127.0.0.1:9000"), ("127.0.0.1", 9000))
        for bad in ("0.0.0.0:8790", "192.168.1.2:8790", ":", "127.0.0.1:x", "127.0.0.1:70000", "::1:80"):
            with self.assertRaises(Exception, msg=bad):
                feeds_api.parse_listen(bad)

    def test_refuses_to_start_without_token(self):
        with self.assertRaises(ValueError):
            feeds_api.Service("", "a", "b", "c")
        with mock.patch.dict(os.environ, {"XC_FEEDS_TOKEN": ""}), mock.patch("sys.stderr"):
            self.assertEqual(feeds_api.main(["--listen", "0"]), 2)


class SearchTests:
    """有 FTS5 和没有 FTS5 时结果必须一样，所以两种库跑同一组用例。"""

    def test_keyword_matches_any_text_field(self):
        e = self.env
        e.add("alpha release")
        e.add("beta", title_cn="阿尔法发布")
        e.add("gamma", summary="mentions alpha inside")
        e.add("delta", tags=["alpha-tag"], source="hn")
        e.add("epsilon", category="robotics")
        e.start()
        self.assertEqual(sorted(self.titles("/v1/news?q=alpha")), ["alpha release", "delta", "gamma"])
        self.assertEqual(self.titles("/v1/news?q=robotics"), ["epsilon"])
        self.assertEqual(self.titles("/v1/news?q=nothing-here"), [])

    def test_all_terms_must_match(self):
        e = self.env
        e.add("one two")
        e.add("one")
        e.add("two three")
        e.start()
        self.assertEqual(self.titles("/v1/news?q=one%20two"), ["one two"])
        self.assertEqual(self.titles("/v1/news?q=%20one%20%20%20two%20"), ["one two"])

    def test_chinese_keyword_in_the_middle_of_a_word(self):
        e = self.env
        e.add("model", title_cn="大模型发布", summary_cn="新一代语言模型")
        e.add("other", title_cn="机器人")
        e.start()
        self.assertEqual(self.titles("/v1/news?q=" + urllib.request.quote("模型")), ["model"])
        self.assertEqual(self.titles("/v1/news?q=" + urllib.request.quote("语言 模型")), ["model"])
        self.assertEqual(self.titles("/v1/news?q=model%20" + urllib.request.quote("模型")), ["model"])

    def test_like_wildcards_are_literal(self):
        e = self.env
        e.add("100% sure")
        e.add("1000 sure")
        e.add("a_b")
        e.add("aXb")
        e.start()
        self.assertEqual(self.titles("/v1/news?q=100%25"), ["100% sure"])
        self.assertEqual(self.titles("/v1/news?q=a_b"), ["a_b"])
        self.assertEqual(self.titles("/v1/news?q=%25"), ["100% sure"])
        self.assertEqual(self.titles("/v1/news?q=1_0"), [])
        self.assertEqual(self.titles("/v1/news?q=%5C"), [])

    def test_special_characters_do_not_change_the_query(self):
        e = self.env
        e.add("alpha beta")
        e.add("alpha only", summary="x")
        e.add("beta only")
        e.add("title alpha")
        e.start()
        evil = [
            '"', '""', '"alpha', 'alpha"', "'", "*", "alpha*", "^alpha", "-alpha", "+alpha", "(", ")", "(alpha",
            "alpha OR beta", "alpha AND beta", "NOT alpha", "alpha NOT beta", "NEAR(alpha beta)", "NEAR/2",
            "title:alpha", "{title}:alpha", "summary:x", "title : alpha", "alpha:", ":", "\\", "%", "_", ";", "--",
            "'; DROP TABLE items; --", "alpha) OR (beta", "\x00alpha", "a\nb", "alpha\tbeta", "　",
        ]
        for q in evil:
            path = "/v1/news?q=" + urllib.request.quote(q)
            status, body, _ = self.env.get(path)
            self.assertEqual(status, 200, q)
            self.assertIsInstance(body["items"], list)
        # 这些词只是普通文字，不是运算符：每个词都要命中，所以没有结果
        for q in ("alpha OR beta", "alpha AND beta", "alpha NOT beta", "NEAR(alpha beta)"):
            self.assertEqual(self.titles("/v1/news?q=" + urllib.request.quote(q)), [], q)
        # 列名: 不是列过滤。title:alpha 里有冒号，不会只查标题
        self.assertEqual(self.titles("/v1/news?q=" + urllib.request.quote("summary:x")), [])
        # 引号里再带双引号也安全
        self.assertEqual(self.titles("/v1/news?q=" + urllib.request.quote('"alpha')), [])
        # 表还在
        self.assertEqual(len(self.env.ok("/v1/news")["items"]), 4)

    def test_search_combines_with_filters_and_paging(self):
        e = self.env
        for i in range(7):
            e.add(f"match {i}", category="ai" if i % 2 == 0 else "dev")
        e.add("other")
        e.start()
        page = self.env.ok("/v1/news?q=match&limit=3")
        self.assertEqual([i["title"] for i in page["items"]], ["match 6", "match 5", "match 4"])
        page = self.env.ok("/v1/news?q=match&limit=3&before=" + page["next"])
        self.assertEqual([i["title"] for i in page["items"]], ["match 3", "match 2", "match 1"])
        self.assertEqual(self.titles("/v1/news?q=match&category=dev"), ["match 5", "match 3", "match 1"])

    def test_tweets_search(self):
        e = self.env
        e.tweet_lines(
            {"id": "100", "author": "Alice @alice", "time": "2026-10-17T01:00:00.000Z", "url": "u1", "text": "hello world"},
            {"id": "101", "author": "Bob @bob", "time": "2026-10-17T02:00:00.000Z", "url": "u2", "text": "你好，大模型来了\n[引用推文]\nquoted"},
            {"id": "102", "author": "Alice @alice", "time": "2026-10-17T03:00:00.000Z", "url": "u3", "text": "100% done_ok"},
        )
        e.start()
        ids = lambda p: [t["id"] for t in e.ok(p)["items"]]
        self.assertEqual(ids("/v1/tweets?q=hello"), ["100"])
        self.assertEqual(ids("/v1/tweets?q=quoted"), ["101"])
        self.assertEqual(ids("/v1/tweets?q=" + urllib.request.quote("模型")), ["101"])
        self.assertEqual(ids("/v1/tweets?q=100%25"), ["102"])
        self.assertEqual(ids("/v1/tweets?q=done_ok"), ["102"])
        self.assertEqual(ids("/v1/tweets?q=hello%20world"), ["100"])
        self.assertEqual(ids("/v1/tweets?q=hello%20bob"), [])
        self.assertEqual(ids("/v1/tweets?q=" + urllib.request.quote('author:alice "OR* NEAR(')), [])
        # 作者名和账号都能搜到
        self.assertEqual(ids("/v1/tweets?q=bob"), ["101"])
        self.assertEqual(ids("/v1/tweets?author=alice"), ["102", "100"])
        self.assertEqual(ids("/v1/tweets?author=%40BOB"), ["101"])
        self.assertEqual(ids("/v1/tweets?author=alice&q=hello"), ["100"])


class SearchWithFTS(SearchTests, Base):
    FTS = True

    def test_uses_fts_for_plain_words(self):
        self.env.add("alpha")
        self.env.start()
        conn = self.env.server.service.news.connect()
        self.assertIn("items_fts", self.env.server.service.news.tables(conn))
        conn.close()
        # 词的前缀也能搜到（FTS 里的前缀匹配）
        self.assertEqual(self.titles("/v1/news?q=alp"), ["alpha"])

    def test_broken_fts_table_falls_back_to_like(self):
        e = self.env
        e.add("alpha")
        e.conn.execute("DROP TABLE items_fts")
        e.conn.execute("CREATE TABLE items_fts (rowid_x INTEGER, title TEXT)")  # 不是虚拟表，MATCH 会报错
        e.conn.commit()
        e.start()
        with self.assertLogs("x-console-feeds", level="WARNING"):
            self.assertEqual(self.titles("/v1/news?q=alpha"), ["alpha"])


class SearchWithoutFTS(SearchTests, Base):
    FTS = False

    def test_no_fts_table(self):
        self.env.start()
        conn = self.env.server.service.news.connect()
        self.assertNotIn("items_fts", self.env.server.service.news.tables(conn))
        conn.close()


class NewsListTest(Base):
    def test_paging_newest_first_without_duplicates(self):
        e = self.env
        for i in range(120):
            e.add(f"n{i:03d}", fetched_at=iso(hours_ago=200 - i))
        e.start()
        seen, cursor, pages = [], None, 0
        while True:
            body = e.ok("/v1/news" + (f"?before={cursor}" if cursor else ""))
            pages += 1
            seen += [i["id"] for i in body["items"]]
            cursor = body["next"]
            if not cursor:
                break
            self.assertEqual(len(body["items"]), 50)
        self.assertEqual(pages, 3)
        self.assertEqual(len(seen), 120)
        self.assertEqual(seen, sorted(set(seen), reverse=True))
        self.assertEqual(seen[0], 120)

    def test_exact_page_has_no_next(self):
        for i in range(50):
            self.env.add(f"n{i}")
        self.env.start()
        body = self.env.ok("/v1/news")
        self.assertEqual(len(body["items"]), 50)
        self.assertIsNone(body["next"])
        self.assertEqual(self.env.ok("/v1/news?limit=10")["next"], str(body["items"][9]["id"]))

    def test_new_rows_do_not_shift_the_next_page(self):
        for i in range(10):
            self.env.add(f"n{i}")
        self.env.start()
        first = self.env.ok("/v1/news?limit=4")
        for i in range(3):
            self.env.add(f"late{i}")
        second = self.env.ok("/v1/news?limit=4&before=" + first["next"])
        self.assertEqual([i["title"] for i in second["items"]], ["n5", "n4", "n3", "n2"])

    def test_limit(self):
        for i in range(130):
            self.env.add(f"n{i}")
        self.env.start()
        self.assertEqual(len(self.env.ok("/v1/news?limit=7")["items"]), 7)
        self.assertEqual(len(self.env.ok("/v1/news?limit=100")["items"]), 100)
        self.assertEqual(len(self.env.ok("/v1/news?limit=5000")["items"]), 100)  # 超过上限按上限
        self.assertEqual(len(self.env.ok("/v1/news?limit=")["items"]), 50)
        for bad in ("0", "-1", "abc", "1.5"):
            status, body, _ = self.env.get("/v1/news?limit=" + bad)
            self.assertEqual(status, 400, bad)
            self.assertIn("error", body)

    def test_bad_parameters(self):
        self.env.start()
        for q in ("days=0", "days=x", "days=99999", "minImportance=0", "minImportance=6", "pushed=maybe",
                  "before=abc", "before=-1", "before=1e3", "before=99999999999999999999", "q=" + "a" * 201,
                  "tag=" + "a" * 101, "category=" + "a" * 101):
            status, _, _ = self.env.get("/v1/news?" + q)
            self.assertEqual(status, 400, q)

    def test_unknown_parameters_are_ignored(self):
        self.env.add("x")
        self.env.start()
        self.assertEqual(self.titles("/v1/news?evil=1&db=/etc/passwd&sql=1;drop"), ["x"])

    def test_item_fields(self):
        e = self.env
        long = "字" * 5000
        item = e.add("title", title_cn="标题", summary=long, summary_cn="摘要", tags=["ai", "模型"], importance=5,
                     pushed=1, published_at="2026-10-16T01:02:03+00:00", fetched_at="2026-10-17T01:02:03+00:00")
        bare = e.add("bare", pushed=None, category=None)
        e.start()
        items = {i["id"]: i for i in e.ok("/v1/news")["items"]}
        self.assertEqual(items[item], {
            "id": item, "title": "title", "titleCn": "标题", "summary": "字" * feeds_api.LIST_SUMMARY_CHARS,
            "summaryCn": "摘要", "url": "https://example.com/title", "source": "src", "category": "ai",
            "tags": ["ai", "模型"], "importance": 5, "pushed": True,
            "publishedAt": "2026-10-16T01:02:03+00:00", "fetchedAt": "2026-10-17T01:02:03+00:00",
        })
        b = items[bare]
        self.assertEqual((b["titleCn"], b["summary"], b["summaryCn"], b["importance"], b["pushed"], b["tags"], b["category"]),
                         (None, None, None, None, False, [], ""))

    def test_bad_tags_text_does_not_break_the_list(self):
        self.env.add("a", tags="ai, agents")
        self.env.add("b", tags="[broken")
        self.env.add("c", tags='{"not": "a list"}')
        self.env.start()
        by = {i["title"]: i["tags"] for i in self.env.ok("/v1/news")["items"]}
        self.assertEqual(by["a"], ["ai", "agents"])
        self.assertEqual(by["c"], [])

    def test_old_database_without_ai_columns(self):
        self.env.close()
        self.env = Env(fts=False, old_schema=True)
        self.addCleanup(self.env.close)
        self.env.add("old", summary="s")
        self.env.start()
        item = self.env.ok("/v1/news")["items"][0]
        self.assertEqual((item["title"], item["titleCn"], item["importance"], item["pushed"], item["tags"]),
                         ("old", None, None, False, []))
        self.assertEqual(self.titles("/v1/news?minImportance=3&pushed=true&tag=x"), [])
        self.assertEqual(self.env.ok("/v1/news/facets")["tags"], [])

    def test_database_path_with_special_characters(self):
        self.env.close()
        d = tempfile.TemporaryDirectory()
        self.addCleanup(d.cleanup)
        self.env = Env(db_dir=os.path.join(d.name, "my news #1 ?x%20y"))
        self.addCleanup(self.env.close)
        self.env.add("ok")
        self.env.start()
        self.assertEqual(self.titles("/v1/news"), ["ok"])

    def test_database_is_opened_read_only(self):
        self.env.add("x")
        self.env.start()
        conn = self.env.server.service.news.connect()
        with self.assertRaises(sqlite3.OperationalError):
            conn.execute("DELETE FROM items")
        conn.close()
        self.assertEqual(self.titles("/v1/news"), ["x"])

    def test_missing_database_is_a_503_not_a_crash(self):
        self.env.start()
        os.remove(self.env.news_path)
        for suffix in ("-wal", "-shm"):
            if os.path.exists(self.env.news_path + suffix):
                os.remove(self.env.news_path + suffix)
        status, body, _ = self.env.get("/v1/news")
        self.assertEqual(status, 503)
        self.assertEqual(body, {"error": "database unavailable"})
        self.assertEqual(self.env.get("/v1/health")[0], 503)


class FilterTest(Base):
    def setUp(self):
        super().setUp()
        e = self.env
        e.add("fresh-ai", tags=["ai", "agent"], category="ai", importance=5, pushed=1, fetched_at=iso(hours_ago=2))
        e.add("fresh-dev", tags=["rust"], category="dev", importance=3, pushed=0, fetched_at=iso(hours_ago=5))
        e.add("week-ai", tags=["ai"], category="ai", importance=4, pushed=1, fetched_at=iso(days_ago=5))
        e.add("month-dev", tags=["train", "ai-agent"], category="dev", importance=None, pushed=None, fetched_at=iso(days_ago=20))
        e.add("old", tags=["ai"], category="ai", importance=2, pushed=0, fetched_at=iso(days_ago=100))
        e.start()

    def test_tag_is_exact(self):
        self.assertEqual(self.titles("/v1/news?tag=ai"), ["old", "week-ai", "fresh-ai"])
        self.assertEqual(self.titles("/v1/news?tag=agent"), ["fresh-ai"])
        self.assertEqual(self.titles("/v1/news?tag=ai-agent"), ["month-dev"])
        self.assertEqual(self.titles("/v1/news?tag=a"), [])
        self.assertEqual(self.titles("/v1/news?tag=%25"), [])
        self.assertEqual(self.titles("/v1/news?tag=%22"), [])

    def test_chinese_tag(self):
        self.env.add("zh", tags=["大模型", "开源"])
        self.assertEqual(self.titles("/v1/news?tag=" + urllib.request.quote("大模型")), ["zh"])

    def test_category(self):
        self.assertEqual(self.titles("/v1/news?category=dev"), ["month-dev", "fresh-dev"])
        self.assertEqual(self.titles("/v1/news?category=AI"), [])  # 分类按原样比较

    def test_days(self):
        self.assertEqual(self.titles("/v1/news?days=1"), ["fresh-dev", "fresh-ai"])
        self.assertEqual(self.titles("/v1/news?days=7"), ["week-ai", "fresh-dev", "fresh-ai"])
        self.assertEqual(self.titles("/v1/news?days=30"), ["month-dev", "week-ai", "fresh-dev", "fresh-ai"])
        self.assertEqual(len(self.titles("/v1/news")), 5)

    def test_min_importance(self):
        self.assertEqual(self.titles("/v1/news?minImportance=4"), ["week-ai", "fresh-ai"])
        self.assertEqual(self.titles("/v1/news?minImportance=5"), ["fresh-ai"])
        self.assertEqual(len(self.titles("/v1/news?minImportance=1")), 4)  # 没评分的不算

    def test_pushed(self):
        self.assertEqual(self.titles("/v1/news?pushed=true"), ["week-ai", "fresh-ai"])
        self.assertEqual(self.titles("/v1/news?pushed=1"), ["week-ai", "fresh-ai"])
        self.assertEqual(self.titles("/v1/news?pushed=false"), ["old", "month-dev", "fresh-dev"])
        self.assertEqual(self.titles("/v1/news?pushed=0"), ["old", "month-dev", "fresh-dev"])

    def test_filters_combine(self):
        self.assertEqual(self.titles("/v1/news?category=ai&minImportance=4&pushed=true&days=7&tag=ai"), ["week-ai", "fresh-ai"])
        self.assertEqual(self.titles("/v1/news?category=dev&pushed=true"), [])
        self.assertEqual(self.titles("/v1/news?days=30&tag=ai&limit=1"), ["week-ai"])

    def test_filters_with_cursor(self):
        body = self.env.ok("/v1/news?tag=ai&limit=2")
        self.assertEqual([i["title"] for i in body["items"]], ["old", "week-ai"])
        self.assertEqual(body["next"], str(body["items"][-1]["id"]))
        self.assertEqual(self.titles("/v1/news?tag=ai&limit=2&before=" + body["next"]), ["fresh-ai"])

    def test_facets(self):
        body = self.env.ok("/v1/news/facets")
        self.assertEqual(body["categories"], ["ai", "dev"])
        # 只算最近 30 天；同样多的按名字排
        self.assertEqual(body["tags"], [
            {"tag": "ai", "count": 2}, {"tag": "agent", "count": 1}, {"tag": "ai-agent", "count": 1},
            {"tag": "rust", "count": 1}, {"tag": "train", "count": 1},
        ])

    def test_facets_keep_the_top_fifty(self):
        for i in range(60):
            self.env.add(f"bulk{i}", tags=[f"t{i:02d}"] + (["common"] if i < 10 else []))
        body = self.env.ok("/v1/news/facets")
        self.assertEqual(len(body["tags"]), 50)
        self.assertEqual(body["tags"][0]["tag"], "common")
        self.assertEqual(body["tags"][0]["count"], 10)


class DetailTest(Base):
    def test_not_found(self):
        self.env.start()
        status, body, _ = self.env.get("/v1/news/12345")
        self.assertEqual(status, 404)
        self.assertIn("error", body)

    def test_content_not_fetched_yet(self):
        item = self.env.add("fresh", summary="s", tags=["a"])
        self.env.start()
        body = self.env.ok(f"/v1/news/{item}")
        self.assertEqual(body["title"], "fresh")
        self.assertEqual(body["tags"], ["a"])
        self.assertEqual(body["content"], {"status": "missing", "markdown": None, "markdownCn": None})
        self.assertEqual(body["media"], [])

    def test_content_row_without_text(self):
        item = self.env.add("failed")
        self.env.content(item, status="error")
        self.env.start()
        content = self.env.ok(f"/v1/news/{item}")["content"]
        self.assertEqual(content, {"status": "error", "markdown": None, "markdownCn": None})

    def test_without_content_tables(self):
        self.env.conn.executescript("DROP TABLE article_contents; DROP TABLE article_media;")
        item = self.env.add("x")
        self.env.start()
        body = self.env.ok(f"/v1/news/{item}")
        self.assertEqual((body["content"]["status"], body["media"]), ("missing", []))

    def test_without_zstandard_library(self):
        item = self.env.add("x")
        self.env.content(item, md=b"not really zstd")
        self.env.start()
        with mock.patch.object(feeds_api, "zstandard", None), self.assertLogs("x-console-feeds", level="WARNING"):
            content = self.env.ok(f"/v1/news/{item}")["content"]
        self.assertEqual(content, {"status": "ok", "markdown": None, "markdownCn": None})

    def test_corrupt_blob_does_not_break_the_page(self):
        if zstandard is None:
            self.skipTest("没装 zstandard")
        item = self.env.add("x")
        self.env.content(item, md=b"\x28\xb5\x2f\xfdgarbage", md_cn=zpack("译文"))
        self.env.start()
        with self.assertLogs("x-console-feeds", level="WARNING"):
            content = self.env.ok(f"/v1/news/{item}")["content"]
        self.assertIsNone(content["markdown"])
        self.assertEqual(content["markdownCn"], "译文")

    def test_media_only_valid_public_urls(self):
        item = self.env.add("x")
        self.env.media(item, "https://s3.example.com/a.png", width=640, height=480)
        self.env.media(item, "https://s3.example.com/b.mp4", kind="video")
        self.env.media(item, "https://s3.example.com/bad.png", error="upload failed")
        self.env.conn.execute(
            "INSERT INTO article_media (item_id, media_type, original_url, s3_bucket, s3_key, public_url, fetched_at) "
            "VALUES (?, 'image', 'o', 'b', '', NULL, ?)", (item, iso()))
        self.env.conn.commit()
        other = self.env.add("other")
        self.env.media(other, "https://s3.example.com/other.png")
        self.env.start()
        self.assertEqual(self.env.ok(f"/v1/news/{item}")["media"], [
            {"url": "https://s3.example.com/a.png", "type": "image", "width": 640, "height": 480},
            {"url": "https://s3.example.com/b.mp4", "type": "video", "width": None, "height": None},
        ])

    @unittest.skipUnless(zstandard, "没装 zstandard")
    def test_zstd_content_is_decoded(self):
        item = self.env.add("x", title_cn="标", summary="s", pushed=1, importance=4)
        md = "# Title\n\n正文 with **bold** and 中文。\n"
        self.env.content(item, status="ok", md=zpack(md), md_cn=zpack("# 标题\n\n译文"))
        self.env.start()
        body = self.env.ok(f"/v1/news/{item}")
        self.assertEqual(body["content"], {"status": "ok", "markdown": md, "markdownCn": "# 标题\n\n译文"})
        self.assertEqual((body["id"], body["titleCn"], body["pushed"], body["importance"]), (item, "标", True, 4))
        # 只有原文
        other = self.env.add("y")
        self.env.content(other, status="thin", md=zpack("short"))
        content = self.env.ok(f"/v1/news/{other}")["content"]
        self.assertEqual(content, {"status": "thin", "markdown": "short", "markdownCn": None})

    @unittest.skipUnless(zstandard, "没装 zstandard")
    def test_long_content_is_cut_and_marked(self):
        item = self.env.add("big")
        big = "a" * 1_000_000
        zh = "中" * 400_000  # 1.2 MB，按字节截断时可能切在一个字中间
        self.env.content(item, md=zpack(big), md_cn=zpack(zh))
        self.env.start()
        status, body, _ = self.env.get(f"/v1/news/{item}")
        self.assertEqual(status, 200)
        content = body["content"]
        self.assertTrue(content["truncated"])
        self.assertEqual(len(content["markdown"]), feeds_api.MAX_CONTENT_BYTES)
        self.assertLessEqual(len(content["markdownCn"].encode("utf-8")), feeds_api.MAX_CONTENT_BYTES)
        self.assertGreater(len(content["markdownCn"]), 260_000)
        self.assertEqual(content["markdownCn"], "中" * len(content["markdownCn"]))

    @unittest.skipUnless(zstandard, "没装 zstandard")
    def test_response_stays_under_two_megabytes(self):
        item = self.env.add("big", summary="s" * 100000)
        self.env.content(item, md=zpack("中" * 900_000), md_cn=zpack("文" * 900_000))
        self.env.start()
        req = urllib.request.Request(f"http://127.0.0.1:{self.env.port}/v1/news/{item}")
        req.add_header("Authorization", "Bearer " + TOKEN)
        with urllib.request.urlopen(req, timeout=10) as r:
            raw = r.read()
        self.assertLess(len(raw), feeds_api.MAX_RESPONSE_BYTES)
        self.assertTrue(json.loads(raw)["content"]["truncated"])

    @unittest.skipUnless(zstandard, "没装 zstandard")
    def test_exactly_at_the_limit_is_not_truncated(self):
        item = self.env.add("edge")
        self.env.content(item, md=zpack("b" * feeds_api.MAX_CONTENT_BYTES))
        self.env.start()
        content = self.env.ok(f"/v1/news/{item}")["content"]
        self.assertNotIn("truncated", content)
        self.assertEqual(len(content["markdown"]), feeds_api.MAX_CONTENT_BYTES)


class TweetsTest(Base):
    def tweet(self, n, author="Alice @alice", text=None, **kw):
        t = {"id": str(n), "author": author, "time": f"2026-10-17T{n % 24:02d}:00:00.000Z",
             "url": f"https://x.com/i/web/status/{n}", "text": text if text is not None else f"tweet {n}",
             "collectedAt": "2026-10-17T10:00:00.000Z"}
        t.update(kw)
        return t

    def test_no_jsonl_yet(self):
        self.env.start()
        self.assertEqual(self.env.ok("/v1/tweets"), {"items": [], "next": None})
        self.assertEqual(self.env.ok("/v1/health")["tweets"], {"count": 0, "lastAt": None})

    def test_import_and_fields(self):
        self.env.tweet_lines(self.tweet(5, text="line1\nline2\n[引用推文]\nquoted"))
        self.env.start()
        self.assertEqual(self.env.ok("/v1/tweets")["items"], [{
            "id": "5", "author": "Alice @alice", "time": "2026-10-17T05:00:00.000Z",
            "url": "https://x.com/i/web/status/5", "text": "line1\nline2\n[引用推文]\nquoted",
            "collectedAt": "2026-10-17T10:00:00.000Z",
        }])

    def test_incremental_import_and_dedupe(self):
        e = self.env
        e.tweet_lines(self.tweet(1), self.tweet(2), self.tweet(3))
        e.start()
        self.assertEqual(len(e.ok("/v1/tweets")["items"]), 3)
        offset = os.path.getsize(e.jsonl)
        conn = sqlite3.connect(e.tweets_path)
        self.assertEqual(conn.execute("SELECT value FROM meta WHERE key = 'offset'").fetchone()[0], str(offset))
        # 新追加的行被读到，重复的编号被跳过（保留第一次的内容）
        e.tweet_lines(self.tweet(3, text="changed"), self.tweet(4), self.tweet(2, text="again"))
        items = e.ok("/v1/tweets")["items"]
        self.assertEqual([t["id"] for t in items], ["4", "3", "2", "1"])
        self.assertEqual({t["id"]: t["text"] for t in items}["3"], "tweet 3")
        self.assertEqual(conn.execute("SELECT COUNT(*) FROM tweets").fetchone()[0], 4)
        if self.FTS:
            self.assertEqual(conn.execute("SELECT COUNT(*) FROM tweets_fts").fetchone()[0], 4)
        conn.close()

    def test_only_reads_new_lines(self):
        e = self.env
        e.tweet_lines(self.tweet(1), self.tweet(2))
        e.start()
        e.ok("/v1/tweets")
        with mock.patch.object(feeds_api.TweetsDB, "parse_line", wraps=feeds_api.TweetsDB.parse_line) as parse:
            e.ok("/v1/tweets")
            self.assertEqual(parse.call_count, 0)
            e.tweet_lines(self.tweet(3))
            e.ok("/v1/tweets")
            self.assertEqual(parse.call_count, 1)

    def test_offset_survives_restart(self):
        e = self.env
        e.tweet_lines(self.tweet(1))
        e.start()
        e.ok("/v1/tweets")
        e.server.shutdown()
        e.server.server_close()
        e.tweet_lines(self.tweet(2))
        e.start()
        with mock.patch.object(feeds_api.TweetsDB, "parse_line", wraps=feeds_api.TweetsDB.parse_line) as parse:
            self.assertEqual([t["id"] for t in e.ok("/v1/tweets")["items"]], ["2", "1"])
            self.assertEqual(parse.call_count, 1)

    def test_half_written_last_line_waits(self):
        e = self.env
        e.tweet_lines(self.tweet(1))
        line = json.dumps(self.tweet(2))
        e.tweet_lines(raw=line[:30].encode())
        e.start()
        self.assertEqual([t["id"] for t in e.ok("/v1/tweets")["items"]], ["1"])
        with open(e.jsonl, "ab") as f:
            f.write((line[30:] + "\n").encode())
        self.assertEqual([t["id"] for t in e.ok("/v1/tweets")["items"]], ["2", "1"])

    def test_bad_lines_are_skipped_and_logged(self):
        e = self.env
        e.tweet_lines(self.tweet(1))
        e.tweet_lines(raw=b"this is not json\n")
        e.tweet_lines(raw=b"\n")
        e.tweet_lines(raw=b'["a list"]\n')
        e.tweet_lines(raw=b'{"author": "no id"}\n')
        e.tweet_lines(raw=b'{"id": "abc"}\n')
        e.tweet_lines(raw=b'{"id": "99999999999999999999999"}\n')
        e.tweet_lines(raw=b'{"id": true}\n')
        e.tweet_lines(raw=b'\xff\xfe\xfa binary\n')
        e.tweet_lines(raw=b'{"id": "7", "text": "lone \\ud800 surrogate"}\n')
        e.tweet_lines(raw=b'{"id": 8, "text": 123, "author": null}\n')
        e.tweet_lines(self.tweet(2))
        e.start()
        with self.assertLogs("x-console-feeds", level="WARNING") as logs:
            items = e.ok("/v1/tweets")["items"]
        # 坏行：不是 JSON、不是对象、没有 id、id 不是数字、id 太大、id 是布尔、不是 UTF-8、孤立代理字符
        self.assertEqual(len(logs.records), 8)
        self.assertEqual([t["id"] for t in items], ["8", "2", "1"])
        self.assertEqual((items[0]["text"], items[0]["author"]), ("", ""))

    def test_file_replaced_by_a_smaller_one(self):
        e = self.env
        e.tweet_lines(self.tweet(1), self.tweet(2), self.tweet(3))
        e.start()
        self.assertEqual(len(e.ok("/v1/tweets")["items"]), 3)
        with open(e.jsonl, "wb") as f:
            f.write((json.dumps(self.tweet(9)) + "\n").encode())
        with self.assertLogs("x-console-feeds", level="WARNING"):
            items = e.ok("/v1/tweets")["items"]
        self.assertEqual([t["id"] for t in items], ["9", "3", "2", "1"])

    def test_order_is_numeric_not_alphabetical(self):
        e = self.env
        e.tweet_lines(self.tweet(999), self.tweet(1000), self.tweet(1800000000000000001), self.tweet(2))
        e.start()
        self.assertEqual([t["id"] for t in e.ok("/v1/tweets")["items"]], ["1800000000000000001", "1000", "999", "2"])

    def test_paging(self):
        e = self.env
        e.tweet_lines(*[self.tweet(1000 + i) for i in range(120)])
        e.start()
        seen, cursor, pages = [], None, 0
        while True:
            body = e.ok("/v1/tweets" + (f"?before={cursor}" if cursor else ""))
            pages += 1
            seen += [t["id"] for t in body["items"]]
            cursor = body["next"]
            if not cursor:
                break
        self.assertEqual(pages, 3)
        self.assertEqual(seen, [str(1119 - i) for i in range(120)])
        self.assertEqual(len(e.ok("/v1/tweets?limit=100")["items"]), 100)
        self.assertEqual(len(e.ok("/v1/tweets?limit=9999")["items"]), 100)
        for bad in ("limit=0", "limit=x", "before=abc", "before=-1", "before=1%27", "author=" + "a" * 101, "q=" + "a" * 201):
            self.assertEqual(e.get("/v1/tweets?" + bad)[0], 400, bad)

    def test_author_filter_paging(self):
        e = self.env
        e.tweet_lines(*[self.tweet(100 + i, author="Alice @alice" if i % 2 == 0 else "Bob @bob") for i in range(10)])
        e.start()
        body = e.ok("/v1/tweets?author=bob&limit=2")
        self.assertEqual([t["id"] for t in body["items"]], ["109", "107"])
        body = e.ok("/v1/tweets?author=bob&limit=2&before=" + body["next"])
        self.assertEqual([t["id"] for t in body["items"]], ["105", "103"])

    def test_health(self):
        e = self.env
        e.add("a", fetched_at="2026-10-16T01:00:00+00:00")
        e.add("b", fetched_at="2026-10-17T05:00:00+00:00")
        e.add("c", fetched_at="2026-10-15T01:00:00+00:00")
        e.tweet_lines(self.tweet(1, time="2026-10-17T01:00:00.000Z"), self.tweet(2, time="2026-10-17T09:30:00.000Z"))
        e.start()
        self.assertEqual(e.ok("/v1/health"), {
            "news": {"count": 3, "lastFetchedAt": "2026-10-17T05:00:00+00:00"},
            "tweets": {"count": 2, "lastAt": "2026-10-17T09:30:00.000Z"},
        })

    def test_import_failure_does_not_take_the_page_down(self):
        e = self.env
        e.tweet_lines(self.tweet(1))
        e.start()
        e.ok("/v1/tweets")
        e.tweet_lines(self.tweet(2))
        with mock.patch.object(feeds_api.TweetsDB, "_import", side_effect=sqlite3.OperationalError("locked")), \
                self.assertLogs("x-console-feeds", level="ERROR"):
            self.assertEqual([t["id"] for t in e.ok("/v1/tweets")["items"]], ["1"])
        self.assertEqual([t["id"] for t in e.ok("/v1/tweets")["items"]], ["2", "1"])


class TweetsWithoutFTS(TweetsTest):
    FTS = False

    def setUp(self):
        super().setUp()
        patcher = mock.patch.object(feeds_api, "TWEETS_FTS_DDL", "CREATE VIRTUAL TABLE tweets_fts USING no_such_module(x)")
        patcher.start()
        self.addCleanup(patcher.stop)

    def test_no_fts_table_is_created(self):
        self.env.tweet_lines(self.tweet(1))
        self.env.start()
        self.env.ok("/v1/tweets")
        conn = sqlite3.connect(self.env.tweets_path)
        self.assertIsNone(conn.execute("SELECT 1 FROM sqlite_master WHERE name = 'tweets_fts'").fetchone())
        conn.close()
        self.assertEqual(len(self.env.ok("/v1/tweets?q=tweet")["items"]), 1)


class TweetsFTSBackfill(Base):
    def test_index_is_filled_when_fts_appears_later(self):
        e = self.env
        e.tweet_lines({"id": "1", "author": "a", "text": "findme"}, {"id": "2", "author": "b", "text": "other"})
        with mock.patch.object(feeds_api, "TWEETS_FTS_DDL", "CREATE VIRTUAL TABLE tweets_fts USING no_such_module(x)"):
            e.start()
            e.ok("/v1/tweets")
        conn = sqlite3.connect(e.tweets_path)
        self.assertIsNone(conn.execute("SELECT 1 FROM sqlite_master WHERE name = 'tweets_fts'").fetchone())
        conn.close()
        self.assertEqual([t["id"] for t in e.ok("/v1/tweets?q=findme")["items"]], ["1"])
        conn = sqlite3.connect(e.tweets_path)
        self.assertEqual(conn.execute("SELECT COUNT(*) FROM tweets_fts").fetchone()[0], 2)
        conn.close()


class SearchHelpersTest(unittest.TestCase):
    def test_fts_phrase_escapes_quotes(self):
        self.assertEqual(feeds_api.fts_phrase('say "hi"'), '"say ""hi"""*')
        self.assertEqual(feeds_api.fts_phrase("a"), '"a"*')

    def test_every_word_is_one_quoted_literal(self):
        # OR、NEAR 这样的词在引号里只是普通文字
        where, args = feeds_api.search_clause("foo OR NEAR bar", ("title",), "MATCH ?")
        self.assertEqual(where, ["MATCH ?"])
        self.assertEqual(args, ['"foo"* "OR"* "NEAR"* "bar"*'])

    def test_words_with_punctuation_never_reach_fts(self):
        for term in ('"bar', 'a"b', "title:x", "gpt-5", "100%", "NEAR(a", "a*", "^a", "-a", "(", "--", "a_b"):
            where, args = feeds_api.search_clause(term, ("title",), "MATCH ?")
            self.assertNotIn("MATCH ?", where, term)
            self.assertEqual(args, [feeds_api.like_pattern(term)], term)

    def test_cjk_and_punctuation_use_like(self):
        where, args = feeds_api.search_clause("模型 - ok", ("title",), "MATCH ?")
        self.assertEqual(len(where), 3)
        self.assertEqual(args, ["%模型%", "%-%", '"ok"*'])
        self.assertEqual(where[-1], "MATCH ?")

    def test_like_escape(self):
        self.assertEqual(feeds_api.like_pattern("50%_\\"), "%50\\%\\_\\\\%")

    def test_at_most_eight_terms(self):
        self.assertEqual(len(feeds_api.split_terms(" ".join("w%d" % i for i in range(30)))), feeds_api.MAX_TERMS)

    def test_no_query_no_clause(self):
        self.assertEqual(feeds_api.search_clause(None, ("title",), "x"), ([], []))
        self.assertEqual(feeds_api.search_clause("", ("title",), "x"), ([], []))


if __name__ == "__main__":
    logging.disable(logging.NOTSET)
    unittest.main()
