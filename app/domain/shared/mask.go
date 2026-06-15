package shared

import "strings"

// MaskedValue replaces a redacted secret in anything bound for the error
// tracker. It keeps the surrounding shape visible (the header arrived, a value
// was present) while never egressing the secret itself.
const MaskedValue = "==MASKED=="

// sensitiveHeaderNames are header names whose value is a credential. Matched
// case-insensitively. Their value is masked before it can reach the error
// tracker; the header name itself is kept so you can still see it arrived.
var sensitiveHeaderNames = map[string]bool{
	"authorization":       true,
	"proxy-authorization": true,
	"cookie":              true,
	"set-cookie":          true,
	"x-api-key":           true,
	"x-auth-token":        true,
}

// sensitiveKeySubstrings flag a structured-log / tag / breadcrumb key whose
// value is likely a secret. Substring match (case-insensitive) so access_token,
// refresh_token, db_password, etc. are all caught. Over-masking a benign key is
// the safe failure mode — the log stream is an open seam to the error tracker,
// so we redact by default rather than enumerate every secret-bearing key.
var sensitiveKeySubstrings = []string{
	"authorization", "password", "passwd", "secret", "token",
	"cookie", "credential", "api_key", "apikey", "private_key", "bearer",
}

// MaskHeaderValue masks the value of a sensitive header. For the Authorization /
// Proxy-Authorization headers it preserves the leading scheme token so you can
// still tell what kind of credential arrived — "Authorization: Bearer <jwt>"
// becomes "Bearer ==MASKED==". Every other sensitive header (Cookie, X-Api-Key,
// …) is fully replaced (a Cookie can contain spaces, so partial preservation
// would leak). A non-sensitive header (User-Agent, Accept, …) passes through
// unchanged. Idempotent: masking an already-masked value is a no-op.
func MaskHeaderValue(name, value string) string {
	if value == "" {
		return ""
	}
	lower := strings.ToLower(strings.TrimSpace(name))
	if !sensitiveHeaderNames[lower] {
		return value
	}
	if lower == "authorization" || lower == "proxy-authorization" {
		// Keep the scheme ("Bearer", "Basic"), mask only the credential.
		if scheme, rest, found := strings.Cut(value, " "); found && rest != "" {
			return scheme + " " + MaskedValue
		}
	}
	return MaskedValue
}

// MaskSensitiveHeaders returns a copy of headers with every sensitive header
// value masked. Used as the last-line guard on event.Request.Headers, so even a
// header attached by a future SDK integration cannot egress a raw credential.
func MaskSensitiveHeaders(headers map[string]string) map[string]string {
	if headers == nil {
		return nil
	}
	out := make(map[string]string, len(headers))
	for k, v := range headers {
		out[k] = MaskHeaderValue(k, v)
	}
	return out
}

// IsSensitiveLogKey reports whether a structured-log / tag / breadcrumb key is
// likely to carry a secret as its value.
func IsSensitiveLogKey(key string) bool {
	lk := strings.ToLower(key)
	for _, s := range sensitiveKeySubstrings {
		if strings.Contains(lk, s) {
			return true
		}
	}
	return false
}

// MaskLogValue masks value when key looks sensitive, leaving everything else
// untouched. Applied where the log stream egresses to the error tracker (event
// tags and breadcrumb data) — that seam, unlike the request reconstruction, has
// no whitelist, so it is redacted by key heuristic instead.
func MaskLogValue(key, value string) string {
	if value == "" {
		return ""
	}
	if IsSensitiveLogKey(key) {
		return MaskedValue
	}
	return value
}
