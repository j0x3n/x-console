// Package httpx holds the JSON helpers every handler uses, so all modules
// answer with the same error shape (api/common.yaml#/components/schemas/Error).
package httpx

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
)

// Error is an API error with an HTTP status and a stable machine code.
type Error struct {
	Status  int
	Code    string
	Message string
	Details map[string]any
}

func (e *Error) Error() string { return e.Code + ": " + e.Message }

// Common errors. Create others with NewError.
var (
	ErrNotFound           = &Error{Status: http.StatusNotFound, Code: "not_found", Message: "资源不存在"}
	ErrUnauthorized       = &Error{Status: http.StatusUnauthorized, Code: "unauthorized", Message: "需要登录"}
	ErrElevationRequired  = &Error{Status: http.StatusForbidden, Code: "elevation_required", Message: "需要再次验证两步验证码"}
	ErrForbidden          = &Error{Status: http.StatusForbidden, Code: "forbidden", Message: "没有权限"}
	ErrConflict           = &Error{Status: http.StatusConflict, Code: "conflict", Message: "状态冲突"}
	ErrTooManyRequests    = &Error{Status: http.StatusTooManyRequests, Code: "rate_limited", Message: "请求太频繁，请稍后再试"}
	ErrAgentOffline       = &Error{Status: http.StatusServiceUnavailable, Code: "agent_offline", Message: "代理不在线"}
	ErrIntegrationMissing = &Error{Status: http.StatusPreconditionFailed, Code: "integration_not_configured", Message: "集成还没有配置"}
)

// NewError builds an error with a custom status and code.
func NewError(status int, code, message string) *Error {
	return &Error{Status: status, Code: code, Message: message}
}

// Invalid is a 400 validation error.
func Invalid(message string) *Error {
	return &Error{Status: http.StatusBadRequest, Code: "validation_failed", Message: message}
}

// JSON writes v with the given status.
func JSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	if v != nil {
		_ = json.NewEncoder(w).Encode(v)
	}
}

// NoContent writes 204.
func NoContent(w http.ResponseWriter) { w.WriteHeader(http.StatusNoContent) }

// Fail writes err as an API error. Unknown errors become 500 and are logged.
func Fail(w http.ResponseWriter, r *http.Request, err error) {
	var apiErr *Error
	if !errors.As(err, &apiErr) {
		slog.ErrorContext(r.Context(), "request failed", "method", r.Method, "path", r.URL.Path, "err", err)
		apiErr = &Error{Status: http.StatusInternalServerError, Code: "internal", Message: "服务器内部错误"}
	}
	body := map[string]any{"code": apiErr.Code, "message": apiErr.Message}
	if apiErr.Details != nil {
		body["details"] = apiErr.Details
	}
	JSON(w, apiErr.Status, body)
}

// Decode reads a JSON body into v. Unknown fields are rejected.
func Decode(r *http.Request, v any) error {
	dec := json.NewDecoder(http.MaxBytesReader(nil, r.Body, 1<<20))
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		return Invalid("请求体格式不正确: " + err.Error())
	}
	return nil
}

// BadParam is the ErrorHandlerFunc for generated chi wrappers: it turns
// parameter binding errors into 400 responses with our error shape.
func BadParam(w http.ResponseWriter, r *http.Request, err error) {
	Fail(w, r, Invalid(err.Error()))
}
