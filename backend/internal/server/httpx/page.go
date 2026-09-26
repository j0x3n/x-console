package httpx

import (
	"encoding/base64"
	"strconv"
)

// Pagination uses opaque cursors. Most lists page by descending integer id:
// EncodeIDCursor(lastID) on the way out, DecodeIDCursor on the way in.

// MaxID is used as "before" when no cursor was given.
const MaxID int64 = 1<<63 - 1

// EncodeIDCursor turns the last returned id into a cursor.
func EncodeIDCursor(id int64) string {
	return base64.RawURLEncoding.EncodeToString([]byte(strconv.FormatInt(id, 10)))
}

// DecodeIDCursor returns MaxID for an empty cursor.
func DecodeIDCursor(cursor *string) (int64, error) {
	if cursor == nil || *cursor == "" {
		return MaxID, nil
	}
	raw, err := base64.RawURLEncoding.DecodeString(*cursor)
	if err != nil {
		return 0, Invalid("cursor 无效")
	}
	id, err := strconv.ParseInt(string(raw), 10, 64)
	if err != nil {
		return 0, Invalid("cursor 无效")
	}
	return id, nil
}

// Limit clamps a limit parameter to [1,200] with default 50.
func Limit(limit *int) int64 {
	if limit == nil || *limit <= 0 {
		return 50
	}
	if *limit > 200 {
		return 200
	}
	return int64(*limit)
}
