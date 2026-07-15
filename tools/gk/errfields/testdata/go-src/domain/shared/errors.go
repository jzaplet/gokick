// Package shared may declare ValidationError — the tripwire only fires
// OUTSIDE domain/shared.
package shared

type ValidationError struct{ Field, Message string }
