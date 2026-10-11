package brief_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/j0x3n/x-console/backend/internal/server/contracts"
	"github.com/j0x3n/x-console/backend/internal/server/module"
	"github.com/j0x3n/x-console/backend/internal/server/modules/brief"
)

type fakeCards struct {
	n   int
	err error
}

func (f fakeCards) DueCount(context.Context) (int, error) { return f.n, f.err }

// B130: the brief tells how many review cards wait. No cards, no section.
func TestBriefCardsSection(t *testing.T) {
	env, m, ws := setup(t)
	s := baseSettings(ws)
	s["sections"] = []string{"cards"}
	putSettings(t, env, s)

	gen := func(p contracts.CardsDue) (string, []string) {
		t.Helper()
		reg := module.NewRegistry()
		if p != nil {
			module.Provide[contracts.CardsDue](reg, contracts.CardsDueKey, p)
		}
		content, keys, err := brief.Generate(m, context.Background(), reg, at(27, 8, 0))
		if err != nil {
			t.Fatal(err)
		}
		return content, keys
	}
	content, keys := gen(fakeCards{n: 12})
	if strings.Join(keys, ",") != "cards" || !strings.Contains(content, "## 待复习的卡片\n\n今天有 12 张卡待复习。") {
		t.Fatalf("with cards: %v\n%s", keys, content)
	}
	if _, keys := gen(fakeCards{n: 0}); len(keys) != 0 {
		t.Fatalf("zero cards: %v", keys)
	}
	if _, keys := gen(fakeCards{err: errors.New("boom")}); len(keys) != 0 {
		t.Fatalf("failing provider: %v", keys)
	}
	if _, keys := gen(nil); len(keys) != 0 {
		t.Fatalf("no cards module: %v", keys)
	}

	// The section can be chosen in the settings.
	var view struct{ Sections []string }
	env.MustDo("GET", "/briefs/settings", nil, &view)
	if len(view.Sections) != 1 || view.Sections[0] != "cards" {
		t.Fatalf("saved sections: %v", view.Sections)
	}
}
