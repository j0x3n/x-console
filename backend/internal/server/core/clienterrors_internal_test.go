package core

import (
	"testing"
	"time"
)

func TestClientErrorLogLimitsPerSession(t *testing.T) {
	var l clientErrorLog
	now := time.Now()
	for i := 0; i < clientErrorLimit; i++ {
		if !l.allow("a", now) {
			t.Fatalf("report %d refused", i)
		}
	}
	if l.allow("a", now) {
		t.Fatal("over the limit accepted")
	}
	if !l.allow("b", now) {
		t.Fatal("other session refused")
	}
	if !l.allow("a", now.Add(61*time.Second)) {
		t.Fatal("not reset after a minute")
	}
}
