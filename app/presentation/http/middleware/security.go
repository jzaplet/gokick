package middleware

import (
	"net/http"
	"strings"
)

// SecurityHeadersMiddleware emits a baseline of security response headers
// targeting an A+ rating on securityheaders.com. The middleware is safe by
// default — restrictive CSP, no framing, sniff-protection, conservative
// referrer policy and a wide deny on Permissions-Policy features.
//
// HSTS is only emitted when hstsEnabled is true (typically gated on
// production HTTPS — sending HSTS over HTTP is harmless to spec-compliant
// clients but signals misconfiguration).
//
// CSP allows 'self' for scripts/connects and 'self' + inline for styles
// (Vue's scoped style injection requires inline styles). Adjust the
// directives in this file when adding external script hosts, CDNs or
// embedded media — security headers are intentionally local, not env-driven.
func SecurityHeadersMiddleware(hstsEnabled bool) func(http.Handler) http.Handler {
	csp := strings.Join([]string{
		"default-src 'self'",
		"script-src 'self'",
		"style-src 'self' 'unsafe-inline'",
		"img-src 'self' data:",
		"font-src 'self' data:",
		"connect-src 'self'",
		"object-src 'none'",
		"base-uri 'self'",
		"form-action 'self'",
		"frame-ancestors 'none'",
	}, "; ")

	permissions := strings.Join([]string{
		"accelerometer=()",
		"autoplay=()",
		"camera=()",
		"display-capture=()",
		"encrypted-media=()",
		"fullscreen=()",
		"geolocation=()",
		"gyroscope=()",
		"magnetometer=()",
		"microphone=()",
		"midi=()",
		"payment=()",
		"picture-in-picture=()",
		"publickey-credentials-get=()",
		"sync-xhr=()",
		"usb=()",
		"xr-spatial-tracking=()",
	}, ", ")

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			h := w.Header()

			h.Set("X-Content-Type-Options", "nosniff")
			h.Set("X-Frame-Options", "DENY")
			h.Set("Referrer-Policy", "strict-origin-when-cross-origin")
			h.Set("Content-Security-Policy", csp)
			h.Set("Permissions-Policy", permissions)
			h.Set("Cross-Origin-Opener-Policy", "same-origin")
			h.Set("Cross-Origin-Resource-Policy", "same-origin")

			if hstsEnabled {
				h.Set("Strict-Transport-Security", "max-age=31536000; includeSubDomains; preload")
			}

			next.ServeHTTP(w, r)
		})
	}
}
