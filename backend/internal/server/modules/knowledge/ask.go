package knowledge

import (
	"context"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/j0x3n/x-console/backend/internal/server/contracts"
	"github.com/j0x3n/x-console/backend/internal/server/httpx"
	"github.com/j0x3n/x-console/backend/internal/server/module"
	"github.com/j0x3n/x-console/backend/internal/server/modules/knowledge/api"
)

const askSystem = "你是用户个人资料的助手。只根据下面带编号的资料片段回答问题，在用到的句子后面用 [编号] 标出处。" +
	"片段里没有答案就直接说资料里没找到，不要编造，不要用片段以外的知识补充。用中文回答，简短，直接。"

// search finds the passages closest to the question.
func (m *Module) search(ctx context.Context, question string) ([]api.KnowledgeHit, error) {
	question = strings.TrimSpace(question)
	if question == "" || utf8.RuneCountInString(question) > maxQuestion {
		return nil, httpx.Invalid("问题要写 1 到 500 个字")
	}
	opts := m.options(ctx)
	if !opts.Enabled {
		return nil, httpx.Invalid("资料问答还没开启，请先在设置里开启")
	}
	em, ok := m.embedder()
	if !ok || !em.Available(ctx) {
		return nil, httpx.Invalid("还没有配置 AI 供应商")
	}
	qv, err := em.Embed(ctx, opts.Model, []string{question})
	if err != nil {
		return nil, err
	}
	if len(qv) != 1 || len(qv[0]) == 0 {
		return nil, httpx.Invalid("嵌入接口没有返回向量")
	}
	q := normalize(qv[0])
	all, err := m.loadVecs(ctx)
	if err != nil {
		return nil, err
	}
	hidden := m.hiddenFn(ctx)
	mdl := model(opts)
	type scored struct {
		v     *vec
		score float32
	}
	var cands []scored
	for i := range all {
		v := &all[i]
		if v.model != mdl || len(v.v) != len(q) || !sourceEnabled(opts, v.source) || hidden(v.module) {
			continue
		}
		if s := dot(q, v.v); s >= minScore {
			cands = append(cands, scored{v, s})
		}
	}
	sort.Slice(cands, func(i, j int) bool { return cands[i].score > cands[j].score })
	perDoc := map[string]int{}
	hits := []api.KnowledgeHit{}
	for _, c := range cands {
		key := docKey(c.v.source, c.v.ref)
		if perDoc[key] >= perDocLimit {
			continue
		}
		perDoc[key]++
		hits = append(hits, api.KnowledgeHit{Source: c.v.source, Title: c.v.title, Link: c.v.link, Snippet: c.v.text, Score: round3(float64(c.score))})
		if len(hits) == topK {
			break
		}
	}
	return hits, nil
}

func round3(f float64) float64 { return float64(int(f*1000+0.5)) / 1000 }

func (m *Module) ask(ctx context.Context, question string) (api.KnowledgeAnswer, error) {
	hits, err := m.search(ctx, question)
	if err != nil {
		return api.KnowledgeAnswer{}, err
	}
	out := api.KnowledgeAnswer{Sources: hits}
	if len(hits) == 0 {
		out.Answer = ptr("资料里没有找到相关内容。")
		return out, nil
	}
	llm, ok := module.Lookup[contracts.LLM](m.d.Registry, contracts.LLMKey)
	if !ok || !llm.Available(ctx) {
		return out, nil
	}
	var b strings.Builder
	fmt.Fprintf(&b, "问题：%s\n\n资料片段：\n", strings.TrimSpace(question))
	for i, h := range hits {
		fmt.Fprintf(&b, "\n[%d] %s\n%s\n", i+1, h.Title, h.Snippet)
	}
	text, err := llm.CompleteText(contracts.WithAIUsage(ctx, "knowledge", ""), "fast", askSystem, b.String())
	if err != nil {
		m.d.Log.Warn("knowledge answer", "err", err)
		return out, nil // the sources are still worth showing
	}
	if text = strings.TrimSpace(text); text != "" {
		out.Answer = &text
	}
	return out, nil
}

// AskKnowledge implements api.ServerInterface.
func (m *Module) AskKnowledge(w http.ResponseWriter, r *http.Request) {
	var body api.AskKnowledgeJSONBody
	if err := httpx.Decode(r, &body); fail(w, r, err) {
		return
	}
	out, err := m.ask(r.Context(), body.Question)
	if fail(w, r, err) {
		return
	}
	httpx.JSON(w, http.StatusOK, out)
}

// SearchKnowledge implements api.ServerInterface.
func (m *Module) SearchKnowledge(w http.ResponseWriter, r *http.Request, params api.SearchKnowledgeParams) {
	hits, err := m.search(r.Context(), params.Q)
	if fail(w, r, err) {
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"items": hits})
}
