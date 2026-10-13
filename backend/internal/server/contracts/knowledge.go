package contracts

import (
	"context"
	"time"
)

// EmbedderKey is where the ai module registers Embedder (B139).
const EmbedderKey = "ai.embedder"

// Embedder turns texts into vectors with the AI provider the user configured
// (an OpenAI compatible /embeddings endpoint).
//
// providerID picks the AI provider (B197). 0 uses the provider chosen for
// fast AI calls.
type Embedder interface {
	// Available says whether the provider exists.
	Available(ctx context.Context, providerID int64) bool
	// Embed returns one vector per input, in the same order. model is the
	// model name; empty uses the provider's default.
	Embed(ctx context.Context, providerID int64, model string, inputs []string) ([][]float32, error)
}

// KnowledgeSourcePrefix is the registry key prefix of the modules whose text
// the knowledge module (B139) indexes. The last part of the key is the source
// name, for example "knowledge.sources.notes".
const KnowledgeSourcePrefix = "knowledge.sources."

// KnowledgeDoc is one document to index.
type KnowledgeDoc struct {
	// Ref is unique inside the source, for example "12".
	Ref   string
	Title string
	// Text is the body that is cut into passages. The source limits its size.
	Text string
	// Link is an in-app path that opens the document.
	Link string
	At   time.Time
}

// KnowledgeSource offers documents for the index. It must leave out content
// the user hid with a password or the vault (hidden notes, for example): such
// content never enters the index.
type KnowledgeSource interface {
	// Module is the hideable module the documents belong to (notes,
	// readlater, journal). While it is hidden and locked, the knowledge
	// module does not return the source's passages.
	Module() string
	// Each calls fn for every document that can be indexed now. It stops at
	// the first error fn returns.
	Each(ctx context.Context, fn func(KnowledgeDoc) error) error
}
