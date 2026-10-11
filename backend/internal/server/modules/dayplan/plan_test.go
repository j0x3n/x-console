package dayplan

import (
	"reflect"
	"testing"
)

func hm(s string) int {
	v, ok := parseHM(s)
	if !ok {
		panic(s)
	}
	return v
}

func TestParseAndFormat(t *testing.T) {
	for in, want := range map[string]int{"00:00": 0, "09:05": 545, "23:59": 1439} {
		if got, ok := parseHM(in); !ok || got != want {
			t.Errorf("%s: %d %v", in, got, ok)
		}
		if fmtHM(want) != in {
			t.Errorf("fmt %d: %s", want, fmtHM(want))
		}
	}
	for _, bad := range []string{"", "9:00", "24:00", "09:60", "09-00", "ab:cd", "09:0a", "09:000"} {
		if _, ok := parseHM(bad); ok {
			t.Errorf("%q should be rejected", bad)
		}
	}
}

func TestFreeSlots(t *testing.T) {
	busy := []span{{hm("10:00"), hm("11:30")}, {hm("14:00"), hm("14:20")}, {hm("17:30"), hm("19:00")}}
	got := freeSlots(hm("09:00"), hm("18:00"), 0, busy)
	// 14:00 to 14:20 leaves 14:20-17:30; the 20 minutes before 14:00 stay in 11:30-14:00.
	want := []span{{hm("09:00"), hm("10:00")}, {hm("11:30"), hm("14:00")}, {hm("14:20"), hm("17:30")}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("free: %v want %v", got, want)
	}
	// From the middle of a meeting.
	got = freeSlots(hm("09:00"), hm("18:00"), hm("10:40"), busy)
	if len(got) == 0 || got[0].Start != hm("11:30") {
		t.Fatalf("from 10:40: %v", got)
	}
	// Short gaps are dropped, a meeting that covers the whole window leaves nothing.
	if got := freeSlots(hm("09:00"), hm("12:00"), 0, []span{{hm("09:10"), hm("11:40")}}); got != nil {
		t.Errorf("short gaps: %v", got)
	}
	if got := freeSlots(hm("09:00"), hm("18:00"), 0, []span{{0, 24 * 60}}); got != nil {
		t.Errorf("all busy: %v", got)
	}
	// Overlapping and out-of-order meetings.
	got = freeSlots(hm("09:00"), hm("12:00"), 0, []span{{hm("10:30"), hm("11:00")}, {hm("10:00"), hm("10:45")}})
	if !reflect.DeepEqual(got, []span{{hm("09:00"), hm("10:00")}, {hm("11:00"), hm("12:00")}}) {
		t.Errorf("overlap: %v", got)
	}
	// The work window is over already.
	if got := freeSlots(hm("09:00"), hm("18:00"), hm("18:00"), nil); got != nil {
		t.Errorf("after hours: %v", got)
	}
}

func TestRoundUp(t *testing.T) {
	for in, want := range map[int]int{0: 0, 1: 15, 15: 15, 16: 30, 545: 555} {
		if roundUp(in) != want {
			t.Errorf("%d -> %d", in, roundUp(in))
		}
	}
}

func TestScoreIssue(t *testing.T) {
	if scoreIssue(true, 0, false) != 90 || scoreIssue(false, 0, false) != 70 {
		t.Error("overdue and due today")
	}
	if scoreIssue(false, 1, false) != 90 || scoreIssue(false, 2, true) != 95 || scoreIssue(false, 4, false) != 70 {
		t.Error("priority and progress")
	}
	if !(scoreIssue(true, 1, true) > scoreIssue(false, 1, true)) {
		t.Error("overdue should win")
	}
}

func cand(title string, score, at, dur int) candidate {
	return candidate{Title: title, Kind: "issue", Score: score, At: at, Duration: dur}
}

func TestPackEarliestFit(t *testing.T) {
	slots := []span{{hm("09:00"), hm("10:00")}, {hm("13:00"), hm("15:00")}}
	got := pack([]candidate{cand("低", 10, -1, 60), cand("高", 90, -1, 60), cand("中", 50, -1, 60), cand("最低", 5, -1, 60)}, slots, 3)
	if len(got) != 3 {
		t.Fatalf("limit: %+v", got)
	}
	// Best first into the earliest slot; the next two go to the afternoon.
	if got[0].Title != "高" || got[0].Start != hm("09:00") || got[1].Title != "中" || got[1].Start != hm("13:00") || got[2].Title != "低" || got[2].Start != hm("14:00") {
		t.Fatalf("plan: %+v", got)
	}
}

func TestPackFixedTimeAndNoRoom(t *testing.T) {
	slots := []span{{hm("09:00"), hm("12:00")}}
	// A reminder at 10:00 keeps its time; the rest is planned around it.
	got := pack([]candidate{cand("提醒", 60, hm("10:00"), 30), cand("写周报", 70, -1, 60), cand("太长", 40, -1, 180)}, slots, 3)
	byTitle := map[string]planned{}
	for _, p := range got {
		byTitle[p.Title] = p
	}
	if byTitle["提醒"].Start != hm("10:00") {
		t.Errorf("reminder moved: %+v", byTitle["提醒"])
	}
	if p := byTitle["写周报"]; p.Start != hm("09:00") || p.End != hm("10:00") {
		t.Errorf("report: %+v", p)
	}
	// 180 minutes do not fit what is left: kept without a time, listed last.
	if p := byTitle["太长"]; p.Start != -1 || got[len(got)-1].Title != "太长" {
		t.Errorf("no room: %+v / %+v", p, got)
	}
	// A fixed time that is not free falls back to the earliest fit.
	got = pack([]candidate{cand("占着", 90, -1, 120), cand("提醒", 60, hm("10:00"), 30)}, slots, 3)
	if got[1].Title != "提醒" || got[1].Start != hm("11:00") {
		t.Errorf("fallback: %+v", got)
	}
	// Nothing to plan, or no slots at all.
	if len(pack(nil, slots, 3)) != 0 {
		t.Error("empty")
	}
	if p := pack([]candidate{cand("x", 1, -1, 30)}, nil, 3); len(p) != 1 || p[0].Start != -1 {
		t.Errorf("no slots: %+v", p)
	}
}

func TestValidateChoices(t *testing.T) {
	cands := []candidate{cand("a", 3, -1, 60), cand("b", 2, -1, 60), cand("c", 1, -1, 60)}
	slots := []span{{hm("09:00"), hm("10:00")}, {hm("13:00"), hm("15:00")}}
	good := []aiChoice{{1, "13:00", "14:00", "  先做  "}, {0, "09:00", "10:00", "上午精力好"}}
	got, ok := validateChoices(good, cands, slots, 3)
	if !ok || len(got) != 2 || got[0].Title != "a" || got[0].Start != hm("09:00") || got[1].Title != "b" || got[1].Reason != "先做" {
		t.Fatalf("good: %v %+v", ok, got)
	}
	// An item without a time is allowed.
	if got, ok := validateChoices([]aiChoice{{2, "", "", ""}}, cands, slots, 3); !ok || got[0].Start != -1 {
		t.Errorf("no time: %v %+v", ok, got)
	}
	bad := map[string][]aiChoice{
		"empty":           nil,
		"too many":        {{0, "09:00", "09:30", ""}, {1, "13:00", "13:30", ""}, {2, "14:00", "14:30", ""}, {0, "", "", ""}},
		"unknown":         {{5, "09:00", "10:00", ""}},
		"negative":        {{-1, "09:00", "10:00", ""}},
		"repeated":        {{0, "09:00", "09:30", ""}, {0, "13:00", "13:30", ""}},
		"inside meeting":  {{0, "10:00", "11:00", ""}},
		"half in meeting": {{0, "09:30", "10:30", ""}},
		"overlap":         {{0, "09:00", "09:40", ""}, {1, "09:30", "10:00", ""}},
		"end before":      {{0, "10:00", "09:00", ""}},
		"bad time":        {{0, "9am", "10am", ""}},
		"only start":      {{0, "09:00", "", ""}},
	}
	for name, choices := range bad {
		if _, ok := validateChoices(choices, cands, slots, 3); ok {
			t.Errorf("%s should be rejected", name)
		}
	}
}
