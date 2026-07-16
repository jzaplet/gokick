package vo

import "fixture/canon"

//gkts:out/tiers.ts Tier union
type Tier string

// Conversions, not literals: the exact pattern user.Role uses.
const (
	TierGold   = Tier(canon.TierGold)
	TierSilver = Tier(canon.TierSilver)
)

//gkts:out/Badge.ts Badge
type badge struct {
	Label string `json:"label"`
	Tier  Tier   `json:"tier"`
}
