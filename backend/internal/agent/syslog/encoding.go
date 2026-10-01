package syslog

import (
	"unicode/utf16"
	"unicode/utf8"

	"golang.org/x/text/encoding"
	"golang.org/x/text/encoding/charmap"
	"golang.org/x/text/encoding/japanese"
	"golang.org/x/text/encoding/korean"
	"golang.org/x/text/encoding/simplifiedchinese"
	"golang.org/x/text/encoding/traditionalchinese"
)

// toUTF8 turns command output into UTF-8. wevtutil on a Chinese Windows
// writes GBK (the console code page), and encoding/xml then fails with
// "invalid UTF-8". codePage is the Windows code page the output is in; 0
// means unknown.
func toUTF8(out []byte, codePage uint32) []byte {
	if len(out) >= 2 && out[0] == 0xff && out[1] == 0xfe {
		return utf16le(out[2:])
	}
	// UTF-16 without a byte order mark: "<" followed by a zero byte.
	if len(out) >= 2 && out[0] != 0 && out[1] == 0 {
		return utf16le(out)
	}
	if utf8.Valid(out) {
		return out
	}
	if enc := codePageEncoding(codePage); enc != nil {
		if b, err := enc.NewDecoder().Bytes(out); err == nil {
			return b
		}
	}
	// 不认识的代码页：把坏字节换成替换字符，至少别整页解析失败
	return []byte(string([]rune(string(out))))
}

func utf16le(b []byte) []byte {
	u := make([]uint16, len(b)/2)
	for i := range u {
		u[i] = uint16(b[2*i]) | uint16(b[2*i+1])<<8
	}
	return []byte(string(utf16.Decode(u)))
}

// codePageEncoding maps the common Windows code pages. 936 (simplified
// Chinese) uses GB18030, a superset of GBK.
func codePageEncoding(cp uint32) encoding.Encoding {
	switch cp {
	case 936:
		return simplifiedchinese.GB18030
	case 950:
		return traditionalchinese.Big5
	case 932:
		return japanese.ShiftJIS
	case 949:
		return korean.EUCKR
	case 437:
		return charmap.CodePage437
	case 850:
		return charmap.CodePage850
	case 1252:
		return charmap.Windows1252
	case 1251:
		return charmap.Windows1251
	}
	return nil
}
