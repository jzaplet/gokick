package shared

import "context"

type Seeder interface {
	Seed(ctx context.Context) error
}
