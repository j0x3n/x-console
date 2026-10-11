package github

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/j0x3n/x-console/backend/internal/server/contracts"
	"github.com/j0x3n/x-console/backend/internal/server/httpx"
)

// goalMetrics gives the goals module (B128) the number of the user's own
// commits and merged pull requests. It counts what the activity source finds
// in the cache, so it does not reach further back than the cache does and can
// be lower than the real number.
type goalMetrics struct{ m *Module }

func (goalMetrics) Kinds() []contracts.GoalMetricKind {
	return []contracts.GoalMetricKind{
		{Kind: contracts.GoalMetricGitHubCommits, Module: "github", RefKind: contracts.GoalRefNone},
		{Kind: contracts.GoalMetricGitHubMergedPRs, Module: "github", RefKind: contracts.GoalRefNone},
	}
}

func (goalMetrics) Options(context.Context, string) ([]contracts.GoalMetricOption, error) {
	return nil, nil
}

func (g goalMetrics) Measure(ctx context.Context, kind, _ string, from, until time.Time) (float64, error) {
	if kind != contracts.GoalMetricGitHubCommits && kind != contracts.GoalMetricGitHubMergedPRs {
		return 0, errors.New("不认识的数据来源")
	}
	// without a connected account the activity source returns nothing, which
	// would read as zero commits. Say it is missing instead.
	if _, err := g.m.selected(ctx, nil, nil); err != nil {
		if errors.Is(err, httpx.ErrIntegrationMissing) {
			return 0, contracts.ErrGoalMetricMissing
		}
		return 0, err
	}
	items, err := activitySource{g.m}.Activity(ctx, from, until)
	if err != nil {
		return 0, err
	}
	n := 0
	for _, it := range items {
		switch {
		case kind == contracts.GoalMetricGitHubCommits && it.Kind == "commit":
			n++
		case kind == contracts.GoalMetricGitHubMergedPRs && it.Kind == "pr" && strings.HasSuffix(it.Ref, ":merged"):
			n++
		}
	}
	return float64(n), nil
}
