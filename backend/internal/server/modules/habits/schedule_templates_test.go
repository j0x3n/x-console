package habits

import "testing"

func TestHealthTemplateDefaults(t *testing.T) {
	cases := []struct {
		name, mode, condition string
		interval              int
	}{
		{"water", modeInterval, "awake", 60},
		{"eyes", modeInterval, "active", 20},
		{"move", modeInterval, "active", 45},
		{"medicine", modeTimes, "window", 0},
	}
	for _, tc := range cases {
		got, err := healthTemplate(tc.name)
		if err != nil || got.Mode != tc.mode || got.Interval != tc.interval || len(got.When) != 1 || got.When[0] != tc.condition || got.Target <= 0 || reminderHint(tc.name) == "" {
			t.Fatalf("template %s: %+v %v", tc.name, got, err)
		}
	}
	medicine, _ := healthTemplate("medicine")
	if len(medicine.Times) != 2 || medicine.Times[0] != "09:00" || medicine.Times[1] != "21:00" {
		t.Fatalf("medicine: %+v", medicine)
	}
	if _, err := healthTemplate("invalid"); err == nil {
		t.Fatal("invalid template accepted")
	}
}
