package postgres_test

import (
	"context"
	"net/url"
	"strings"
	"testing"

	"gokick/app/infrastructure/postgres"
	"gokick/app/internal/testfx/pgfx"

	"github.com/jmoiron/sqlx"
)

// The gokick roles pass the startup check.
func TestVerifyRoles_AcceptsTheGokickRoles(t *testing.T) {
	_, mgr := migrated(t)
	if err := mgr.VerifyRoles(context.Background()); err != nil {
		t.Fatalf("VerifyRoles: %v", err)
	}
}

// Every DSN mix-up that would quietly defeat row-level security is refused —
// the owner, a superuser or the BYPASSRLS role on the tenant plane, and a role
// without BYPASSRLS (or a superuser) on the system plane.
func TestVerifyRoles_RefusesRolesThatDefeatTheTenantWall(t *testing.T) {
	db := pgfx.New(t)
	for _, tc := range []struct {
		name              string
		appURL, systemURL string
		want              string
	}{
		{"owner as the tenant plane", db.OwnerURL, db.SystemURL, "owns"},
		{"superuser as the tenant plane", db.AdminURL, db.SystemURL, "superuser"},
		{"system role as the tenant plane", db.SystemURL, db.SystemURL, "BYPASSRLS"},
		{"tenant role as the system plane", db.AppURL, db.AppURL, "lacks BYPASSRLS"},
		{"owner as the system plane", db.AppURL, db.OwnerURL, "lacks BYPASSRLS"},
		{"superuser as the system plane", db.AppURL, db.AdminURL, "superuser"},
	} {
		cfg := db.Config()
		cfg.DBURL, cfg.DBSystemURL = tc.appURL, tc.systemURL
		err := newManager(t, cfg).VerifyRoles(context.Background())
		if err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("%s: got %v, want an error mentioning %q", tc.name, err, tc.want)
		}
	}
}

// A role that can SET ROLE to a superuser or to a BYPASSRLS role is refused as if
// it were one — GRANT gokick_system TO gokick_app must not pass the check.
func TestVerifyRoles_RefusesMembershipOfAnElevatedRole(t *testing.T) {
	db := pgfx.New(t)
	admin := sqlx.MustConnect(postgres.DriverName, db.AdminURL)
	t.Cleanup(func() { _ = admin.Close() }) // after the role drops below (LIFO)
	adminURL, err := url.Parse(db.AdminURL)
	if err != nil {
		t.Fatal(err)
	}
	superuser := adminURL.User.Username()

	for _, tc := range []struct {
		name, options, want string
		asSystem            bool
	}{
		{"tenant plane in gokick_system", "NOBYPASSRLS IN ROLE gokick_system", "gokick_system", false},
		{"system plane in a superuser", "BYPASSRLS IN ROLE " + superuser, "superuser", true},
	} {
		dsn := tempRole(t, admin, db, tc.options)
		cfg := db.Config()
		if tc.asSystem {
			cfg.DBSystemURL = dsn
		} else {
			cfg.DBURL = dsn
		}
		err := newManager(t, cfg).VerifyRoles(context.Background())
		if err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("%s: got %v, want an error mentioning %q", tc.name, err, tc.want)
		}
	}
}

// tempRole creates a login role with options for this test only (roles are
// cluster-wide, so it is dropped again) and returns its DSN on db.
func tempRole(t *testing.T, admin *sqlx.DB, db *pgfx.DB, options string) string {
	t.Helper()
	name := "gokick_t_role_" + strings.ReplaceAll(newID()[24:], "-", "")
	if _, err := admin.Exec(
		"CREATE ROLE " + name + " LOGIN PASSWORD '" + name + "' " + options); err != nil {
		t.Fatalf("create role: %v", err)
	}
	t.Cleanup(func() {
		if _, err := admin.Exec("DROP ROLE IF EXISTS " + name); err != nil {
			t.Errorf("drop role %s: %v", name, err)
		}
	})
	u, err := url.Parse(db.AppURL)
	if err != nil {
		t.Fatal(err)
	}
	u.User = url.UserPassword(name, name)
	return u.String()
}
