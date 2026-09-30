package actions

import (
	"context"
	"encoding/json"
	"testing"
)

func TestRegisterRunList(t *testing.T) {
	r := NewRegistry()
	r.Register(Action{Name: "b.echo", Title: "回显", Input: Schema(`{"type":"object"}`), Effect: Read,
		Run: func(ctx context.Context, in json.RawMessage) (any, error) { return string(in), nil }})
	r.Register(Action{Name: "a.noop", Title: "空", Input: Schema(`{"type":"object"}`), Effect: Write,
		Run: func(ctx context.Context, in json.RawMessage) (any, error) { return nil, nil }})
	out, err := r.Run(context.Background(), "b.echo", nil)
	if err != nil || out != "{}" {
		t.Fatalf("run: %v %v", out, err)
	}
	if _, err := r.Run(context.Background(), "missing", nil); err == nil {
		t.Fatal("want error for unknown action")
	}
	if list := r.List(); len(list) != 2 || list[0].Name != "a.noop" {
		t.Fatalf("list: %+v", list)
	}
	// An alias runs but is not listed.
	r.Register(Action{Name: "c.echo", Input: Schema(`{"type":"object"}`), Effect: Read, AliasOf: "b.echo",
		Run: func(ctx context.Context, in json.RawMessage) (any, error) { return "alias", nil }})
	if out, err := r.Run(context.Background(), "c.echo", nil); err != nil || out != "alias" {
		t.Fatalf("alias run: %v %v", out, err)
	}
	if list := r.List(); len(list) != 2 {
		t.Fatalf("alias listed: %+v", list)
	}
	defer func() {
		if recover() == nil {
			t.Fatal("duplicate register should panic")
		}
	}()
	r.Register(Action{Name: "a.noop", Input: Schema(`{}`), Effect: Read, Run: func(context.Context, json.RawMessage) (any, error) { return nil, nil }})
}
