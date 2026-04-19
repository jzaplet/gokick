package user

import "time"

type UserCreated struct {
	UserID    string
	Nickname  string
	Email     string
	Role      string
	Timestamp time.Time
}

func (e UserCreated) EventName() string     { return "user.created" }
func (e UserCreated) OccurredAt() time.Time { return e.Timestamp }
