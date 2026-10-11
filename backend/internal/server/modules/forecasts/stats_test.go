package forecasts

import (
	"math"
	"testing"
)

func near(t *testing.T, name string, got, want float64) {
	t.Helper()
	if math.Abs(got-want) > 1e-9 {
		t.Errorf("%s = %v, want %v", name, got, want)
	}
}

func TestCalibrateEmpty(t *testing.T) {
	s := calibrate(nil)
	if s.Count != 0 || s.Brier != 0 || s.Bias != 0 || !s.Small || len(s.Bins) != 5 {
		t.Fatalf("empty: %+v", s)
	}
	for _, b := range s.Bins {
		if b.Count != 0 || b.MeanProbability != 0 || !b.Few {
			t.Errorf("bin: %+v", b)
		}
	}
}

func TestBrierByHand(t *testing.T) {
	// 0.9 and yes: 0.01. 0.7 and no: 0.49. 0.5 and yes: 0.25. Mean = 0.25.
	s := calibrate([]scored{{90, true}, {70, false}, {50, true}})
	near(t, "brier", s.Brier, 0.25)
	// Always saying 50 % gives 0.25 whatever happens.
	s = calibrate([]scored{{50, true}, {50, false}, {50, false}, {50, true}, {50, true}})
	near(t, "coin", s.Brier, 0.25)
	// A perfect, bold forecaster.
	s = calibrate([]scored{{99, true}, {1, false}})
	if s.Brier > 0.001 {
		t.Errorf("bold = %v, want about 0", s.Brier)
	}
	// Bold and wrong is worse than 0.25.
	if calibrate([]scored{{99, false}, {1, true}}).Brier < 0.9 {
		t.Error("bold and wrong should score badly")
	}
}

func TestBinsAndBias(t *testing.T) {
	items := []scored{
		{10, false}, {20, false}, // bin 1–20
		{30, true},                                                  // 21–40
		{70, true}, {75, true}, {80, false}, {65, true}, {61, true}, // 61–80
		{90, true}, {95, false}, // 81–99
	}
	s := calibrate(items)
	if s.Count != 10 || !s.Small {
		t.Fatalf("count: %+v", s)
	}
	b := s.Bins
	if b[0].Count != 2 || b[0].MeanProbability != 15 || b[0].Observed != 0 || !b[0].Few {
		t.Errorf("1–20: %+v", b[0])
	}
	if b[1].Count != 1 || b[1].Observed != 100 {
		t.Errorf("21–40: %+v", b[1])
	}
	if b[2].Count != 0 {
		t.Errorf("41–60: %+v", b[2])
	}
	// 61–80: 70, 75, 80, 65, 61 → mean 70.2, four of five came true → 80.
	if b[3].Count != 5 || b[3].MeanProbability != 70.2 || b[3].Observed != 80 || b[3].Few {
		t.Errorf("61–80: %+v", b[3])
	}
	if b[4].Count != 2 || b[4].MeanProbability != 92.5 || b[4].Observed != 50 {
		t.Errorf("81–99: %+v", b[4])
	}
	// Mean stated 59.6 (596/10), hit rate 6/10 = 60 → bias −0.4.
	near(t, "bias", s.Bias, -0.4)
	// Twenty records are no longer a small sample.
	many := make([]scored, 20)
	for i := range many {
		many[i] = scored{60, i%2 == 0}
	}
	if calibrate(many).Small {
		t.Error("20 records should not be small")
	}
}

func TestBiasSign(t *testing.T) {
	over := calibrate([]scored{{90, false}, {90, true}, {90, false}})
	if over.Bias <= 0 {
		t.Errorf("over-confident should be positive: %v", over.Bias)
	}
	under := calibrate([]scored{{20, true}, {20, true}, {20, false}})
	if under.Bias >= 0 {
		t.Errorf("under-confident should be negative: %v", under.Bias)
	}
}
