package canon

// The canonical values live HERE, in another package — the shape the real
// domain uses so the authorization ladder and the value object cannot drift
// apart. tsgen must follow the conversion in ../vo into this file.
const (
	TierGold   = "gold"
	TierSilver = "silver"
)
