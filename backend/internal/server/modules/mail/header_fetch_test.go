package mail

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/emersion/go-imap/v2"
	"github.com/emersion/go-imap/v2/imapclient"
)

// Check bytes sent by the real client, rather than interpreting FetchOptions
// with the same library on both sides of the test.
func TestSummaryFetchWireCompatibility(t *testing.T) {
	for _, tt := range []struct {
		name     string
		statuses []string
		wantErr  string
	}{
		{name: "standard server", statuses: []string{"OK"}},
		{name: "rejects header fields", statuses: []string{"BAD", "OK"}},
		{name: "NO is not retried", statuses: []string{"NO"}, wantErr: "FETCH HEADER.FIELDS"},
		{name: "fallback failure", statuses: []string{"BAD", "NO"}, wantErr: "重试 FETCH HEADER"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			clientConn, serverConn := net.Pipe()
			t.Cleanup(func() { clientConn.Close(); serverConn.Close() })
			type result struct {
				commands []string
				err      error
			}
			done := make(chan result, 1)
			go func() {
				var r result
				defer func() { done <- r }()
				_ = serverConn.SetDeadline(time.Now().Add(5 * time.Second))
				if _, r.err = io.WriteString(serverConn, "* PREAUTH [CAPABILITY IMAP4rev1] test ready\r\n"); r.err != nil {
					return
				}
				reader := bufio.NewReader(serverConn)
				for {
					line, err := reader.ReadString('\n')
					if errors.Is(err, io.EOF) {
						return
					}
					if err != nil {
						r.err = err
						return
					}
					r.commands = append(r.commands, strings.TrimSpace(line))
					status := "BAD"
					if i := len(r.commands) - 1; i < len(tt.statuses) {
						status = tt.statuses[i]
					}
					tag, _, _ := strings.Cut(line, " ")
					if _, r.err = fmt.Fprintf(serverConn, "%s %s invalid command or parameters\r\n", tag, status); r.err != nil {
						return
					}
				}
			}()
			c := imapclient.New(clientConn, nil)
			if err := c.WaitGreeting(); err != nil {
				t.Fatal(err)
			}
			_, err := fetchSummaries(c, imap.UIDSetNum(7, 8))
			c.Close()
			r := <-done
			if r.err != nil {
				t.Fatalf("wire server: %v", r.err)
			}
			if tt.wantErr == "" {
				if err != nil {
					t.Fatal(err)
				}
			} else {
				var status *imap.Error
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) || !errors.As(err, &status) || status.Type != imap.StatusResponseTypeNo {
					t.Fatalf("missing error context or underlying status: %v", err)
				}
			}
			if len(r.commands) != len(tt.statuses) {
				t.Fatalf("commands: %q", r.commands)
			}
			for i, command := range r.commands {
				section := `BODY.PEEK[HEADER.FIELDS ("Subject" "From" "Date" "Message-ID")]`
				if i > 0 {
					section = "BODY.PEEK[HEADER]"
				}
				if !strings.Contains(command, "UID FETCH 7:8 (") || !strings.Contains(command, section) || strings.Contains(command, "ENVELOPE") {
					t.Fatalf("unexpected request: %s", command)
				}
				if i > 0 && strings.Contains(command, "HEADER.FIELDS") {
					t.Fatalf("fallback still uses field list: %s", command)
				}
			}
		})
	}
}
