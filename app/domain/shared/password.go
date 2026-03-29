package shared

type PasswordHasher interface {
	Hash(password string) (string, error)
	Verify(password, hash string) error
}
