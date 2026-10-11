package knowledge

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strings"

	"github.com/j0x3n/x-console/backend/internal/server/contracts"
	"github.com/j0x3n/x-console/backend/internal/server/modules/knowledge/api"
	"github.com/j0x3n/x-console/backend/internal/server/modules/knowledge/db"
)

// sync brings the index up to date. Only one sync runs at a time. Errors are
// kept for the status, not returned: a provider that has no embedding
// endpoint must not make the scheduler fail every ten minutes.
func (m *Module) sync(ctx context.Context) {
	opts := m.options(ctx)
	if !opts.Enabled {
		return
	}
	m.mu.Lock()
	if m.syncing {
		m.mu.Unlock()
		return
	}
	m.syncing = true
	m.mu.Unlock()

	pending, err := m.syncOnce(ctx, opts)

	m.mu.Lock()
	m.syncing = false
	m.last = m.now().UTC()
	m.pending = pending
	m.lastErr = ""
	if err != nil {
		m.lastErr = err.Error()
		m.d.Log.Warn("knowledge sync", "err", err)
	}
	m.mu.Unlock()
	m.invalidate()
}

func docKey(source, ref string) string { return source + "\x00" + ref }

// syncOnce returns how many passages were left for the next time.
func (m *Module) syncOnce(ctx context.Context, opts api.KnowledgeSettings) (int, error) {
	em, ok := m.embedder()
	if !ok || !em.Available(ctx) {
		return 0, errors.New("还没有配置 AI 供应商，不能建索引")
	}
	rows, err := m.q.ListKnowledgeDocs(ctx)
	if err != nil {
		return 0, err
	}
	existing := make(map[string]db.KnowledgeDoc, len(rows))
	for _, r := range rows {
		existing[docKey(r.Source, r.Ref)] = r
	}
	sources := m.sources()
	// Sources that were switched off, or are not registered any more, lose their index.
	for _, r := range rows {
		if _, registered := sources[r.Source]; !registered || !sourceEnabled(opts, r.Source) {
			if err := m.q.DeleteKnowledgeDoc(ctx, db.DeleteKnowledgeDocParams{Source: r.Source, Ref: r.Ref}); err != nil {
				return 0, err
			}
			delete(existing, docKey(r.Source, r.Ref))
		}
	}
	mdl := model(opts)
	budget, pending := m.budget, 0
	for _, name := range AllSources {
		src, registered := sources[name]
		if !registered || !sourceEnabled(opts, name) {
			continue
		}
		seen := map[string]bool{}
		err := src.Each(ctx, func(doc contracts.KnowledgeDoc) error {
			return m.indexDoc(ctx, em, opts, mdl, name, src.Module(), doc, existing, seen, &budget, &pending)
		})
		if err != nil {
			return pending, err
		}
		// Documents the source does not offer any more: deleted, archived, hidden or empty.
		for key, r := range existing {
			if r.Source == name && !seen[key] {
				if err := m.q.DeleteKnowledgeDoc(ctx, db.DeleteKnowledgeDocParams{Source: r.Source, Ref: r.Ref}); err != nil {
					return pending, err
				}
			}
		}
	}
	return pending, nil
}

func (m *Module) indexDoc(ctx context.Context, em contracts.Embedder, opts api.KnowledgeSettings, mdl, source, mod string,
	doc contracts.KnowledgeDoc, existing map[string]db.KnowledgeDoc, seen map[string]bool, budget, pending *int) error {
	title := strings.TrimSpace(doc.Title)
	chunks := chunkText(doc.Text)
	if len(chunks) == 0 {
		return nil // not seen: an empty document leaves the index
	}
	key := docKey(source, doc.Ref)
	seen[key] = true
	sum := sha256.Sum256([]byte(mdl + "\x00" + title + "\x00" + strings.Join(chunks, "\x01")))
	hash := hex.EncodeToString(sum[:])
	old, had := existing[key]
	if had && old.Hash == hash {
		if old.Module != mod || old.Title != title || old.Link != doc.Link {
			return m.q.UpdateKnowledgeDocMeta(ctx, db.UpdateKnowledgeDocMetaParams{Module: mod, Title: title, Link: doc.Link, Source: source, Ref: doc.Ref})
		}
		return nil
	}
	if len(chunks) > *budget {
		*pending += len(chunks)
		return nil
	}
	inputs := make([]string, len(chunks))
	for i, c := range chunks {
		inputs[i] = c
		if title != "" {
			inputs[i] = title + "\n" + c
		}
	}
	vecs, err := em.Embed(ctx, opts.Model, inputs)
	if err != nil {
		return err
	}
	if len(vecs) != len(chunks) {
		return errors.New("嵌入接口返回的条数不对")
	}
	tx, err := m.d.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	q := m.q.WithTx(tx)
	if err := q.DeleteKnowledgeChunks(ctx, db.DeleteKnowledgeChunksParams{Source: source, Ref: doc.Ref}); err != nil {
		return err
	}
	if err := q.UpsertKnowledgeDoc(ctx, db.UpsertKnowledgeDocParams{Source: source, Ref: doc.Ref, Module: mod, Title: title,
		Link: doc.Link, Hash: hash, Model: mdl, IndexedAt: m.now().UTC()}); err != nil {
		return err
	}
	for i, c := range chunks {
		if err := q.InsertKnowledgeChunk(ctx, db.InsertKnowledgeChunkParams{Source: source, Ref: doc.Ref, Seq: int64(i), Text: c,
			Embedding: encodeVec(normalize(vecs[i]))}); err != nil {
			return err
		}
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	*budget -= len(chunks)
	return nil
}

// vec is one passage in memory.
type vec struct {
	source, ref, text, module, title, link, model string
	v                                             []float32
}

// loadVecs reads every passage once and keeps them until the next sync.
func (m *Module) loadVecs(ctx context.Context) ([]vec, error) {
	m.mu.Lock()
	if m.cached {
		out := m.cache
		m.mu.Unlock()
		return out, nil
	}
	m.mu.Unlock()
	rows, err := m.q.ListKnowledgeChunks(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]vec, 0, len(rows))
	for _, r := range rows {
		v := decodeVec(r.Embedding)
		if len(v) == 0 {
			continue
		}
		out = append(out, vec{source: r.Source, ref: r.Ref, text: r.Text, module: r.Module, title: r.Title, link: r.Link, model: r.Model, v: v})
	}
	m.mu.Lock()
	m.cache, m.cached = out, true
	m.mu.Unlock()
	return out, nil
}
