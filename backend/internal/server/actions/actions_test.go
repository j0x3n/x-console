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
	if list := r.List(context.Background()); len(list) != 2 || list[0].Name != "a.noop" {
		t.Fatalf("list: %+v", list)
	}
	// An alias runs but is not listed.
	r.Register(Action{Name: "c.echo", Input: Schema(`{"type":"object"}`), Effect: Read, AliasOf: "b.echo",
		Run: func(ctx context.Context, in json.RawMessage) (any, error) { return "alias", nil }})
	if out, err := r.Run(context.Background(), "c.echo", nil); err != nil || out != "alias" {
		t.Fatalf("alias run: %v %v", out, err)
	}
	if list := r.List(context.Background()); len(list) != 2 {
		t.Fatalf("alias listed: %+v", list)
	}
	defer func() {
		if recover() == nil {
			t.Fatal("duplicate register should panic")
		}
	}()
	r.Register(Action{Name: "a.noop", Input: Schema(`{}`), Effect: Read, Run: func(context.Context, json.RawMessage) (any, error) { return nil, nil }})
}

// B43/B47：外部调用方（API 令牌、内置 Agent）能用哪些动作。
func TestAllowedFor(t *testing.T) {
	run := func(context.Context, json.RawMessage) (any, error) { return nil, nil }
	schema := Schema(`{}`)
	read := Action{Name: "notes.search", Effect: Read, Input: schema, Run: run}
	write := Action{Name: "notes.create", Effect: Write, Input: schema, Run: run}
	del := Action{Name: "notes.delete", Effect: Write, Input: schema, Run: run}
	danger := Action{Name: "hosts.exec", Effect: Dangerous, Input: schema, Run: run}
	alias := Action{Name: "issues.list", Effect: Read, Input: schema, Run: run, AliasOf: "projects.list_issues"}
	cases := []struct {
		a       Action
		access  string
		modules []string
		want    bool
	}{
		{read, AccessRead, nil, true},
		{write, AccessRead, nil, false},
		{write, AccessWrite, nil, true},
		{del, AccessWrite, nil, false},
		{del, AccessWriteDelete, nil, true},
		{danger, AccessWriteDelete, nil, false},
		{alias, AccessRead, nil, false},
		{write, AccessWrite, []string{"notes"}, true},
		{write, AccessWrite, []string{"reminders"}, false},
		{read, "admin", nil, false},
	}
	for _, c := range cases {
		if got := AllowedFor(c.a, c.access, c.modules); got != c.want {
			t.Errorf("%s %s %v: got %v", c.a.Name, c.access, c.modules, got)
		}
	}
}
