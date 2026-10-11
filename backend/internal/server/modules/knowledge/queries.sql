-- name: GetKnowledgeDoc :one
SELECT * FROM knowledge_docs WHERE source = ? AND ref = ?;

-- name: ListKnowledgeDocs :many
SELECT * FROM knowledge_docs;

-- name: UpsertKnowledgeDoc :exec
INSERT INTO knowledge_docs (source, ref, module, title, link, hash, model, indexed_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT (source, ref) DO UPDATE SET module = excluded.module, title = excluded.title, link = excluded.link,
  hash = excluded.hash, model = excluded.model, indexed_at = excluded.indexed_at;

-- name: UpdateKnowledgeDocMeta :exec
UPDATE knowledge_docs SET module = ?, title = ?, link = ? WHERE source = ? AND ref = ?;

-- name: DeleteKnowledgeDoc :exec
DELETE FROM knowledge_docs WHERE source = ? AND ref = ?;

-- name: DeleteKnowledgeChunks :exec
DELETE FROM knowledge_chunks WHERE source = ? AND ref = ?;

-- name: InsertKnowledgeChunk :exec
INSERT INTO knowledge_chunks (source, ref, seq, text, embedding) VALUES (?, ?, ?, ?, ?);

-- name: ListKnowledgeChunks :many
SELECT c.id, c.source, c.ref, c.text, c.embedding, d.module, d.title, d.link, d.model
FROM knowledge_chunks c JOIN knowledge_docs d ON d.source = c.source AND d.ref = c.ref;

-- name: KnowledgeCounts :many
SELECT d.source, count(DISTINCT d.ref) AS docs, count(c.id) AS chunks
FROM knowledge_docs d LEFT JOIN knowledge_chunks c ON c.source = d.source AND c.ref = d.ref
GROUP BY d.source;

-- name: ClearKnowledgeDocs :exec
DELETE FROM knowledge_docs;
