package screentime

import (
	"context"
	"sort"
)

// appsProvider gives other modules the programs of a category (B131).
type appsProvider struct{ m *Module }

// AppsOfCategory implements contracts.ScreenTimeApps. The category comes from
// the program name alone: the user's own program rules first, then the
// built-in ones. A browser that was sorted into a category by its window title
// is not a program of that category. The names are the ones the agents
// reported, once per program, sorted.
func (p appsProvider) AppsOfCategory(ctx context.Context, category string) ([]string, error) {
	rules, err := p.m.loadRules(ctx)
	if err != nil {
		return nil, err
	}
	rows, err := p.m.q.ScreenDistinctApps(ctx)
	if err != nil {
		return nil, err
	}
	seen := map[string]bool{}
	var out []string
	add := func(app string) {
		n := normApp(app)
		if n == "" || seen[n] || browsers[n] {
			return
		}
		seen[n] = true
		out = append(out, app)
	}
	for _, row := range rows {
		c, ok := customApp(rules, row.App)
		if !ok {
			c, ok = builtinApp(row.App)
		}
		if ok && c == category {
			add(row.App)
		}
	}
	for _, r := range rules {
		if r.field == "app" && r.category == category {
			add(r.pattern)
		}
	}
	sort.Strings(out)
	return out, nil
}
