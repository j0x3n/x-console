package core

import (
	"net/http"

	"github.com/j0x3n/x-console/backend/internal/server/core/api"
	"github.com/j0x3n/x-console/backend/internal/server/httpx"
)

func (h *Handlers) SkipSetupTotp(w http.ResponseWriter, r *http.Request) {
	var body api.SkipSetupTotpJSONBody
	if err := httpx.Decode(r, &body); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	if err := h.Auth.SkipSetupTOTP(r.Context(), w, r, body.Password); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.NoContent(w)
}

func (h *Handlers) EnrollTotp(w http.ResponseWriter, r *http.Request) {
	secret, url, err := h.Auth.EnrollTOTP(r.Context())
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, api.TotpEnrollment{Secret: secret, OtpauthUrl: url})
}

func (h *Handlers) ConfirmTotp(w http.ResponseWriter, r *http.Request) {
	var body api.TotpCode
	if err := httpx.Decode(r, &body); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	if err := h.Auth.ConfirmTOTP(r.Context(), body.Code); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.NoContent(w)
}

func (h *Handlers) DisableTotp(w http.ResponseWriter, r *http.Request) {
	var body api.DisableTotpJSONBody
	if err := httpx.Decode(r, &body); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	if err := h.Auth.DisableTOTP(r.Context(), body.Password, body.Code); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.NoContent(w)
}

func (h *Handlers) ChangePassword(w http.ResponseWriter, r *http.Request) {
	var body api.ChangePasswordJSONBody
	if err := httpx.Decode(r, &body); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	if err := h.Auth.ChangePassword(r.Context(), body.OldPassword, body.NewPassword); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.NoContent(w)
}

// B48：敏感操作二次验证的方式。

func (h *Handlers) GetElevationMode(w http.ResponseWriter, r *http.Request) {
	httpx.JSON(w, http.StatusOK, api.ElevationModeBody{Mode: api.ElevationModeBodyMode(h.Auth.ElevationMode(r.Context()))})
}

func (h *Handlers) SetElevationMode(w http.ResponseWriter, r *http.Request) {
	var body api.ElevationModeBody
	if err := httpx.Decode(r, &body); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	if err := h.Auth.SetElevationMode(r.Context(), string(body.Mode)); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, body)
}
