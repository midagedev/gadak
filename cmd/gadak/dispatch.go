package main

// gadak's compound commands share one verb-table shape: a leading word
// names the verb, no args (or a leading flag) means "list", and what an
// unknown leading word means is the command's one real choice. This file
// is the single owner of that shape (GDK-1778) — pairing.go's comment
// named it back when there were two copies; config, views, dashboards,
// dashboards lib, and recipes grew four more. The verb tables themselves
// stay in their command files, next to the functions they route to.

import "strings"

// subcmd is one verb of a compound command: run gets the args after the
// verb (`gadak config set x y` calls configSet with ["x", "y"]).
type subcmd struct {
	name string
	run  func(args []string) error
}

// dispatchSub routes args through the table. Bare args — or a leading
// flag, `gadak pairing --json` — fall to "list", which every table here
// has. An unknown leading word is the one place the tables disagree:
// strict commands (config, pairing, recipes, dashboards lib) refuse it
// with the usage line, while showFallback commands (views, dashboards)
// keep the word in the args and hand the whole line to "show" —
// `gadak views ENG-1` is `gadak views show ENG-1`, because a bare word
// there is a view name far more often than a typo.
func dispatchSub(args []string, cmd, usage string, showFallback bool, subs ...subcmd) error {
	sub, rest := "list", args
	if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		found := false
		for _, s := range subs {
			if args[0] == s.name {
				sub, rest, found = s.name, args[1:], true
				break
			}
		}
		if !found {
			if showFallback {
				for _, s := range subs {
					if s.name == "show" {
						return s.run(args)
					}
				}
			}
			return usageError(cmd, usage)
		}
	}
	for _, s := range subs {
		if s.name == sub {
			return s.run(rest)
		}
	}
	// sub is "list" or a table verb by construction; this arm only covers
	// a table that registers neither — same defensive shape the per-command
	// switch's default arm used to hold.
	return usageError(cmd, usage)
}
