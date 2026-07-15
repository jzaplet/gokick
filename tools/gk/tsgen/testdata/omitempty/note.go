// Package fixtures holds an omitempty DTO: with guards enabled this needs the
// `optional` helper from guards.ts — absent (optionalReady=false) the
// generator must refuse loudly; present (true) it renders optional(...).
package fixtures

//gkts:assets/types/Note.ts Note
type noteDTO struct {
	Text string `json:"text"`
	Hint string `json:"hint,omitempty"`
}
