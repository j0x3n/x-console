package mail

import (
	"strings"
	"testing"

	"github.com/emersion/go-imap/v2"
	"github.com/emersion/go-imap/v2/imapclient"
)

func TestSummaryAcceptsReorderedHeaders(t *testing.T) {
	b := &imapclient.FetchMessageBuffer{UID: 17, BodySection: []imapclient.FetchBodySectionBuffer{{
		Section: &imap.FetchItemBodySection{Specifier: imap.PartSpecifierHeader, HeaderFields: []string{"MESSAGE-ID", "date", "from", "subject"}},
		Bytes:   []byte("Subject: =?UTF-8?B?5Lit5paH5Li76aKY?=\r\nFrom: \"=?UTF-8?B?5bCP546L?=\" <sender@example.com>\r\nMessage-ID: <seventeen@example.com>\r\n\r\n"),
	}}}
	if b.FindBodySection(summaryHeaderSection()) != nil {
		t.Fatal("fixture should reproduce the dependency's order-sensitive matching")
	}
	row := messageRow(1, b, "snippet")
	if row.Subject != "中文主题" || row.FromName != "小王" || row.FromAddress != "sender@example.com" || row.MessageID != "seventeen@example.com" {
		t.Fatalf("lost summary headers: %+v", row)
	}
}

func TestQuotedEncodedAddressNames(t *testing.T) {
	h := summaryHeader([]byte("To: \"=?UTF-8?B?5bCP546L?=\" <qq@example.com>\r\nCc: \"=?UTF-8?Q?Alice=27s_team?=\" <cc@example.com>\r\n\r\n"))
	for _, tt := range []struct{ field, want string }{{"To", "小王"}, {"Cc", "Alice's team"}} {
		got := addresses(h, tt.field)
		if len(got) != 1 || got[0].Name == nil || *got[0].Name != tt.want {
			t.Fatalf("%s quoted encoded name: %+v", tt.field, got)
		}
		if strings.Contains(*got[0].Name, "=?") {
			t.Fatalf("encoded name leaked: %s", *got[0].Name)
		}
	}
}

func TestSummaryHeaderSectionBoundaries(t *testing.T) {
	for _, tt := range []struct {
		name    string
		section *imap.FetchItemBodySection
		valid   bool
	}{
		{"complete empty header", fullHeaderSection(), true},
		{"body", &imap.FetchItemBodySection{}, false},
		{"text", &imap.FetchItemBodySection{Specifier: imap.PartSpecifierText}, false},
		{"nested header", &imap.FetchItemBodySection{Specifier: imap.PartSpecifierHeader, Part: []int{1}}, false},
		{"partial header", &imap.FetchItemBodySection{Specifier: imap.PartSpecifierHeader, Partial: &imap.SectionPartial{Offset: 0, Size: 10}}, false},
		{"excluded fields", &imap.FetchItemBodySection{Specifier: imap.PartSpecifierHeader, HeaderFieldsNot: []string{"Subject"}}, false},
		{"missing fields", &imap.FetchItemBodySection{Specifier: imap.PartSpecifierHeader, HeaderFields: []string{"Subject"}}, false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			b := &imapclient.FetchMessageBuffer{UID: 1, BodySection: []imapclient.FetchBodySectionBuffer{{Section: tt.section, Bytes: []byte("\r\n")}}}
			_, err := checkedSummaryHeaders([]*imapclient.FetchMessageBuffer{b})
			if (err == nil) != tt.valid {
				t.Fatalf("valid=%t err=%v", tt.valid, err)
			}
		})
	}
	if _, err := checkedSummaryHeaders([]*imapclient.FetchMessageBuffer{{UID: 1}}); err == nil {
		t.Fatal("missing header must fail before writing a summary or advancing the UID")
	}
}
