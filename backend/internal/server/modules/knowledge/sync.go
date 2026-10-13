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

	pending, skipped, err := m.syncOnce(ctx, opts)

	m.mu.Lock()
	m.syncing = false
	m.last = m.now().UTC()
	m.pending = pending
	m.skipped = skipped
	m.lastErr = ""
	if err != nil {
		m.lastErr = err.Error()
		m.d.Log.Warn("knowledge sync", "err", err)
	}
	m.mu.Unlock()
	m.invalidate()
}

func docKey(source, ref string) string { return source + "\x00" + ref }

// syncOnce returns how many passages were left for the next time, and how
// many files had no readable text.
func (m *Module) syncOnce(ctx context.Context, opts api.KnowledgeSettings) (int, int, error) {
	em, ok := m.embedder()
	if !ok || !em.Available(ctx, providerID(opts)) {
		return 0, 0, errors.New(noProvider(opts))
	}
	rows, err := m.q.ListKnowledgeDocs(ctx)
	if err != nil {
		return 0, 0, err
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
				return 0, 0, err
			}
			delete(existing, docKey(r.Source, r.Ref))
		}
	}
	mdl := indexKey(opts)
	run := &syncRun{em: em, opts: opts, mdl: mdl, existing: existing, budget: m.budget}
	for _, name := range indexOrder() {
		src, registered := sources[name]
		if !registered || !sourceEnabled(opts, name) {
			continue
		}
		seen := map[string]bool{}
		var err error
		if fs, ok := src.(fileSource); ok {
			err = fs.EachFile(ctx, opts, func(doc fileDoc) error {
				return m.indexDoc(ctx, run, name, src.Module(), doc, seen)
			})
		} else {
			err = src.Each(ctx, func(doc contracts.KnowledgeDoc) error {
				return m.indexDoc(ctx, run, name, src.Module(), fileDoc{KnowledgeDoc: doc}, seen)
			})
		}
		if err != nil {
			return run.pending, run.skipped, err
		}
		// Documents the source does not offer any more: deleted, archived, hidden or empty.
		for key, r := range existing {
			if r.Source == name && !seen[key] {
				if err := m.q.DeleteKnowledgeDoc(ctx, db.DeleteKnowledgeDocParams{Source: r.Source, Ref: r.Ref}); err != nil {
					return run.pending, run.skipped, err
				}
			}
		}
	}
	return run.pending, run.skipped, nil
}

// syncRun is the state of one sync.
type syncRun struct {
	em       contracts.Embedder
	opts     api.KnowledgeSettings
	mdl      string
	existing map[string]db.KnowledgeDoc
	budget   int // passages that may still be embedded
	pending  int // passages left for the next sync
	skipped  int // files without readable text
}

func hashOf(parts ...string) string {
	sum := sha256.Sum256([]byte(strings.Join(parts, "\x00")))
	return hex.EncodeToString(sum[:])
}

// indexDoc embeds a document when it changed. A document with a Load
// function is a file: its version decides whether it changed, and its text is
// read only then.
func (m *Module) indexDoc(ctx context.Context, run *syncRun, source, mod string, doc fileDoc, seen map[string]bool) error {
	title := strings.TrimSpace(doc.Title)
	key := docKey(source, doc.Ref)
	old, had := run.existing[key]
	keep := func() error {
		seen[key] = true
		if old.Module != mod || old.Title != title || old.Link != doc.Link {
			return m.q.UpdateKnowledgeDocMeta(ctx, db.UpdateKnowledgeDocMetaParams{Module: mod, Title: title, Link: doc.Link, Source: source, Ref: doc.Ref})
		}
		return nil
	}
	var chunks []string
	var hash string
	if doc.Load != nil {
		hash = hashOf(run.mdl, title, "file", doc.Version)
		if had && old.Hash == hash {
			return keep()
		}
		if m.isUnreadable(doc.Version) {
			run.skipped++
			return nil
		}
		text, err := doc.Load()
		var bad unreadableError
		if errors.As(err, &bad) {
			m.markUnreadable(doc.Version)
			run.skipped++
			m.d.Log.Info("knowledge file has no text", "source", source, "ref", doc.Ref, "err", err)
			return nil
		}
		if err != nil {
			return err
		}
		chunks = chunkText(text)
	} else {
		chunks = chunkText(doc.Text)
		hash = hashOf(run.mdl, title, strings.Join(chunks, "\x01"))
	}
	if len(chunks) == 0 {
		return nil // not seen: an empty document leaves the index
	}
	if had && old.Hash == hash {
		return keep()
	}
	seen[key] = true
	if len(chunks) > run.budget {
		run.pending += len(chunks)
		return nil
	}
	inputs := make([]string, len(chunks))
	for i, c := range chunks {
		inputs[i] = c
		if title != "" {
			inputs[i] = title + "\n" + c
		}
	}
	vecs, err := run.em.Embed(ctx, providerID(run.opts), run.opts.Model, inputs)
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
		Link: doc.Link, Hash: hash, Model: run.mdl, IndexedAt: m.now().UTC()}); err != nil {
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
	run.budget -= len(chunks)
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

func (m *Module) isUnreadable(version string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.unreadable[version]
}

func (m *Module) markUnreadable(version string) {
	m.mu.Lock()
	m.unreadable[version] = true
	m.mu.Unlock()
}
