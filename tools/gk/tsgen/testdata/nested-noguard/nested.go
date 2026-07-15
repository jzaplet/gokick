package fixtures

//gkts:assets/types/Outer.ts Outer
type outerDTO struct {
	Inner innerDTO `json:"inner"`
}

//gkts:assets/types/Inner.ts Inner noguard
type innerDTO struct {
	ID string `json:"id"`
}
