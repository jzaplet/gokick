package shared_test

import (
	"testing"

	"gokick/app/domain/shared"
)

func TestMaskHeaderValue(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name, header, in, want string
	}{
		{
			"authorization keeps scheme",
			"Authorization",
			"Bearer eyJhbGciOi.J9",
			"Bearer " + shared.MaskedValue,
		},
		{
			"authorization basic keeps scheme",
			"Authorization",
			"Basic dXNlcjpwYXNz",
			"Basic " + shared.MaskedValue,
		},
		{
			"authorization no scheme fully masked",
			"Authorization",
			"opaque-token",
			shared.MaskedValue,
		},
		{
			"authorization case-insensitive",
			"AUTHORIZATION",
			"Bearer x",
			"Bearer " + shared.MaskedValue,
		},
		{
			"cookie fully masked (no partial leak on spaces)",
			"Cookie",
			"a=1; gk_session=1",
			shared.MaskedValue,
		},
		{
			"set-cookie fully masked",
			"Set-Cookie",
			"refresh_token=abc; HttpOnly",
			shared.MaskedValue,
		},
		{"x-api-key fully masked", "X-Api-Key", "secret-key", shared.MaskedValue},
		{"user-agent passes through", "User-Agent", "Mozilla/5.0", "Mozilla/5.0"},
		{"accept passes through", "Accept", "application/json", "application/json"},
		{"empty stays empty", "Authorization", "", ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			if got := shared.MaskHeaderValue(c.header, c.in); got != c.want {
				t.Fatalf("MaskHeaderValue(%q, %q) = %q, want %q", c.header, c.in, got, c.want)
			}
		})
	}
}

func TestMaskSensitiveHeaders(t *testing.T) {
	t.Parallel()
	in := map[string]string{
		"User-Agent":    "agent/1.0",
		"Authorization": "Bearer secret-jwt",
		"Cookie":        "gk_session=1",
	}
	got := shared.MaskSensitiveHeaders(in)
	if got["User-Agent"] != "agent/1.0" {
		t.Fatalf("non-sensitive header must pass through, got %q", got["User-Agent"])
	}
	if got["Authorization"] != "Bearer "+shared.MaskedValue {
		t.Fatalf("Authorization not masked, got %q", got["Authorization"])
	}
	if got["Cookie"] != shared.MaskedValue {
		t.Fatalf("Cookie not masked, got %q", got["Cookie"])
	}
	if shared.MaskSensitiveHeaders(nil) != nil {
		t.Fatal("nil headers must stay nil")
	}
}

func TestIsSensitiveLogKey(t *testing.T) {
	t.Parallel()
	sensitive := []string{
		"authorization",
		"password",
		"db_password",
		"access_token",
		"refresh_token",
		"api_key",
		"Cookie",
		"secret",
	}
	for _, k := range sensitive {
		if !shared.IsSensitiveLogKey(k) {
			t.Errorf("key %q must be sensitive", k)
		}
	}
	benign := []string{
		"method",
		"url",
		"user_id",
		"trace_id",
		"job_kind",
		"slot",
		"status",
		"duration_ms",
	}
	for _, k := range benign {
		if shared.IsSensitiveLogKey(k) {
			t.Errorf("key %q must NOT be sensitive", k)
		}
	}
}

func TestMaskLogValue(t *testing.T) {
	t.Parallel()
	if got := shared.MaskLogValue("authorization", "Bearer x"); got != shared.MaskedValue {
		t.Fatalf("sensitive key value must be masked, got %q", got)
	}
	if got := shared.MaskLogValue("method", "POST"); got != "POST" {
		t.Fatalf("benign key value must pass through, got %q", got)
	}
	if got := shared.MaskLogValue("password", ""); got != "" {
		t.Fatalf("empty value stays empty, got %q", got)
	}
}
