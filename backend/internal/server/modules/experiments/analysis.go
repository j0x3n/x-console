package experiments

import (
	"math"
	"sort"
)

// A daily value of a metric.
type point struct {
	Date  string
	Value float64
}

// stats are the numbers of one phase.
type stats struct {
	N    int
	Mean float64
	SD   float64
}

// minDays is the fewest days with data a phase needs before the module says
// anything about the difference.
const minDays = 5

const (
	verdictNoData       = "no_data"
	verdictTooFew       = "too_few"
	verdictInconclusive = "inconclusive"
	verdictHigher       = "higher"
	verdictLower        = "lower"
)

// outcome is the comparison of the two phases of one metric.
type outcome struct {
	Verdict  string
	Baseline stats
	Change   stats
	Diff     float64
	CILow    float64
	CIHigh   float64
	// Effect is Cohen's d. HasEffect is false when the pooled deviation is 0.
	Effect    float64
	HasEffect bool
}

func summarize(values []float64) stats {
	s := stats{N: len(values)}
	if s.N == 0 {
		return s
	}
	for _, v := range values {
		s.Mean += v
	}
	s.Mean /= float64(s.N)
	if s.N > 1 {
		var ss float64
		for _, v := range values {
			ss += (v - s.Mean) * (v - s.Mean)
		}
		s.SD = math.Sqrt(ss / float64(s.N-1))
	}
	return s
}

// compare takes the values of the baseline and of the change phase.
func compare(baseline, change []float64) outcome {
	o := outcome{Baseline: summarize(baseline), Change: summarize(change)}
	if o.Baseline.N == 0 || o.Change.N == 0 {
		o.Verdict = verdictNoData
		return o
	}
	o.Diff = o.Change.Mean - o.Baseline.Mean
	if o.Baseline.N < minDays || o.Change.N < minDays {
		o.Verdict = verdictTooFew
		return o
	}
	n1, n2 := float64(o.Baseline.N), float64(o.Change.N)
	v1, v2 := o.Baseline.SD*o.Baseline.SD/n1, o.Change.SD*o.Change.SD/n2
	se := math.Sqrt(v1 + v2)
	if se == 0 {
		// Both phases are constant. Any difference is exact, and 0 is not a
		// difference to explain.
		o.CILow, o.CIHigh = o.Diff, o.Diff
	} else {
		df := (v1 + v2) * (v1 + v2) / (v1*v1/(n1-1) + v2*v2/(n2-1))
		half := tQuantile975(df) * se
		o.CILow, o.CIHigh = o.Diff-half, o.Diff+half
	}
	pooled := math.Sqrt(((n1-1)*o.Baseline.SD*o.Baseline.SD + (n2-1)*o.Change.SD*o.Change.SD) / (n1 + n2 - 2))
	if pooled > 0 {
		o.Effect, o.HasEffect = o.Diff/pooled, true
	}
	switch {
	case o.CILow > 0:
		o.Verdict = verdictHigher
	case o.CIHigh < 0:
		o.Verdict = verdictLower
	default:
		o.Verdict = verdictInconclusive
	}
	return o
}

// tQuantile975 is the 97.5 % point of Student's t distribution with df
// degrees of freedom (df may be fractional, as in Welch's test). Small df use
// a table and interpolate; larger df use the Cornish-Fisher expansion around
// the normal quantile.
func tQuantile975(df float64) float64 {
	if df < 1 {
		df = 1
	}
	if df <= 10 {
		table := []float64{12.706, 4.303, 3.182, 2.776, 2.571, 2.447, 2.365, 2.306, 2.262, 2.228}
		lo := int(math.Floor(df))
		if lo >= len(table) {
			return table[len(table)-1]
		}
		frac := df - float64(lo)
		if frac == 0 || lo == len(table)-1 {
			return table[lo-1]
		}
		return table[lo-1] + (table[lo]-table[lo-1])*frac
	}
	z := 1.959964
	return z + (z*z*z+z)/(4*df) + (5*math.Pow(z, 5)+16*z*z*z+3*z)/(96*df*df)
}

// split divides daily points into the two phases by date text (YYYY-MM-DD
// compares correctly as text). Points before baseline start or after end are
// dropped. The values of each phase come back in date order.
func split(points []point, baselineStart, changeStart, end string) (baseline, change []point) {
	sorted := append([]point(nil), points...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].Date < sorted[j].Date })
	for _, p := range sorted {
		switch {
		case p.Date < baselineStart || p.Date > end:
		case p.Date < changeStart:
			baseline = append(baseline, p)
		default:
			change = append(change, p)
		}
	}
	return baseline, change
}

func values(ps []point) []float64 {
	out := make([]float64, len(ps))
	for i, p := range ps {
		out[i] = p.Value
	}
	return out
}
