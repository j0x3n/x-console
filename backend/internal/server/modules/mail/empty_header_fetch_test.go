package mail

import (
	"bufio"
	"fmt"
	"io"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/emersion/go-imap/v2"
	"github.com/emersion/go-imap/v2/imapclient"
)

func TestSummaryRetriesSuccessfulButEmptyHeaders(t *testing.T) {
	for _, emptyFullHeader := range []bool{false, true} {
		t.Run(fmt.Sprintf("empty full header=%t", emptyFullHeader), func(t *testing.T) {
			client, server := net.Pipe()
			t.Cleanup(func() { client.Close(); server.Close() })
			done := make(chan error, 1)
			header := "Subject: Restored subject\r\nFrom: Sender <sender@example.com>\r\nMessage-ID: <restored@example.com>\r\n\r\n"
			go func() {
				_ = server.SetDeadline(time.Now().Add(5 * time.Second))
				if _, err := io.WriteString(server, "* PREAUTH [CAPABILITY IMAP4rev1] ready\r\n"); err != nil {
					done <- err
					return
				}
				reader := bufio.NewReader(server)
				steps := 2
				if emptyFullHeader {
					steps++
				}
				for i := range steps {
					line, err := reader.ReadString('\n')
					if err != nil {
						done <- err
						return
					}
					tag, _, _ := strings.Cut(line, " ")
					section, raw := `HEADER.FIELDS ("Subject" "From" "Date" "Message-ID")`, "\r\n"
					if i == 1 {
						section, raw = "HEADER", header
						if emptyFullHeader {
							raw = "\r\n"
						}
					}
					if i == 2 {
						// A complete BODY[] reply may omit the requested <0> marker.
						section, raw = "", header+"Restored message body"
					}
					if !strings.Contains(line, "BODY.PEEK[") || strings.Contains(line, "ENVELOPE") {
						done <- fmt.Errorf("unexpected command: %s", line)
						return
					}
					if _, err := fmt.Fprintf(server, "* 1 FETCH (UID 7 BODY[%s] {%d}\r\n%s)\r\n%s OK done\r\n", section, len(raw), raw, tag); err != nil {
						done <- err
						return
					}
				}
				done <- nil
			}()
			c := imapclient.New(client, nil)
			if err := c.WaitGreeting(); err != nil {
				t.Fatal(err)
			}
			msgs, err := fetchSummaries(c, imap.UIDSetNum(7))
			if err != nil {
				t.Fatal(err)
			}
			if err := <-done; err != nil {
				t.Fatal(err)
			}
			if len(msgs) != 1 {
				t.Fatalf("messages: %d", len(msgs))
			}
			row := messageRow(1, msgs[0], "")
			if row.Subject != "Restored subject" || row.FromAddress != "sender@example.com" {
				t.Fatalf("empty metadata accepted: %+v", row)
			}
		})
	}
}

func TestWholeBodySectionDoesNotAcceptOtherParts(t *testing.T) {
	for _, section := range []*imap.FetchItemBodySection{
		{Specifier: imap.PartSpecifierHeader},
		{Specifier: imap.PartSpecifierText},
		{Part: []int{1}},
		{Partial: &imap.SectionPartial{Offset: 1024}},
	} {
		b := &imapclient.FetchMessageBuffer{BodySection: []imapclient.FetchBodySectionBuffer{{Section: section, Bytes: []byte("wrong part")}}}
		if messageBodyBytes(b) != nil {
			t.Fatalf("accepted wrong section: %+v", section)
		}
	}
}
