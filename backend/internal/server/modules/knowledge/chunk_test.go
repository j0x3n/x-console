package knowledge

import (
	"math"
	"strings"
	"testing"
)

func TestChunkShortAndEmpty(t *testing.T) {
	if got := chunkText("  \n "); len(got) != 0 {
		t.Fatalf("blank: %q", got)
	}
	got := chunkText("  一小段话  ")
	if len(got) != 1 || got[0] != "一小段话" {
		t.Fatalf("short: %q", got)
	}
}

func TestChunkBreaksAtSentencesAndOverlaps(t *testing.T) {
	sentence := strings.Repeat("字", 99) + "。" // 100 runes
	text := strings.Repeat(sentence, 20)      // 2000 runes
	got := chunkText(text)
	if len(got) < 3 {
		t.Fatalf("expected several passages, got %d", len(got))
	}
	for i, c := range got {
		if n := len([]rune(c)); n > chunkSize {
			t.Errorf("passage %d has %d runes", i, n)
		}
		if !strings.HasSuffix(c, "。") {
			t.Errorf("passage %d does not end at a sentence: %q", i, string([]rune(c)[len([]rune(c))-3:]))
		}
	}
	// The overlap: the start of passage 2 is the end of passage 1.
	first, second := []rune(got[0]), []rune(got[1])
	tail := string(first[len(first)-chunkLap/2:])
	if !strings.Contains(string(second[:chunkLap+10]), tail) {
		t.Errorf("no overlap between passages")
	}
}

func TestChunkLimits(t *testing.T) {
	// No break characters: cut at chunkSize.
	got := chunkText(strings.Repeat("a", 5000))
	if len([]rune(got[0])) != chunkSize {
		t.Errorf("first passage: %d runes", len([]rune(got[0])))
	}
	// A huge document is capped in passages and in length.
	got = chunkText(strings.Repeat("字", 500000))
	if len(got) != maxChunks {
		t.Errorf("passages: %d", len(got))
	}
	// Everything is kept up to the cap: 20000 runes need more than 25 passages of 700, so the cap bites.
	total := 0
	for _, c := range got {
		total += len([]rune(c))
	}
	if total > maxChunks*chunkSize {
		t.Errorf("total %d", total)
	}
}

func TestChunkTerminates(t *testing.T) {
	// Breaks only near the start of the window must not loop forever.
	text := "。" + strings.Repeat("b", 3000)
	if got := chunkText(text); len(got) == 0 {
		t.Fatal("nothing")
	}
}

func TestVectors(t *testing.T) {
	v := normalize([]float32{3, 4})
	if math.Abs(float64(v[0])-0.6) > 1e-6 || math.Abs(float64(v[1])-0.8) > 1e-6 {
		t.Fatalf("normalize: %v", v)
	}
	if z := normalize([]float32{0, 0}); z[0] != 0 || z[1] != 0 {
		t.Fatalf("zero: %v", z)
	}
	back := decodeVec(encodeVec(v))
	if len(back) != 2 || back[0] != v[0] || back[1] != v[1] {
		t.Fatalf("roundtrip: %v", back)
	}
	if decodeVec([]byte{1, 2, 3}) != nil {
		t.Error("odd length should not decode")
	}
	a, b := normalize([]float32{1, 0}), normalize([]float32{1, 1})
	if d := dot(a, b); math.Abs(float64(d)-math.Sqrt2/2) > 1e-6 {
		t.Errorf("dot: %v", d)
	}
}
