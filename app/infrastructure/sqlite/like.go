package sqlite

import "strings"

// LikeEscape declares the escape character LikeContains uses. Append it to every
// LIKE that takes a LikeContains argument: `nickname LIKE ?` + LikeEscape.
const LikeEscape = ` ESCAPE '\'`

// likeEscaper escapes LIKE's wildcards and the escape character itself.
var likeEscaper = strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`)

// LikeContains is the LIKE argument that matches s as a literal substring: a %
// or _ the user typed into a search box is searched for, not treated as a
// wildcard. Case is ignored (Unicode-aware — see registerConnFuncs).
func LikeContains(s string) string {
	return "%" + likeEscaper.Replace(s) + "%"
}
