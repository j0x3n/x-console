package syslog

import (
	"testing"

	"golang.org/x/text/encoding/simplifiedchinese"
	"golang.org/x/text/encoding/unicode"
)

const gbkEvent = `<Event><System><TimeCreated SystemTime='2026-10-01T08:00:00.0000000Z'/><EventRecordID>7</EventRecordID></System><RenderingInfo><Message>服务已启动</Message></RenderingInfo></Event>`

func TestToUTF8DecodesGBKAndUTF16(t *testing.T) {
	gbk, err := simplifiedchinese.GBK.NewEncoder().String(gbkEvent)
	if err != nil {
		t.Fatal(err)
	}
	rows, err := parseEvents(toUTF8([]byte(gbk), 936))
	if err != nil || len(rows) != 1 || rows[0].entry.Message != "服务已启动" {
		t.Fatalf("gbk: rows=%+v err=%v", rows, err)
	}
	for _, bom := range []unicode.BOMPolicy{unicode.UseBOM, unicode.IgnoreBOM} {
		u16, _ := unicode.UTF16(unicode.LittleEndian, bom).NewEncoder().String(gbkEvent)
		rows, err := parseEvents(toUTF8([]byte(u16), 0))
		if err != nil || len(rows) != 1 || rows[0].entry.Message != "服务已启动" {
			t.Fatalf("utf16 bom=%v: rows=%+v err=%v", bom, rows, err)
		}
	}
	if got := string(toUTF8([]byte("plain"), 936)); got != "plain" {
		t.Fatalf("utf8 changed: %q", got)
	}
	if _, err := parseEvents(toUTF8([]byte(gbk), 0)); err != nil {
		t.Fatalf("unknown code page should not fail parsing: %v", err)
	}
}
