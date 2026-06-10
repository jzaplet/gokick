package user

import (
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"gokick/app/domain/shared"
)

// TestNewUser_SetsDefaults closes domain-02: NewUser must mint a fresh UUID
// for ID, default Active to true, and stamp CreatedAt/UpdatedAt with the
// current time. CreatedAt and UpdatedAt come from two separate time.Now()
// calls, so we only assert each falls within the [before, after] window
// rather than that they are equal.
func TestNewUser_SetsDefaults(t *testing.T) {
	before := time.Now()
	u := NewUser(Nickname("bob"), "hash", Email("bob@example.com"), Role("user"))
	after := time.Now()

	if _, err := uuid.Parse(u.ID); err != nil {
		t.Fatalf("ID should be a parseable UUID, got %q: %v", u.ID, err)
	}
	if u.Active != true {
		t.Fatalf("Active: got %v want true", u.Active)
	}
	if u.CreatedAt.Before(before) || u.CreatedAt.After(after) {
		t.Fatalf("CreatedAt %v not within [%v, %v]", u.CreatedAt, before, after)
	}
	if u.UpdatedAt.Before(before) || u.UpdatedAt.After(after) {
		t.Fatalf("UpdatedAt %v not within [%v, %v]", u.UpdatedAt, before, after)
	}

	// Sanity: the value-object inputs are copied through verbatim.
	if u.Nickname != "bob" {
		t.Fatalf("Nickname: got %q want %q", u.Nickname, "bob")
	}
	if u.PasswordHash != "hash" {
		t.Fatalf("PasswordHash: got %q want %q", u.PasswordHash, "hash")
	}
	if u.Email != "bob@example.com" {
		t.Fatalf("Email: got %q want %q", u.Email, "bob@example.com")
	}
	if u.Role != "user" {
		t.Fatalf("Role: got %q want %q", u.Role, "user")
	}
}

// TestNewNickname_EmptyRequired closes domain-07 and guide-forms-fe-03:
// an empty nickname yields a *shared.ValidationError with Field "nickname"
// and Message "nickname is required".
func TestNewNickname_EmptyRequired(t *testing.T) {
	_, err := NewNickname("")

	var ve *shared.ValidationError
	if !errors.As(err, &ve) {
		t.Fatalf("expected *shared.ValidationError, got %T: %v", err, err)
	}
	if ve.Field != "nickname" {
		t.Fatalf("Field: got %q want %q", ve.Field, "nickname")
	}
	if ve.Message != "nickname is required" {
		t.Fatalf("Message: got %q want %q", ve.Message, "nickname is required")
	}
}

// TestNewNickname_TooLong closes domain-08 and guide-forms-fe-04: a nickname
// longer than 50 characters yields a *shared.ValidationError with Field
// "nickname" and Message "nickname must be at most 50 characters".
func TestNewNickname_TooLong(t *testing.T) {
	_, err := NewNickname(strings.Repeat("a", 51))

	var ve *shared.ValidationError
	if !errors.As(err, &ve) {
		t.Fatalf("expected *shared.ValidationError, got %T: %v", err, err)
	}
	if ve.Field != "nickname" {
		t.Fatalf("Field: got %q want %q", ve.Field, "nickname")
	}
	if ve.Message != "nickname must be at most 50 characters" {
		t.Fatalf("Message: got %q want %q", ve.Message, "nickname must be at most 50 characters")
	}
}

// TestNewNickname_AtLimitOK guards the boundary: exactly 50 characters is
// accepted, proving the rejection above is the >50 branch, not >=50.
func TestNewNickname_AtLimitOK(t *testing.T) {
	in := strings.Repeat("a", 50)
	n, err := NewNickname(in)
	if err != nil {
		t.Fatalf("50-char nickname should be accepted, got: %v", err)
	}
	if string(n) != in {
		t.Fatalf("Nickname: got %q want %q", string(n), in)
	}
}

// TestNewRole_Invalid closes domain-11: any value other than "admin"/"user"
// yields a *shared.ValidationError with Field "role" and Message "invalid role".
func TestNewRole_Invalid(t *testing.T) {
	_, err := NewRole("superadmin")

	var ve *shared.ValidationError
	if !errors.As(err, &ve) {
		t.Fatalf("expected *shared.ValidationError, got %T: %v", err, err)
	}
	if ve.Field != "role" {
		t.Fatalf("Field: got %q want %q", ve.Field, "role")
	}
	if ve.Message != "invalid role" {
		t.Fatalf("Message: got %q want %q", ve.Message, "invalid role")
	}
}

// TestNewRole_AcceptsAdminAndUser guards that the only two allowed values are
// admitted, so the rejection above is genuinely the default branch.
func TestNewRole_AcceptsAdminAndUser(t *testing.T) {
	for _, want := range []string{"admin", "user"} {
		r, err := NewRole(want)
		if err != nil {
			t.Fatalf("role %q should be accepted, got: %v", want, err)
		}
		if string(r) != want {
			t.Fatalf("Role: got %q want %q", string(r), want)
		}
	}
}

// TestNewPassword_TooLong closes domain-19: a password longer than 128
// characters yields a *shared.ValidationError with Field "password" and
// Message "password must be at most 128 characters".
func TestNewPassword_TooLong(t *testing.T) {
	_, err := NewPassword(strings.Repeat("a", 129))

	var ve *shared.ValidationError
	if !errors.As(err, &ve) {
		t.Fatalf("expected *shared.ValidationError, got %T: %v", err, err)
	}
	if ve.Field != "password" {
		t.Fatalf("Field: got %q want %q", ve.Field, "password")
	}
	if ve.Message != "password must be at most 128 characters" {
		t.Fatalf("Message: got %q want %q", ve.Message, "password must be at most 128 characters")
	}
}

// TestNewPassword_AtLimitOK guards the boundary: exactly 128 characters is
// accepted, proving the rejection above is the >128 branch.
func TestNewPassword_AtLimitOK(t *testing.T) {
	in := strings.Repeat("a", 128)
	p, err := NewPassword(in)
	if err != nil {
		t.Fatalf("128-char password should be accepted, got: %v", err)
	}
	if string(p) != in {
		t.Fatalf("Password length: got %d want 128", len(string(p)))
	}
}

// TestNewEmail_TooLong closes domain-14: a non-empty email longer than 254
// characters yields a *shared.ValidationError with Field "email" and Message
// "email must be at most 254 characters". The length check runs before the
// "@" check, so a 255-char string with no "@" still hits this branch.
func TestNewEmail_TooLong(t *testing.T) {
	_, err := NewEmail(strings.Repeat("a", 255))

	var ve *shared.ValidationError
	if !errors.As(err, &ve) {
		t.Fatalf("expected *shared.ValidationError, got %T: %v", err, err)
	}
	if ve.Field != "email" {
		t.Fatalf("Field: got %q want %q", ve.Field, "email")
	}
	if ve.Message != "email must be at most 254 characters" {
		t.Fatalf("Message: got %q want %q", ve.Message, "email must be at most 254 characters")
	}
}

// TestUserCreated_Shape closes domain-51: UserCreated exposes UserID,
// Nickname, Email, Role as strings plus a Timestamp; EventName() returns
// "user.created" and OccurredAt() returns the Timestamp.
func TestUserCreated_Shape(t *testing.T) {
	now := time.Now()
	ev := UserCreated{
		UserID:    "id-1",
		Nickname:  "bob",
		Email:     "bob@example.com",
		Role:      "user",
		Timestamp: now,
	}

	if ev.EventName() != "user.created" {
		t.Fatalf("EventName: got %q want %q", ev.EventName(), "user.created")
	}
	if !ev.OccurredAt().Equal(ev.Timestamp) {
		t.Fatalf("OccurredAt: got %v want %v", ev.OccurredAt(), ev.Timestamp)
	}
	if ev.UserID != "id-1" || ev.Nickname != "bob" || ev.Email != "bob@example.com" ||
		ev.Role != "user" {
		t.Fatalf("fields not preserved: %+v", ev)
	}

	// UserCreated satisfies shared.DomainEvent.
	var _ shared.DomainEvent = ev
}

// TestUserCreated_OnlyPrimitiveFields closes overview-37 and domain-57: a
// domain event must carry only primitive values (string/bool/numeric) or
// time.Time — never a pointer, slice, map, or a nested struct that could be
// a whole entity / value object. This fails the day someone adds an entity
// field to a domain event.
func TestUserCreated_OnlyPrimitiveFields(t *testing.T) {
	timeType := reflect.TypeOf(time.Time{})
	rt := reflect.TypeOf(UserCreated{})

	for i := 0; i < rt.NumField(); i++ {
		f := rt.Field(i)
		if f.Type == timeType {
			continue
		}
		switch f.Type.Kind() {
		case reflect.String,
			reflect.Bool,
			reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
			reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64,
			reflect.Float32, reflect.Float64:
			// primitive — ok
		default:
			t.Fatalf(
				"field %s has non-primitive type %s (kind %s); domain events must carry only primitives or time.Time",
				f.Name,
				f.Type,
				f.Type.Kind(),
			)
		}
	}
}
