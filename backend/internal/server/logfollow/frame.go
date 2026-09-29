// Package logfollow defines the frames shared by local and remote log viewers.
package logfollow

import "encoding/json"

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
