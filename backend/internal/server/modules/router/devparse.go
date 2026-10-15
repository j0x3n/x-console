package router

// B228, B229: parsers for the sections the report script adds. Each one takes
// the text as the router printed it and ignores what it does not understand,
// so a firmware with a different format loses a feature, not the report.

import (
	"encoding/json"
	"net/netip"
	"strconv"
	"strings"
)

// deviceMAC turns a MAC into the stored form, upper case with colons. It
// returns "" for text that is not a MAC and for addresses that are no device:
// all zero, broadcast and multicast.
func deviceMAC(s string) string {
	s = strings.TrimSpace(s)
	if len(s) == 12 && !strings.ContainsAny(s, ":-") {
		s = colonMAC(s)
	}
	s = strings.ReplaceAll(s, "-", ":")
	if len(s) != 17 {
		return ""
	}
	var first byte
	zero := true
	for i := 0; i < 17; i++ {
		c := s[i]
		if i%3 == 2 {
			if c != ':' {
				return ""
			}
			continue
		}
		v, ok := hexVal(c)
		if !ok {
			return ""
		}
		if v != 0 {
			zero = false
		}
		if i == 0 {
			first = v
		}
		if i == 1 {
			first = first<<4 | v
		}
	}
	if zero || first&1 == 1 {
		return ""
	}
	return strings.ToUpper(s)
}

func hexVal(c byte) (byte, bool) {
	switch {
	case c >= '0' && c <= '9':
		return c - '0', true
	case c >= 'a' && c <= 'f':
		return c - 'a' + 10, true
	case c >= 'A' && c <= 'F':
		return c - 'A' + 10, true
	}
	return 0, false
}

// parseWifi reads #xc:wifi. The script writes one block per interface. A block
// starts with "=hostapd <ubus object>" followed by the JSON of
// `ubus call <object> get_clients`, or with "=iwinfo <interface>" followed by
// the text of `iwinfo <interface> assoclist`. It returns MAC to interface. The
// second result is false when a block could not be read, which happens when
// the ubus call failed: the list is then not to be trusted.
func parseWifi(text string) (map[string]string, bool) {
	out := map[string]string{}
	ok := true
	var kind, iface string
	var buf strings.Builder
	flush := func() {
		body := buf.String()
		buf.Reset()
		switch kind {
		case "hostapd":
			var v struct {
				Clients map[string]struct {
					Assoc *bool `json:"assoc"`
				} `json:"clients"`
			}
			if json.Unmarshal([]byte(body), &v) != nil {
				ok = false
				return
			}
			for k, c := range v.Clients {
				if mac := deviceMAC(k); mac != "" && (c.Assoc == nil || *c.Assoc) {
					out[mac] = iface
				}
			}
		case "iwinfo":
			for _, line := range strings.Split(body, "\n") {
				f := strings.Fields(line)
				if len(f) == 0 {
					continue
				}
				if mac := deviceMAC(f[0]); mac != "" {
					out[mac] = iface
				}
			}
		}
	}
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimRight(line, "\r")
		if rest, ok := strings.CutPrefix(line, "="); ok {
			flush()
			k, name, _ := strings.Cut(strings.TrimSpace(rest), " ")
			kind, iface = k, strings.TrimPrefix(strings.TrimSpace(name), "hostapd.")
			continue
		}
		buf.WriteString(line)
		buf.WriteByte('\n')
	}
	flush()
	return out, ok
}

// neighEntry is one line of `ip neigh show`.
type neighEntry struct {
	ip    netip.Addr
	dev   string
	mac   string
	state string // REACHABLE, STALE ...; "" when the table has no state (ARP)
}

// reachable says the router talked to the neighbor a moment ago, which means
// it is on the network now. STALE is old news.
func (n neighEntry) reachable() bool {
	switch n.state {
	case "REACHABLE", "DELAY", "PROBE", "PERMANENT":
		return true
	}
	return false
}

// parseNeigh reads `ip neigh show`:
// "192.168.1.10 dev br-lan lladdr aa:bb:cc:dd:ee:ff REACHABLE".
// Entries without a MAC (FAILED, INCOMPLETE) are dropped.
func parseNeigh(text string) []neighEntry {
	var out []neighEntry
	for _, line := range strings.Split(text, "\n") {
		f := strings.Fields(line)
		if len(f) < 4 {
			continue
		}
		ip, err := netip.ParseAddr(f[0])
		if err != nil {
			continue
		}
		e := neighEntry{ip: ip.Unmap()}
		for i := 1; i+1 < len(f); i++ {
			switch f[i] {
			case "dev":
				e.dev = f[i+1]
			case "lladdr":
				e.mac = deviceMAC(f[i+1])
			}
		}
		if e.mac == "" {
			continue
		}
		if last := f[len(f)-1]; isUpperWord(last) {
			e.state = last
		}
		out = append(out, e)
	}
	return out
}

// parseARPEntries reads /proc/net/arp for routers whose script sends no
// neighbor table. Only complete entries (flag 0x2) are returned.
func parseARPEntries(text string) []neighEntry {
	var out []neighEntry
	for _, line := range strings.Split(text, "\n") {
		f := strings.Fields(line)
		if len(f) < 4 || f[0] == "IP" {
			continue
		}
		flags, err := strconv.ParseInt(strings.TrimPrefix(f[2], "0x"), 16, 64)
		ip, ipErr := netip.ParseAddr(f[0])
		mac := deviceMAC(f[3])
		if err != nil || flags&0x2 == 0 || ipErr != nil || mac == "" {
			continue
		}
		e := neighEntry{ip: ip, mac: mac}
		if len(f) >= 6 {
			e.dev = f[5]
		}
		out = append(out, e)
	}
	return out
}

func isUpperWord(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] < 'A' || s[i] > 'Z' {
			return false
		}
	}
	return s != ""
}

// nlbwRow is the byte counters of one device since nlbwmon's period began.
type nlbwRow struct{ rx, tx uint64 }

// parseNlbw reads `nlbw -c json -g mac`:
// {"columns":["mac","conns","rx_bytes",...],"data":[[...],...]}.
// Columns are found by name. rx is download and tx is upload. The second
// result is false when the text is not nlbwmon's answer.
func parseNlbw(text string) (map[string]nlbwRow, bool) {
	text = strings.TrimSpace(text)
	if text == "" {
		return nil, false
	}
	var v struct {
		Columns []string            `json:"columns"`
		Data    [][]json.RawMessage `json:"data"`
	}
	if json.Unmarshal([]byte(text), &v) != nil || len(v.Columns) == 0 {
		return nil, false
	}
	col := map[string]int{}
	for i, c := range v.Columns {
		col[c] = i
	}
	mi, ok1 := col["mac"]
	ri, ok2 := col["rx_bytes"]
	ti, ok3 := col["tx_bytes"]
	if !ok1 || !ok2 || !ok3 {
		return nil, false
	}
	out := map[string]nlbwRow{}
	for _, row := range v.Data {
		if mi >= len(row) || ri >= len(row) || ti >= len(row) {
			continue
		}
		var macText string
		if json.Unmarshal(row[mi], &macText) != nil {
			continue
		}
		mac := deviceMAC(macText)
		if mac == "" {
			continue
		}
		r := out[mac]
		r.rx += jsonUint(row[ri])
		r.tx += jsonUint(row[ti])
		out[mac] = r
	}
	return out, true
}

// jsonUint reads a JSON number as a byte count. Counters can be big, so the
// integer form is tried first.
func jsonUint(raw json.RawMessage) uint64 {
	s := strings.TrimSpace(string(raw))
	if n, err := strconv.ParseUint(s, 10, 64); err == nil {
		return n
	}
	if f, err := strconv.ParseFloat(s, 64); err == nil && f > 0 {
		return uint64(f)
	}
	return 0
}
