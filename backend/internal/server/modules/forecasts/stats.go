package forecasts

import (
	"math"

	"github.com/j0x3n/x-console/backend/internal/server/modules/forecasts/api"
)

// scored is one forecast with a probability and a result of yes or no.
type scored struct {
	Probability int // 1 to 99
	Yes         bool
}

const (
	smallSample = 20 // fewer records than this: the whole picture is thin
	fewInBin    = 5  // fewer in a bin than this: the bin says little
)

var binEdges = []struct {
	low, high int
	label     string
}{
	{1, 20, "1–20"}, {21, 40, "21–40"}, {41, 60, "41–60"}, {61, 80, "61–80"}, {81, 99, "81–99"},
}

func round1(v float64) float64 { return math.Round(v*10) / 10 }
func round3(v float64) float64 { return math.Round(v*1000) / 1000 }

// calibrate works out the Brier score, the bins and the overall bias.
// The Brier score is the mean of (probability - outcome)^2 with the
// probability as a fraction: 0 is perfect, always saying 50 % gives 0.25.
func calibrate(items []scored) api.ForecastStats {
	out := api.ForecastStats{Count: len(items), Small: len(items) < smallSample, Bins: make([]api.ForecastBin, 0, len(binEdges))}
	var brier, sumP, hits float64
	type acc struct {
		n         int
		sumP, yes float64
	}
	bins := make([]acc, len(binEdges))
	for _, it := range items {
		p := float64(it.Probability) / 100
		o := 0.0
		if it.Yes {
			o = 1
			hits++
		}
		brier += (p - o) * (p - o)
		sumP += float64(it.Probability)
		for i, e := range binEdges {
			if it.Probability >= e.low && it.Probability <= e.high {
				bins[i].n++
				bins[i].sumP += float64(it.Probability)
				bins[i].yes += o
			}
		}
	}
	for i, e := range binEdges {
		b := api.ForecastBin{Label: e.label, Count: bins[i].n, Few: bins[i].n < fewInBin}
		if bins[i].n > 0 {
			b.MeanProbability = round1(bins[i].sumP / float64(bins[i].n))
			b.Observed = round1(bins[i].yes / float64(bins[i].n) * 100)
		}
		out.Bins = append(out.Bins, b)
	}
	if n := float64(len(items)); n > 0 {
		out.Brier = round3(brier / n)
		out.Bias = round1(sumP/n - hits/n*100)
	}
	return out
}
