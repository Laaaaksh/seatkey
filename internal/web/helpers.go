package web

import (
	"html/template"
	"net/url"

	"github.com/attentionfs/seatkey/internal/crypto"
	"github.com/attentionfs/seatkey/internal/license"
)

var templateFuncs = template.FuncMap{}

func urlEscape(s string) string {
	return url.QueryEscape(s)
}

func encodedPublicKey(svc *license.Service) string {
	return crypto.EncodePublicKey(svc.PublicKey())
}
