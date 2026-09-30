// Package logfollow defines the frames shared by local and remote log viewers.
package logfollow

import (
	"encoding/json"
	"unicode/utf8"
)

type Frame struct {
	Type   string `json:"type"`
	Offset int64  `json:"offset,omitempty"`
	Data   string `json:"data,omitempty"`
}

func Append(offset int64, data []byte) []byte {
	b, _ := json.Marshal(Frame{Type: "append", Offset: offset, Data: string(data)})
	return b
}

func Reset() []byte {
	b, _ := json.Marshal(Frame{Type: "reset"})
	return b
}

// UTF8Prefix is the length of data without a rune cut off at its end. JSON
// would turn half a rune into U+FFFD, so callers send only this prefix and
// read the rest again with the next chunk.
func UTF8Prefix(data []byte) int {
	i := 0
	for i < len(data) {
		if !utf8.FullRune(data[i:]) {
			break
		}
		_, size := utf8.DecodeRune(data[i:])
		i += size
	}
	return i
}
