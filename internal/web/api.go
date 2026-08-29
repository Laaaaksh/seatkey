package web

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/Laaaaksh/seatkey/internal/crypto"
	"github.com/Laaaaksh/seatkey/internal/license"
)

// activateRequest is shared by /v1/activate and /v1/validate: both key off
// the license key plus a client-chosen device identifier (a machine
// fingerprint, hardware ID, or similar - Seatkey does not prescribe how it is
// derived, only that the same device produces the same ID on every call).
type activateRequest struct {
	LicenseKey string `json:"license_key"`
	DeviceID   string `json:"device_id"`
	DeviceName string `json:"device_name"`
}

func decodeJSON[T any](r *http.Request) (T, error) {
	var v T
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	err := dec.Decode(&v)
	return v, err
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeAPIError(w http.ResponseWriter, status int, code, message string) {
	writeJSON(w, status, map[string]string{"error": code, "message": message})
}

// mapLicenseError translates internal/license sentinel errors to the
// (status, code, message) an API client should see.
func mapLicenseError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, license.ErrNotFound):
		writeAPIError(w, http.StatusNotFound, "not_found", "license key or device not found")
	case errors.Is(err, license.ErrDeviceLimit):
		writeAPIError(w, http.StatusConflict, "device_limit_reached", "this license has reached its device limit")
	case errors.Is(err, license.ErrLicenseStatus):
		writeAPIError(w, http.StatusForbidden, "license_not_active", err.Error())
	default:
		writeAPIError(w, http.StatusInternalServerError, "internal_error", "internal error")
	}
}

func (s *Server) handleAPIActivate(w http.ResponseWriter, r *http.Request) {
	req, err := decodeJSON[activateRequest](r)
	if err != nil {
		writeAPIError(w, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	if req.LicenseKey == "" || req.DeviceID == "" {
		writeAPIError(w, http.StatusBadRequest, "invalid_request", "license_key and device_id are required")
		return
	}

	env, err := s.licenses.Activate(req.LicenseKey, req.DeviceID, req.DeviceName)
	if err != nil {
		mapLicenseError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, env)
}

func (s *Server) handleAPIValidate(w http.ResponseWriter, r *http.Request) {
	req, err := decodeJSON[activateRequest](r)
	if err != nil {
		writeAPIError(w, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	if req.LicenseKey == "" || req.DeviceID == "" {
		writeAPIError(w, http.StatusBadRequest, "invalid_request", "license_key and device_id are required")
		return
	}

	env, err := s.licenses.Validate(req.LicenseKey, req.DeviceID)
	if err != nil {
		mapLicenseError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, env)
}

func (s *Server) handleAPIDeactivate(w http.ResponseWriter, r *http.Request) {
	req, err := decodeJSON[activateRequest](r)
	if err != nil {
		writeAPIError(w, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	if req.LicenseKey == "" || req.DeviceID == "" {
		writeAPIError(w, http.StatusBadRequest, "invalid_request", "license_key and device_id are required")
		return
	}

	if err := s.licenses.Deactivate(req.LicenseKey, req.DeviceID); err != nil {
		mapLicenseError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (s *Server) handleAPIPubkey(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{
		"public_key": crypto.EncodePublicKey(s.licenses.PublicKey()),
		"algorithm":  "ed25519",
	})
}
