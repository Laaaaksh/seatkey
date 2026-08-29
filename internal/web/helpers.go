package web

import (
	"html/template"
	"net/url"

	"github.com/Laaaaksh/seatkey/internal/crypto"
	"github.com/Laaaaksh/seatkey/internal/license"
)

var templateFuncs = template.FuncMap{}

func urlEscape(s string) string {
	return url.QueryEscape(s)
}

func encodedPublicKey(svc *license.Service) string {
	return crypto.EncodePublicKey(svc.PublicKey())
}
