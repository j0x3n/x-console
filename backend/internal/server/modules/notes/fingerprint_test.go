package notes_test

import (
	"encoding/json"
	"math/rand"
	"strings"
	"testing"

	"github.com/j0x3n/x-console/backend/internal/server/modules/notes"
)

// longNote is about n characters of random Chinese text; seed picks the text.
func longNote(seed int64, n int) string {
	r := rand.New(rand.NewSource(seed))
	var b strings.Builder
	for i := 0; i < n; i++ {
		b.WriteRune(rune(0x4E00 + r.Intn(0x9FA5-0x4E00)))
		if i%40 == 39 {
			b.WriteString("\n")
		}
	}
	return b.String()
}

func TestNoteFingerprintStaysSmall(t *testing.T) {
	body := longNote(1, 100000) // about 100k characters
	fp := notes.NoteFingerprintForTest(body)
	if len(fp) > 3000 {
		t.Fatalf("fingerprint is %d bytes", len(fp))
	}
	if notes.ChangedEnoughForTest(fp, notes.NoteFingerprintForTest(body+"补一句。")) {
		t.Fatal("a one-line edit counted as a big change")
	}
	if !notes.ChangedEnoughForTest(fp, notes.NoteFingerprintForTest(longNote(2, 100000))) {
		t.Fatal("a rewritten note did not count as a big change")
	}
}

func TestNoteFingerprintReadsOldFormat(t *testing.T) {
	// Before the cap, the fingerprint held every hash, in order.
	body := longNote(1, 3000)
	var capped []uint64
	_ = json.Unmarshal([]byte(notes.NoteFingerprintForTest(body)), &capped)
	full := append([]uint64{}, capped...)
	for i := range 500 {
		full = append(full, capped[len(capped)-1]+uint64(i)+1) // larger than every kept hash
	}
	raw, _ := json.Marshal(full)
	if notes.ChangedEnoughForTest(string(raw), notes.NoteFingerprintForTest(body)) {
		t.Fatal("old full fingerprint of the same text counted as changed")
	}
}
