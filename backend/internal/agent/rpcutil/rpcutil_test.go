package rpcutil

import (
	"errors"
	"os"
	"testing"

	"github.com/j0x3n/x-console/backend/pkg/protocol"
)

func TestFromOS(t *testing.T) {
	_, err := os.Stat("/definitely/not/here")
	var pe *protocol.Error
	if !errors.As(FromOS(err), &pe) || pe.Code != protocol.CodeNotFound {
		t.Fatalf("not found: %v", FromOS(err))
	}
	if !errors.As(FromOS(errors.New("x")), &pe) || pe.Code != protocol.CodeFailed {
		t.Fatalf("generic: %v", pe)
	}
	if FromOS(nil) != nil {
		t.Fatal("nil")
	}
}

func TestDecode(t *testing.T) {
	var v struct{ A int }
	if err := Decode(nil, &v); err != nil {
		t.Fatal(err)
	}
	if err := Decode([]byte(`{"A":2}`), &v); err != nil || v.A != 2 {
		t.Fatal(err, v)
	}
	var pe *protocol.Error
	if err := Decode([]byte(`{`), &v); !errors.As(err, &pe) || pe.Code != protocol.CodeBadParams {
		t.Fatal(err)
	}
}
