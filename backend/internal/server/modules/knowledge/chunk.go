package knowledge

import (
	"encoding/binary"
	"math"
	"strings"
)

const (
	chunkSize   = 800 // runes
	chunkLap    = 100 // runes shared with the next passage
	maxChunks   = 25  // per document
	maxDocRunes = 20000
)

// chunkText cuts text into passages of about chunkSize runes. A passage ends
// at a line break or sentence end when there is one in its second half. The
// next passage starts chunkLap runes before the end of the previous one.
func chunkText(text string) []string {
	text = strings.TrimSpace(strings.ReplaceAll(text, "\r\n", "\n"))
	r := []rune(text)
	if len(r) > maxDocRunes {
		r = r[:maxDocRunes]
	}
	var out []string
	for pos := 0; pos < len(r) && len(out) < maxChunks; {
		end := min(pos+chunkSize, len(r))
		if end < len(r) {
			for i := end; i > pos+chunkSize/2; i-- {
				if isBreak(r[i-1]) {
					end = i
					break
				}
			}
		}
		if piece := strings.TrimSpace(string(r[pos:end])); piece != "" {
			out = append(out, piece)
		}
		if end >= len(r) {
			break
		}
		pos = max(end-chunkLap, pos+1)
	}
	return out
}

func isBreak(c rune) bool {
	switch c {
	case '\n', '。', '！', '？', '；', '.', '!', '?', ';':
		return true
	}
	return false
}

// normalize scales v to length 1, so that the dot product of two vectors is
// their cosine similarity. A zero vector stays as it is.
func normalize(v []float32) []float32 {
	var sum float64
	for _, x := range v {
		sum += float64(x) * float64(x)
	}
	if sum == 0 {
		return v
	}
	n := float32(1 / math.Sqrt(sum))
	out := make([]float32, len(v))
	for i, x := range v {
		out[i] = x * n
	}
	return out
}

func encodeVec(v []float32) []byte {
	b := make([]byte, 4*len(v))
	for i, x := range v {
		binary.LittleEndian.PutUint32(b[4*i:], math.Float32bits(x))
	}
	return b
}

func decodeVec(b []byte) []float32 {
	if len(b)%4 != 0 {
		return nil
	}
	v := make([]float32, len(b)/4)
	for i := range v {
		v[i] = math.Float32frombits(binary.LittleEndian.Uint32(b[4*i:]))
	}
	return v
}

func dot(a, b []float32) float32 {
	var s float32
	for i := range a {
		s += a[i] * b[i]
	}
	return s
}
