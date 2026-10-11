package experiments

import (
	"math"
	"testing"
)

func near(t *testing.T, name string, got, want, tol float64) {
	t.Helper()
	if math.Abs(got-want) > tol {
		t.Errorf("%s = %v, want %v (±%v)", name, got, want, tol)
	}
}

func TestSummarize(t *testing.T) {
	s := summarize([]float64{2, 4, 4, 4, 5, 5, 7, 9})
	near(t, "mean", s.Mean, 5, 1e-9)
	near(t, "sd", s.SD, 2.138089935, 1e-6) // sample sd
	if s := summarize(nil); s.N != 0 || s.Mean != 0 {
		t.Errorf("empty: %+v", s)
	}
	if s := summarize([]float64{3}); s.SD != 0 || s.Mean != 3 {
		t.Errorf("one value: %+v", s)
	}
}

func TestTQuantile(t *testing.T) {
	// Reference values of the t distribution, 97.5 % point.
	for df, want := range map[float64]float64{1: 12.706, 2: 4.303, 5: 2.571, 10: 2.228, 12: 2.179, 20: 2.086, 30: 2.042, 60: 2.000, 120: 1.980, 1000: 1.962} {
		near(t, "t", tQuantile975(df), want, 0.01)
	}
	// Fractional df lies between its neighbours.
	if v := tQuantile975(7.5); v >= tQuantile975(7) || v <= tQuantile975(8) {
		t.Errorf("t(7.5) = %v", v)
	}
	if tQuantile975(0.2) != tQuantile975(1) {
		t.Error("df below 1 should count as 1")
	}
}

func TestCompareClearDifference(t *testing.T) {
	// Two weeks without a change in level, then two weeks about 1.5 higher.
	base := []float64{6.0, 6.2, 5.8, 6.1, 5.9, 6.0, 6.3, 5.7, 6.1, 5.9, 6.0, 6.2, 5.8, 6.0}
	change := []float64{7.4, 7.6, 7.5, 7.3, 7.7, 7.5, 7.4, 7.6, 7.5, 7.5, 7.3, 7.7, 7.5, 7.4}
	o := compare(base, change)
	if o.Verdict != verdictHigher {
		t.Fatalf("verdict %s: %+v", o.Verdict, o)
	}
	near(t, "diff", o.Diff, 1.5, 0.05)
	if o.CILow <= 1 || o.CIHigh >= 2 || o.CILow >= o.CIHigh {
		t.Errorf("interval %v..%v", o.CILow, o.CIHigh)
	}
	if !o.HasEffect || o.Effect < 5 {
		t.Errorf("effect %v", o.Effect)
	}
	// The same data the other way round is lower.
	if o2 := compare(change, base); o2.Verdict != verdictLower || o2.Diff > -1.4 {
		t.Errorf("reversed: %+v", o2)
	}
}

func TestCompareNoise(t *testing.T) {
	a := []float64{5, 7, 6, 4, 8, 6, 5}
	b := []float64{6, 5, 7, 6, 4, 8, 6}
	o := compare(a, b)
	if o.Verdict != verdictInconclusive || o.CILow >= 0 || o.CIHigh <= 0 {
		t.Fatalf("noise should be inconclusive: %+v", o)
	}
}

func TestCompareSmallSamples(t *testing.T) {
	few := []float64{1, 2, 3, 4}
	many := []float64{1, 2, 3, 4, 5, 6}
	if o := compare(few, many); o.Verdict != verdictTooFew || o.Diff != many[0]*0+summarize(many).Mean-summarize(few).Mean {
		t.Errorf("too few: %+v", o)
	}
	if o := compare(nil, many); o.Verdict != verdictNoData {
		t.Errorf("no data: %+v", o)
	}
	if o := compare(many, nil); o.Verdict != verdictNoData {
		t.Errorf("no data: %+v", o)
	}
	// Exactly five days in each phase is enough.
	if o := compare([]float64{1, 2, 3, 4, 5}, []float64{9, 9, 9, 9, 9}); o.Verdict != verdictHigher {
		t.Errorf("five and five: %+v", o)
	}
}

func TestCompareConstantPhases(t *testing.T) {
	o := compare([]float64{3, 3, 3, 3, 3}, []float64{3, 3, 3, 3, 3})
	if o.Verdict != verdictInconclusive || o.HasEffect || o.Diff != 0 {
		t.Errorf("identical constants: %+v", o)
	}
	o = compare([]float64{3, 3, 3, 3, 3}, []float64{4, 4, 4, 4, 4})
	if o.Verdict != verdictHigher || o.CILow != 1 || o.CIHigh != 1 {
		t.Errorf("shifted constants: %+v", o)
	}
}

func TestSplit(t *testing.T) {
	pts := []point{
		{"2026-11-10", 1},                    // before the start
		{"2026-11-12", 2}, {"2026-11-11", 3}, // out of order on purpose
		{"2026-11-15", 4}, {"2026-11-20", 5},
		{"2026-11-21", 6}, // after the end
	}
	b, c := split(pts, "2026-11-11", "2026-11-15", "2026-11-20")
	if len(b) != 2 || b[0].Date != "2026-11-11" || b[1].Date != "2026-11-12" {
		t.Errorf("baseline: %+v", b)
	}
	if len(c) != 2 || c[0].Date != "2026-11-15" || c[1].Date != "2026-11-20" {
		t.Errorf("change: %+v", c)
	}
	if got := values(b); len(got) != 2 || got[0] != 3 {
		t.Errorf("values: %v", got)
	}
}
