package reminders

import (
	"encoding/json"
	"testing"
	"time"
)

func TestPushPayloadSentAt(t *testing.T) {
	now := time.Date(2026, 10, 1, 0, 4, 5, 0, time.UTC)
	loc := time.FixedZone("CST", 8*3600)
	raw, err := json.Marshal(withSentAt(pushPayload{
		Kind: "test", Title: "X Console 测试消息", Body: testPushBody(now, loc), Priority: "normal",
	}, now))
	if err != nil {
		t.Fatal(err)
	}
	var got map[string]any
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatal(err)
	}
	if got["sentAt"] != "2026-10-01T00:04:05Z" {
		t.Fatalf("sentAt: %v", got["sentAt"])
	}
	if got["body"] != "收到这条说明浏览器推送能用。发送时间 08:04:05" {
		t.Fatalf("body: %v", got["body"])
	}
	if pushTTL != 3*24*3600 {
		t.Fatalf("ttl: %d", pushTTL)
	}
}
