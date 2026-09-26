// Package rpcutil holds small helpers shared by the agent feature packages:
// decoding params and turning Go errors into protocol errors.
package rpcutil

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"

	"github.com/j0x3n/x-console/backend/pkg/protocol"
)

// Decode unmarshals params into v. Empty params leave v unchanged.
func Decode(raw json.RawMessage, v any) error {
	if len(raw) == 0 || string(raw) == "null" {
		return nil
	}
	if err := json.Unmarshal(raw, v); err != nil {
		return BadParams("invalid params: %v", err)
	}
	return nil
}

// BadParams is a protocol.CodeBadParams error.
func BadParams(format string, args ...any) error {
	return &protocol.Error{Code: protocol.CodeBadParams, Message: fmt.Sprintf(format, args...)}
}

// Unsupported is a protocol.CodeUnsupported error.
func Unsupported(what string) error {
	return &protocol.Error{Code: protocol.CodeUnsupported, Message: what + " is not supported on this system"}
}

// Failed is a protocol.CodeFailed error.
func Failed(format string, args ...any) error {
	return &protocol.Error{Code: protocol.CodeFailed, Message: fmt.Sprintf(format, args...)}
}

// FromOS maps file system errors to protocol codes so the server can answer
// 404, 403 or 409 instead of 502.
func FromOS(err error) error {
	if err == nil {
		return nil
	}
	var pe *protocol.Error
	if errors.As(err, &pe) {
		return pe
	}
	msg := err.Error()
	switch {
	case errors.Is(err, fs.ErrNotExist):
		return &protocol.Error{Code: protocol.CodeNotFound, Message: msg}
	case errors.Is(err, fs.ErrPermission):
		return &protocol.Error{Code: protocol.CodePermission, Message: msg}
	case errors.Is(err, fs.ErrExist):
		return &protocol.Error{Code: protocol.CodeExists, Message: msg}
	case errors.Is(err, fs.ErrInvalid):
		return &protocol.Error{Code: protocol.CodeBadParams, Message: msg}
	}
	return &protocol.Error{Code: protocol.CodeFailed, Message: msg}
}
