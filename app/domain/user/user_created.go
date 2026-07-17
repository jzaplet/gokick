package user

import "time"

// UserCreated announces a new user. Primitives only (the domain-event rule), so
// a subscriber never holds an entity.
//
// TenantID is on the event because a subscriber must NOT reach for the tenant in
// ctx. Events dispatch after commit in the COMMAND's context, whose active tenant
// is the ACTOR's — and for the platform create the actor is a superadmin sitting
// in the default tenant while the new user lives in the tenant they chose. Every
// other create path has actor-tenant == user-tenant, so "read it from ctx" would
// look correct everywhere it was tried and be wrong only on the platform plane —
// silently, after commit, where no request error can surface it. Carrying the
// tenant makes the event self-describing and the question moot.
type UserCreated struct {
	UserID    string
	Nickname  string
	Email     string
	Role      string
	TenantID  string
	Timestamp time.Time
}

func (e UserCreated) EventName() string     { return "user.created" }
func (e UserCreated) OccurredAt() time.Time { return e.Timestamp }
