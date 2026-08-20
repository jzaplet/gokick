// Command gk is the gokick dev-tooling entrypoint: ONE module, one binary,
// subcommands per tool — so a new dev tool is a new subpackage + a case below,
// not a new go.mod to maintain (gokick-roadmap follow-up ②; today: tsgen,
// planned: boundary-check). It is a separate module on purpose: dev tools
// write to stdout and to files, which the main module's lint invariants (the
// single slog logging path) forbid.
package main

import (
	"fmt"
	"os"

	"gokick-gk/boundary"
	"gokick-gk/docpaths"
	"gokick-gk/errfields"
	"gokick-gk/i18n"
	"gokick-gk/tsgen"
)

func main() {
	if len(os.Args) < 2 {
		usage()
	}
	switch tool, args := os.Args[1], os.Args[2:]; tool {
	case "tsgen":
		tsgen.Run(args)
	case "boundary":
		boundary.Run(args)
	case "errfields":
		errfields.Run(args)
	case "i18n":
		i18n.Run(args)
	case "docpaths":
		docpaths.Run(args)
	default:
		fmt.Fprintf(os.Stderr, "gk: unknown tool %q\n\n", tool)
		usage()
	}
}

func usage() {
	fmt.Fprint(os.Stderr, `usage: gk <tool> [args]

tools:
  tsgen generate|check   TypeScript types from //gkts-annotated Go DTOs
  boundary               wire payloads must be //gkts-annotated named structs
  errfields              Go ValidationError fields ↔ FE *Errors keys parity
  i18n generate|check    msgkey constants from the translation catalogs + parity gate
  docpaths               every path/skill a doc cites must resolve
`)
	os.Exit(2)
}
