package contacts

import (
	"io"
	"strconv"
	"strings"
	"unicode"

	"github.com/emersion/go-vcard"

	"github.com/j0x3n/x-console/backend/internal/server/modules/contacts/api"
)

// person is one vCard reduced to what this module keeps.
type person struct {
	uid    string
	name   string
	note   string
	phones []string
	emails []string
	// events have ids starting with importedPrefix, so a later import can
	// replace them without touching the dates the user added by hand.
	events []event
}

const importedPrefix = "im-"

// parseVCards reads every card in r. Cards without a usable name are counted
// in bad and left out.
func parseVCards(r io.Reader) (people []person, bad int) {
	dec := vcard.NewDecoder(r)
	for {
		card, err := dec.Decode()
		if len(card) > 0 {
			if p, ok := personOf(card); ok {
				people = append(people, p)
			} else {
				bad++
			}
		}
		if err != nil {
			return people, bad
		}
	}
}

func personOf(card vcard.Card) (person, bool) {
	p := person{uid: strings.TrimSpace(card.Value(vcard.FieldUID)), note: strings.TrimSpace(card.Value(vcard.FieldNote))}
	p.name = cardName(card)
	if p.name == "" {
		return p, false
	}
	for _, f := range card[vcard.FieldTelephone] {
		if v := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(f.Value), "tel:")); v != "" {
			p.phones = append(p.phones, v)
		}
	}
	for _, f := range card[vcard.FieldEmail] {
		if v := strings.TrimSpace(f.Value); v != "" {
			p.emails = append(p.emails, v)
		}
	}
	if f := card.Get(vcard.FieldBirthday); f != nil {
		if d, ok := vcardDate(f); ok {
			p.events = append(p.events, event{ID: importedPrefix + "bday", Kind: api.ContactEventKindBirthday, Label: kindLabels[api.ContactEventKindBirthday], Date: d})
		}
	}
	if f := card.Get(vcard.FieldAnniversary); f != nil {
		if d, ok := vcardDate(f); ok {
			p.events = append(p.events, event{ID: importedPrefix + "ann", Kind: api.ContactEventKindAnniversary, Label: kindLabels[api.ContactEventKindAnniversary], Date: d})
		}
	}
	// Apple keeps other dates as X-ABDATE, with the name in a X-ABLABEL of the same group.
	for i, f := range card["X-ABDATE"] {
		d, ok := vcardDate(f)
		if !ok {
			continue
		}
		kind, label := api.ContactEventKindOther, kindLabels[api.ContactEventKindOther]
		if f.Group != "" {
			for _, l := range card["X-ABLABEL"] {
				if l.Group != f.Group {
					continue
				}
				switch v := strings.TrimSpace(l.Value); {
				case strings.EqualFold(v, "_$!<Anniversary>!$_"):
					kind, label = api.ContactEventKindAnniversary, kindLabels[api.ContactEventKindAnniversary]
				case strings.HasPrefix(v, "_$!<"):
					// 别的系统标签（Other 等）按“日子”
				case v != "":
					label = v
				}
			}
		}
		p.events = append(p.events, event{ID: importedPrefix + "d" + strconv.Itoa(i), Kind: kind, Label: label, Date: d})
	}
	return p, true
}

// cardName is the display name: FN, else the parts of N, else the company.
func cardName(card vcard.Card) string {
	if v := strings.TrimSpace(card.PreferredValue(vcard.FieldFormattedName)); v != "" {
		return v
	}
	if n := card.Name(); n != nil {
		family, given := strings.TrimSpace(n.FamilyName), strings.TrimSpace(n.GivenName)
		switch {
		case family == "" && given == "":
		case isCJK(family) && isCJK(given) || family == "" || given == "":
			return family + given
		default:
			return given + " " + family
		}
	}
	if v := strings.TrimSpace(strings.SplitN(card.Value(vcard.FieldOrganization), ";", 2)[0]); v != "" {
		return v
	}
	return strings.TrimSpace(card.Value(vcard.FieldNickname))
}

func isCJK(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if !unicode.Is(unicode.Han, r) && !unicode.Is(unicode.Hiragana, r) && !unicode.Is(unicode.Katakana, r) && !unicode.Is(unicode.Hangul, r) {
			return false
		}
	}
	return true
}

// vcardDate turns a vCard date into "YYYY-MM-DD", or "MM-DD" when the year is
// unknown. It reads 1990-05-12, 19900512, --0512 and --05-12. Apple writes a
// made-up year (often 1604) with X-APPLE-OMIT-YEAR when the person never gave
// one, and that year is dropped too.
func vcardDate(f *vcard.Field) (string, bool) {
	v := strings.TrimSpace(f.Value)
	if i := strings.IndexAny(v, "Tt "); i > 0 {
		v = v[:i]
	}
	var year, month, day string
	switch {
	case strings.HasPrefix(v, "--"):
		rest := strings.ReplaceAll(strings.TrimPrefix(v, "--"), "-", "")
		if len(rest) != 4 {
			return "", false
		}
		month, day = rest[:2], rest[2:]
	default:
		compact := strings.ReplaceAll(v, "-", "")
		if len(compact) != 8 {
			return "", false
		}
		year, month, day = compact[:4], compact[4:6], compact[6:]
	}
	if year != "" {
		y, err := strconv.Atoi(year)
		if err != nil {
			return "", false
		}
		if omit := f.Params.Get("X-APPLE-OMIT-YEAR"); omit != "" && omit == year || y < 1900 {
			year = ""
		}
	}
	out := month + "-" + day
	if year != "" {
		out = year + "-" + out
	}
	if _, _, _, ok := splitDate(out); !ok {
		return "", false
	}
	return out, true
}
