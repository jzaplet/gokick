package user

import (
	"gokick/app/domain/shared"
	"gokick/app/domain/shared/msgkey"
)

// NewLang validates a raw UI-language preference into a shared.Lang — the
// value-object gate for the self-service profile language change.
func NewLang(s string) (shared.Lang, error) {
	lang, ok := shared.ParseLang(s)
	if !ok {
		return "", &shared.ValidationError{Field: "lang", Key: msgkey.UserLangInvalid}
	}
	return lang, nil
}
