package web

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/Laaaaksh/seatkey/internal/license"
	"github.com/Laaaaksh/seatkey/internal/store"
)

func (s *Server) handleProductsList(w http.ResponseWriter, r *http.Request) {
	products, err := s.store.ListProducts()
	if err != nil {
		s.serverError(w, err)
		return
	}
	s.render(w, "products", map[string]any{"Products": products})
}

func (s *Server) handleProductCreate(w http.ResponseWriter, r *http.Request) {
	name := r.FormValue("name")
	if name == "" {
		http.Redirect(w, r, "/products", http.StatusSeeOther)
		return
	}
	p, err := s.store.CreateProduct(name)
	if err != nil {
		s.serverError(w, err)
		return
	}
	http.Redirect(w, r, "/products/"+p.ID, http.StatusSeeOther)
}

type licenseView struct {
	store.License
	StatusText  string
	DeviceCount int
	ExpiresText string
	CreatedText string
}

func (s *Server) toLicenseView(l store.License) (licenseView, error) {
	count, err := s.store.ActiveDeviceCount(l.ID)
	if err != nil {
		return licenseView{}, err
	}
	expires := "never"
	if l.ExpiresAt != nil {
		expires = l.ExpiresAt.Format("2006-01-02")
	}
	return licenseView{
		License:     l,
		StatusText:  l.Status(time.Now()),
		DeviceCount: count,
		ExpiresText: expires,
		CreatedText: l.CreatedAt.Format("2006-01-02"),
	}, nil
}

func (s *Server) handleProductDetail(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	product, err := s.store.GetProduct(id)
	if errors.Is(err, store.ErrNotFound) {
		http.NotFound(w, r)
		return
	}
	if err != nil {
		s.serverError(w, err)
		return
	}

	licenses, err := s.store.ListLicenses(id)
	if err != nil {
		s.serverError(w, err)
		return
	}
	var views []licenseView
	for _, l := range licenses {
		v, err := s.toLicenseView(l)
		if err != nil {
			s.serverError(w, err)
			return
		}
		views = append(views, v)
	}

	s.render(w, "product", map[string]any{
		"Product":  product,
		"Licenses": views,
	})
}

func (s *Server) handleLicenseCreate(w http.ResponseWriter, r *http.Request) {
	productID := r.PathValue("id")
	if _, err := s.store.GetProduct(productID); errors.Is(err, store.ErrNotFound) {
		http.NotFound(w, r)
		return
	} else if err != nil {
		s.serverError(w, err)
		return
	}

	maxDevices, err := strconv.Atoi(r.FormValue("max_devices"))
	if err != nil || maxDevices < 1 {
		maxDevices = 1
	}

	var expiresAt *time.Time
	if raw := r.FormValue("expires_at"); raw != "" {
		if t, err := time.Parse("2006-01-02", raw); err == nil {
			end := t.Add(24 * time.Hour)
			expiresAt = &end
		}
	}

	l, err := s.store.CreateLicense(store.CreateLicenseParams{
		ProductID:     productID,
		CustomerName:  r.FormValue("customer_name"),
		CustomerEmail: r.FormValue("customer_email"),
		MaxDevices:    maxDevices,
		ExpiresAt:     expiresAt,
	})
	if err != nil {
		s.serverError(w, err)
		return
	}
	http.Redirect(w, r, "/licenses/"+l.ID, http.StatusSeeOther)
}

func (s *Server) handleLicenseDetail(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	l, err := s.store.GetLicense(id)
	if errors.Is(err, store.ErrNotFound) {
		http.NotFound(w, r)
		return
	}
	if err != nil {
		s.serverError(w, err)
		return
	}

	devices, err := s.store.ListDevices(id)
	if err != nil {
		s.serverError(w, err)
		return
	}

	view, err := s.toLicenseView(l)
	if err != nil {
		s.serverError(w, err)
		return
	}

	data := map[string]any{
		"License": view,
		"Devices": devices,
	}
	if raw := r.URL.Query().Get("offline_result"); raw != "" {
		data["OfflineResult"] = raw
	}
	s.render(w, "license", data)
}

func (s *Server) handleLicenseRevoke(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if err := s.store.RevokeLicense(id); err != nil && !errors.Is(err, store.ErrNotFound) {
		s.serverError(w, err)
		return
	}
	http.Redirect(w, r, "/licenses/"+id, http.StatusSeeOther)
}

func (s *Server) handleDeviceDeactivate(w http.ResponseWriter, r *http.Request) {
	licenseID := r.PathValue("id")
	deviceID := r.PathValue("deviceID")
	l, err := s.store.GetLicense(licenseID)
	if errors.Is(err, store.ErrNotFound) {
		http.NotFound(w, r)
		return
	}
	if err != nil {
		s.serverError(w, err)
		return
	}
	if err := s.licenses.Deactivate(l.Key, deviceID); err != nil && !errors.Is(err, license.ErrNotFound) {
		s.serverError(w, err)
		return
	}
	http.Redirect(w, r, "/licenses/"+licenseID, http.StatusSeeOther)
}

// handleLicenseOfflineActivate is the admin side of the offline-activation
// flow: a customer on an air-gapped machine runs the demo CLI's
// `offline-request` command and sends the admin the resulting JSON; the
// admin pastes it here and gets back a signed activation file to send back.
func (s *Server) handleLicenseOfflineActivate(w http.ResponseWriter, r *http.Request) {
	licenseID := r.PathValue("id")
	l, err := s.store.GetLicense(licenseID)
	if errors.Is(err, store.ErrNotFound) {
		http.NotFound(w, r)
		return
	}
	if err != nil {
		s.serverError(w, err)
		return
	}

	var req struct {
		DeviceID   string `json:"device_id"`
		DeviceName string `json:"device_name"`
	}
	if err := json.Unmarshal([]byte(r.FormValue("request_json")), &req); err != nil || req.DeviceID == "" {
		http.Redirect(w, r, "/licenses/"+licenseID+"?offline_result="+urlEscape("invalid request file: could not find a device_id field"), http.StatusSeeOther)
		return
	}

	env, err := s.licenses.OfflineActivate(l.Key, req.DeviceID, req.DeviceName)
	if err != nil {
		msg := "could not issue offline activation"
		if errors.Is(err, license.ErrDeviceLimit) {
			msg = "device limit reached for this license"
		} else if errors.Is(err, license.ErrLicenseStatus) {
			msg = "license is not active"
		}
		http.Redirect(w, r, "/licenses/"+licenseID+"?offline_result="+urlEscape(msg), http.StatusSeeOther)
		return
	}

	out, err := json.MarshalIndent(env, "", "  ")
	if err != nil {
		s.serverError(w, err)
		return
	}
	http.Redirect(w, r, "/licenses/"+licenseID+"?offline_result="+urlEscape(string(out)), http.StatusSeeOther)
}

func (s *Server) handleSettings(w http.ResponseWriter, r *http.Request) {
	s.renderSettings(w, r, "")
}

func (s *Server) renderSettings(w http.ResponseWriter, r *http.Request, errMsg string) {
	webhookURL, _, _ := s.store.GetSetting("webhook_url")
	webhookSecret, _, _ := s.store.GetSetting("webhook_secret")
	s.render(w, "settings", map[string]any{
		"WebhookURL":    webhookURL,
		"WebhookSecret": webhookSecret,
		"PublicKey":     encodedPublicKey(s.licenses),
		"Error":         errMsg,
	})
}

func (s *Server) handleWebhookSettings(w http.ResponseWriter, r *http.Request) {
	if err := s.store.SetSetting("webhook_url", r.FormValue("webhook_url")); err != nil {
		s.serverError(w, err)
		return
	}
	if err := s.store.SetSetting("webhook_secret", r.FormValue("webhook_secret")); err != nil {
		s.serverError(w, err)
		return
	}
	http.Redirect(w, r, "/settings", http.StatusSeeOther)
}
