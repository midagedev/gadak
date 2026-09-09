package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
)

// errDryRun is the sentinel a write path returns after emitDryRun printed its
// plan (GDK-1446). Callers treat it as success-with-no-write: the plan is the
// output, there is no key to refresh or print, and --batch moves to the next
// line. It never escapes to the shell — every command function folds it to nil.
var errDryRun = errors.New("dry run")

// emitDryRun prints one --dry-run plan (GDK-1446): a single compact JSON line
// on stdout carrying the exact request this write would send, and a stderr
// note that nothing was sent. keys are the issues the write touches — "key"
// for one, "keys" for link's pair, neither for create (the key does not exist
// yet). The plan is machine-readable on purpose: a Jira-workspace agent is
// meant to paste it to a person for approval.
func emitDryRun(verb string, request map[string]any, keys ...string) error {
	body := map[string]any{"dry_run": true, "verb": verb, "request": request}
	if len(keys) == 1 {
		body["key"] = keys[0]
	} else if len(keys) > 1 {
		body["keys"] = keys
	}
	line, err := json.Marshal(body)
	if err != nil {
		return err
	}
	fmt.Println(string(line))
	fmt.Fprintf(os.Stderr, "gadak %s --dry-run: nothing was sent\n", verb)
	return nil
}

// foldDryRun maps the errDryRun sentinel to command success — the plan is
// already on stdout and there is no key to refresh or print. Every verb that
// reaches its dry-run split inside mutate/withKeyWriteSession closes with
// this at the top level; the sentinel never escapes to the shell.
func foldDryRun(err error) error {
	if errors.Is(err, errDryRun) {
		return nil
	}
	return err
}
