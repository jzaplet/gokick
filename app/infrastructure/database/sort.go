package database

// SortCollation is the collation every user-facing ORDER BY on text goes through
// (the grids and lists). Every adapter provides it under this exact name — SQLite
// registers it on each connection, Postgres creates it as an ICU collation in a
// migration — so the same query sorts identically whichever backend runs it.
//
// It is applied in queries only, never in the schema: equality and UNIQUE stay
// binary (case-sensitive, exactly as before), and a SQLite file stays readable by
// tools that do not have the collation registered.
const SortCollation = "app_sort"

// SortLocale is the CLDR locale behind SortCollation: Czech alphabetical order —
// "ch" after "h", č after c, ř after r, š after s, ž after z, lowercase before
// uppercase. One constant, so a project generated from this template switches its
// sort language in one place.
const SortLocale = "cs-CZ"
