package ai

import (
	"context"
	"database/sql"
	"encoding/csv"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	openapi_types "github.com/oapi-codegen/runtime/types"

	"github.com/j0x3n/x-console/backend/internal/server/httpx"
	"github.com/j0x3n/x-console/backend/internal/server/modules/ai/api"
)

// B42 AI 用量：缓存命中、按天和模型统计、明细、导出。

// usageKeepDays is how long usage rows are kept.
const usageKeepDays = 400

// usagePrices are USD per million tokens; nil when unknown.
type usagePrices struct {
	Input, Output, CacheRead, CacheWrite *float32
}

// usageCost prices one call. input counts every input token, cached and
// cacheWrite are the parts read from and written to the prompt cache. When a
// cache price is missing that part is priced as normal input and estimated
// is true. It returns nil when the model has no input or output price.
func usageCost(input, cached, cacheWrite, output int64, p usagePrices) (cost *float64, estimated bool) {
	if p.Input == nil || p.Output == nil {
		return nil, false
	}
	plain := input - cached - cacheWrite
	if plain < 0 {
		plain = 0
	}
	readPrice, writePrice := float64(*p.Input), float64(*p.Input)
	if p.CacheRead != nil {
		readPrice = float64(*p.CacheRead)
	} else if cached > 0 {
		estimated = true
	}
	if p.CacheWrite != nil {
		writePrice = float64(*p.CacheWrite)
	} else if cacheWrite > 0 {
		estimated = true
	}
	v := (float64(plain)*float64(*p.Input) + float64(cached)*readPrice +
		float64(cacheWrite)*writePrice + float64(output)*float64(*p.Output)) / 1_000_000
	return &v, estimated
}

// usageRow is one AI call to record.
type usageRow struct {
	ProviderID                                   *int64
	ProviderName, Model, Purpose, Source, Ref    string
	Input, Cached, CacheWrite, Output, Reasoning int64
	Duration                                     time.Duration
	Err                                          error
}

// writeUsage stores one call. A known cost (for example Claude Code's
// total_cost_usd) wins over the price calculation.
func (m *Module) writeUsage(ctx context.Context, row usageRow, prices usagePrices, knownCost *float64) {
	cost, estimated := usageCost(row.Input, row.Cached, row.CacheWrite, row.Output, prices)
	if knownCost != nil {
		cost, estimated = knownCost, false
	}
	status, errText := "ok", ""
	if row.Err != nil {
		status, errText = "error", clipRunes(row.Err.Error(), 500)
	}
	purpose := row.Purpose
	if purpose != "fast" {
		purpose = "agent"
	}
	var providerID any
	if row.ProviderID != nil && *row.ProviderID != 0 {
		providerID = *row.ProviderID
	}
	_, err := m.d.DB.ExecContext(ctx, `INSERT INTO ai_usage(provider_id,provider_name,model,purpose,input_tokens,output_tokens,
 cached_input_tokens,cache_write_tokens,reasoning_tokens,source,ref,status,error,cost_estimated,duration_ms,cost,created_at)
 VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		providerID, row.ProviderName, row.Model, purpose, row.Input, row.Output,
		row.Cached, row.CacheWrite, row.Reasoning, row.Source, row.Ref, status, errText, boolInt(estimated),
		row.Duration.Milliseconds(), cost, time.Now().UTC())
	if err != nil {
		m.d.Log.Error("AI usage write failed", "err", err, "callErr", row.Err)
	}
}

// RecordExternalUsage stores usage reported by something outside the llm
// package, such as an executor on an agent (B42 part 2).
func (m *Module) RecordExternalUsage(ctx context.Context, provider, model, source, ref string, input, cached, cacheWrite, output int64, duration time.Duration, costUSD *float64) {
	m.writeUsage(ctx, usageRow{ProviderName: provider, Model: model, Purpose: "agent", Source: source, Ref: ref,
		Input: input, Cached: cached, CacheWrite: cacheWrite, Output: output, Duration: duration}, usagePrices{}, costUSD)
}

func (m *Module) cleanupUsage(ctx context.Context) error {
	_, err := m.d.DB.ExecContext(ctx, `DELETE FROM ai_usage WHERE created_at < ?`,
		time.Now().UTC().AddDate(0, 0, -usageKeepDays))
	return err
}

func boolInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

func clipRunes(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n])
}

// usageRange turns inclusive local dates into a UTC [start, end) range.
func (m *Module) usageRange(from, to openapi_types.Date) (start, end time.Time, days int, err error) {
	loc := m.d.Config.Location
	f := time.Date(from.Year(), from.Month(), from.Day(), 0, 0, 0, 0, loc)
	t := time.Date(to.Year(), to.Month(), to.Day(), 0, 0, 0, 0, loc)
	if t.Before(f) {
		return start, end, 0, httpx.Invalid("结束日期不能早于开始日期")
	}
	days = int(t.Sub(f).Hours()/24+0.5) + 1
	if days > 366 {
		return start, end, 0, httpx.Invalid("时间范围最多 366 天")
	}
	return f.UTC(), t.AddDate(0, 0, 1).UTC(), days, nil
}

type usageFilter struct {
	start, end            time.Time
	model, source, status string
}

func (f usageFilter) where() (string, []any) {
	sql := `created_at>=? AND created_at<?`
	args := []any{f.start, f.end}
	if f.model != "" {
		sql += ` AND model=?`
		args = append(args, f.model)
	}
	if f.source != "" {
		sql += ` AND source=?`
		args = append(args, f.source)
	}
	if f.status != "" {
		sql += ` AND status=?`
		args = append(args, f.status)
	}
	return sql, args
}

const usageColumns = `id,created_at,provider_name,model,purpose,source,ref,status,error,input_tokens,cached_input_tokens,
 cache_write_tokens,output_tokens,reasoning_tokens,duration_ms,cost,cost_estimated`

func scanUsage(rows *sql.Rows) (api.AiUsageRecord, error) {
	var r api.AiUsageRecord
	var errText string
	var cost sql.NullFloat64
	var estimated int
	var purpose, status string
	if err := rows.Scan(&r.Id, &r.At, &r.ProviderName, &r.Model, &purpose, &r.Source, &r.Ref, &status, &errText,
		&r.InputTokens, &r.CachedInputTokens, &r.CacheWriteTokens, &r.OutputTokens, &r.ReasoningTokens,
		&r.DurationMs, &cost, &estimated); err != nil {
		return r, err
	}
	r.Purpose, r.Status = api.AiUsageRecordPurpose(purpose), api.AiUsageRecordStatus(status)
	if errText != "" {
		r.Error = &errText
	}
	if cost.Valid {
		c := float32(cost.Float64)
		r.Cost = &c
	}
	if estimated == 1 {
		t := true
		r.CostEstimated = &t
	}
	return r, nil
}

// totals accumulates usage rows.
type totals struct {
	calls, errors                                int
	input, cached, cacheWrite, output, reasoning int64
	cost                                         float64
	hasCost, estimated                           bool
	durationMs                                   int64
}

func (t *totals) add(r api.AiUsageRecord) {
	t.calls++
	if r.Status == "error" {
		t.errors++
	}
	t.input += r.InputTokens
	t.cached += r.CachedInputTokens
	t.cacheWrite += r.CacheWriteTokens
	t.output += r.OutputTokens
	t.reasoning += r.ReasoningTokens
	t.durationMs += int64(r.DurationMs)
	if r.Cost != nil {
		t.cost += float64(*r.Cost)
		t.hasCost = true
	}
	if r.CostEstimated != nil && *r.CostEstimated {
		t.estimated = true
	}
}

func (t totals) api() api.AiUsageTotals {
	out := api.AiUsageTotals{Calls: t.calls, Errors: t.errors, InputTokens: t.input, CachedInputTokens: t.cached,
		CacheWriteTokens: t.cacheWrite, OutputTokens: t.output, ReasoningTokens: t.reasoning}
	if t.calls > 0 {
		out.AvgDurationMs = int(t.durationMs / int64(t.calls))
	}
	if t.input > 0 {
		rate := float32(float64(t.cached) / float64(t.input))
		out.CacheHitRate = &rate
	}
	if t.hasCost {
		c := float32(t.cost)
		out.Cost = &c
	}
	if t.estimated {
		out.CostEstimated = &t.estimated
	}
	return out
}

// eachUsage calls fn for every row in the filter, oldest first.
func (m *Module) eachUsage(ctx context.Context, f usageFilter, fn func(api.AiUsageRecord)) error {
	where, args := f.where()
	rows, err := m.d.DB.QueryContext(ctx, `SELECT `+usageColumns+` FROM ai_usage WHERE `+where+` ORDER BY id`, args...)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		r, err := scanUsage(rows)
		if err != nil {
			return err
		}
		fn(r)
	}
	return rows.Err()
}

func (m *Module) GetAiUsageSummary(w http.ResponseWriter, r *http.Request, params api.GetAiUsageSummaryParams) {
	start, end, days, err := m.usageRange(params.From, params.To)
	if m.fail(w, r, err) {
		return
	}
	loc := m.d.Config.Location
	groups := map[string]*totals{}
	providers := map[string]string{}
	var order []string
	if params.GroupBy == api.GetAiUsageSummaryParamsGroupByDay {
		for d := 0; d < days; d++ {
			key := start.In(loc).AddDate(0, 0, d).Format(time.DateOnly)
			groups[key] = &totals{}
			order = append(order, key)
		}
	}
	var total totals
	err = m.eachUsage(r.Context(), usageFilter{start: start, end: end}, func(row api.AiUsageRecord) {
		total.add(row)
		var key string
		switch params.GroupBy {
		case api.GetAiUsageSummaryParamsGroupByDay:
			key = row.At.In(loc).Format(time.DateOnly)
		case api.GetAiUsageSummaryParamsGroupByModel:
			key = row.Model
			providers[key] = row.ProviderName
		case api.GetAiUsageSummaryParamsGroupBySource:
			key = row.Source
		case api.GetAiUsageSummaryParamsGroupByProvider:
			key = row.ProviderName
		default:
			return
		}
		g := groups[key]
		if g == nil {
			g = &totals{}
			groups[key] = g
			order = append(order, key)
		}
		g.add(row)
	})
	if m.fail(w, r, err) {
		return
	}
	switch params.GroupBy {
	case api.GetAiUsageSummaryParamsGroupByDay, api.GetAiUsageSummaryParamsGroupByModel,
		api.GetAiUsageSummaryParamsGroupBySource, api.GetAiUsageSummaryParamsGroupByProvider:
	default:
		httpx.Fail(w, r, httpx.Invalid("groupBy 不正确"))
		return
	}
	var previous totals
	span := end.Sub(start)
	err = m.eachUsage(r.Context(), usageFilter{start: start.Add(-span), end: start}, previous.add)
	if m.fail(w, r, err) {
		return
	}
	out := api.AiUsageSummary{From: params.From, To: params.To, GroupBy: api.AiUsageSummaryGroupBy(params.GroupBy),
		Total: total.api(), Groups: make([]api.AiUsageGroup, 0, len(order))}
	prev := previous.api()
	out.Previous = &prev
	if params.GroupBy != api.GetAiUsageSummaryParamsGroupByDay {
		// Biggest spenders first: cost, then tokens.
		sort.SliceStable(order, func(i, j int) bool {
			ga, gb := groups[order[i]], groups[order[j]]
			if ga.cost != gb.cost {
				return ga.cost > gb.cost
			}
			return ga.input+ga.output > gb.input+gb.output
		})
	}
	for _, key := range order {
		g := api.AiUsageGroup{Key: key, Totals: groups[key].api()}
		if name, ok := providers[key]; ok {
			g.ProviderName = &name
		}
		out.Groups = append(out.Groups, g)
	}
	httpx.JSON(w, http.StatusOK, out)
}

func (m *Module) usageFilter(from, to openapi_types.Date, model, source *string, status *string) (usageFilter, error) {
	start, end, _, err := m.usageRange(from, to)
	if err != nil {
		return usageFilter{}, err
	}
	f := usageFilter{start: start, end: end}
	if model != nil {
		f.model = *model
	}
	if source != nil {
		f.source = *source
	}
	if status != nil {
		f.status = *status
	}
	return f, nil
}

func (m *Module) ListAiUsageRecords(w http.ResponseWriter, r *http.Request, params api.ListAiUsageRecordsParams) {
	f, err := m.usageFilter(params.From, params.To, params.Model, params.Source, (*string)(params.Status))
	if m.fail(w, r, err) {
		return
	}
	limit := 50
	if params.Limit != nil {
		limit = min(max(*params.Limit, 1), 200)
	}
	where, args := f.where()
	if params.Cursor != nil && *params.Cursor != "" {
		id, err := strconv.ParseInt(*params.Cursor, 10, 64)
		if err != nil {
			httpx.Fail(w, r, httpx.Invalid("cursor 不正确"))
			return
		}
		where += ` AND id<?`
		args = append(args, id)
	}
	args = append(args, limit+1)
	rows, err := m.d.DB.QueryContext(r.Context(), `SELECT `+usageColumns+` FROM ai_usage WHERE `+where+` ORDER BY id DESC LIMIT ?`, args...)
	if m.fail(w, r, err) {
		return
	}
	defer rows.Close()
	items := make([]api.AiUsageRecord, 0, limit)
	for rows.Next() {
		rec, err := scanUsage(rows)
		if m.fail(w, r, err) {
			return
		}
		items = append(items, rec)
	}
	if m.fail(w, r, rows.Err()) {
		return
	}
	var next *string
	if len(items) > limit {
		items = items[:limit]
		c := strconv.FormatInt(items[limit-1].Id, 10)
		next = &c
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"items": items, "nextCursor": next})
}

func (m *Module) ExportAiUsageRecords(w http.ResponseWriter, r *http.Request, params api.ExportAiUsageRecordsParams) {
	f, err := m.usageFilter(params.From, params.To, params.Model, params.Source, (*string)(params.Status))
	if m.fail(w, r, err) {
		return
	}
	var rows []api.AiUsageRecord
	if m.fail(w, r, m.eachUsage(r.Context(), f, func(rec api.AiUsageRecord) { rows = append(rows, rec) })) {
		return
	}
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="ai-usage-`+params.From.String()+`-`+params.To.String()+`.csv"`)
	w.Header().Set("X-Content-Type-Options", "nosniff")
	_, _ = w.Write([]byte("\xef\xbb\xbf")) // UTF-8 BOM, for Excel
	cw := csv.NewWriter(w)
	_ = cw.Write([]string{"时间", "供应商", "模型", "用途", "来源", "关联", "状态", "输入", "命中缓存", "写入缓存", "输出", "思考", "耗时毫秒", "费用美元", "费用是估算", "报错"})
	loc := m.d.Config.Location
	for _, rec := range rows {
		cost := ""
		if rec.Cost != nil {
			cost = strconv.FormatFloat(float64(*rec.Cost), 'f', 6, 64)
		}
		estimated := ""
		if rec.CostEstimated != nil && *rec.CostEstimated {
			estimated = "是"
		}
		errText := ""
		if rec.Error != nil {
			errText = *rec.Error
		}
		_ = cw.Write([]string{rec.At.In(loc).Format("2006-01-02 15:04:05"), csvSafe(rec.ProviderName), csvSafe(rec.Model), string(rec.Purpose),
			csvSafe(rec.Source), csvSafe(rec.Ref), string(rec.Status), i64(rec.InputTokens), i64(rec.CachedInputTokens), i64(rec.CacheWriteTokens),
			i64(rec.OutputTokens), i64(rec.ReasoningTokens), strconv.Itoa(rec.DurationMs), cost, estimated, csvSafe(errText)})
	}
	cw.Flush()
}

func i64(v int64) string { return strconv.FormatInt(v, 10) }

// csvSafe stops spreadsheet programs from treating a cell as a formula.
func csvSafe(s string) string {
	if s != "" && strings.ContainsRune("=+-@", rune(s[0])) {
		return "'" + s
	}
	return s
}
