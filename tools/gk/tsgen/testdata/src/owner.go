package fixtures

//gkts:assets/types/Owner.ts Owner
type ownerDTO struct {
	Name  string     `json:"name"`
	Peers []ownerDTO `json:"peers"`
}
