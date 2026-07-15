// Package fixtures exercises every shape the generator maps: scalar kinds,
// arrays, nullable pointers, nested named types (cross-file guard + type
// imports), json-tag handling (rename, omit, no tag), unexported-field skip,
// noguard request DTOs and the identical-duplicate dedupe.
package fixtures

//gkts:assets/types/Widget.ts Widget
type widgetDTO struct {
	ID       string   `json:"id"`
	Count    int      `json:"count"`
	Ratio    float64  `json:"ratio"`
	Active   bool     `json:"active"`
	Tags     []string `json:"tags"`
	Note     *string  `json:"note"`
	Owner    ownerDTO `json:"owner"`
	Secret   string   `json:"-"`
	Plain    string
	internal string
}

// keep the unexported field referenced so the fixture stays vet-clean
var _ = widgetDTO{internal: ""}
