-- +goose Up
-- B139: 个人资料问答的索引。
-- knowledge_docs 一行一个文档，hash 是内容的哈希，没变就不重新嵌入。
-- module 是文档所属的可隐藏模块，隐藏内容锁着时按它过滤。
-- knowledge_chunks 一行一段，embedding 是归一化后的 float32（小端）。
CREATE TABLE knowledge_docs (
 source TEXT NOT NULL,
 ref TEXT NOT NULL,
 module TEXT NOT NULL,
 title TEXT NOT NULL,
 link TEXT NOT NULL,
 hash TEXT NOT NULL,
 model TEXT NOT NULL,
 indexed_at DATETIME NOT NULL,
 PRIMARY KEY (source, ref)
);

CREATE TABLE knowledge_chunks (
 id INTEGER PRIMARY KEY AUTOINCREMENT,
 source TEXT NOT NULL,
 ref TEXT NOT NULL,
 seq INTEGER NOT NULL,
 text TEXT NOT NULL,
 embedding BLOB NOT NULL,
 FOREIGN KEY (source, ref) REFERENCES knowledge_docs(source, ref) ON DELETE CASCADE
);
CREATE INDEX knowledge_chunks_doc ON knowledge_chunks(source, ref, seq);

-- +goose Down
DROP TABLE knowledge_chunks;
DROP TABLE knowledge_docs;
