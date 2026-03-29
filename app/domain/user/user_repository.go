package user

import "context"

type Repository interface {
	Save(ctx context.Context, user *User) error
	Update(ctx context.Context, user *User) error
	Delete(ctx context.Context, id string) error
	FindByID(ctx context.Context, id string) (*User, error)
	FindByNickname(ctx context.Context, nickname string) (*User, error)
	FindAllActive(ctx context.Context) ([]User, error)
	FindAll(ctx context.Context) ([]User, error)
}
