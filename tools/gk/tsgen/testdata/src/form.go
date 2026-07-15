package fixtures

// Request-only DTO: type emitted, guard suppressed (noguard).
//
//gkts:assets/types/WidgetForm.ts WidgetForm noguard
type widgetFormDTO struct {
	Name string `json:"name"`
}

// The SAME TS type declared by a second Go struct with identical fields —
// dedupe emits one definition (the real repo does this for UserFormData).
//
//gkts:assets/types/WidgetForm.ts WidgetForm noguard
type widgetFormAliasDTO struct {
	Name string `json:"name"`
}
