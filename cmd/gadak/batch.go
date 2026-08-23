package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"
)

// batchLineLimit is the maximum number of non-empty --batch JSON lines.
// It is an origin API courtesy (do not fire unbounded writes in one
// invocation), not a gadak database constraint.
const batchLineLimit = 50

// batchResult is one envelope row. TSV columns are key, ok, changed, error
// (header on the first line). --json is one object per line with the same
// fields. transition_id is dry-run only (resolved id; omitted otherwise).
//
// mirror_stale rides along for the same reason the single-key JSON carries
// it (GDK-740): the write landed and only the re-read failed, so ok is
// true, and a machine reader should not have to parse stderr to learn the
// row it will read back is the pre-write one. Omitted when false, so the
// common envelope is unchanged.
type batchResult struct {
	Key          string `json:"key"`
	OK           bool   `json:"ok"`
	Changed      bool   `json:"changed"`
	Error        string `json:"error"`
	TransitionID string `json:"transition_id,omitempty"`
	MirrorStale  bool   `json:"mirror_stale,omitempty"`
}

// rejectBatchFlag is the create.go --batch idiom: only `-` (stdin JSON
// lines), and `-m -` cannot share stdin.
func rejectBatchFlag(value, dashBody string) error {
	if value != "-" {
		return fmt.Errorf("--batch only accepts - (JSON lines on stdin)")
	}
	if dashBody == "-" {
		return fmt.Errorf("--batch - already reads stdin; -m - cannot be used together")
	}
	return nil
}

type batchFailedError struct {
	verb        string
	fail, total int
}

func (e *batchFailedError) Error() string {
	return fmt.Sprintf("%s --batch: %d of %d failed", e.verb, e.fail, e.total)
}

// runWriteBatch reads stdin JSON lines, refuses an empty stdin and a
// line-count over batchLineLimit before any apply, then tries every line
// (parse failures included) and prints one envelope row per input line in
// input order. Any failure makes the returned error non-nil after the
// envelopes are written.
func runWriteBatch(verb string, asJSON bool, apply func(raw string) batchResult) error {
	lines, err := readBatchLines()
	if err != nil {
		return err
	}
	if len(lines) == 0 {
		return usageError(verb, fmt.Sprintf("usage: gadak %s --batch -", verb))
	}
	results := make([]batchResult, len(lines))
	fails := 0
	for i, raw := range lines {
		results[i] = apply(raw)
		if !results[i].OK {
			fails++
		}
	}
	if err := emitBatchResults(asJSON, results); err != nil {
		return err
	}
	if fails > 0 {
		return &batchFailedError{verb: verb, fail: fails, total: len(lines)}
	}
	return nil
}

func readBatchLines() ([]string, error) {
	sc := bufio.NewScanner(os.Stdin)
	var lines []string
	n := 0
	for sc.Scan() {
		raw := strings.TrimSpace(sc.Text())
		if raw == "" {
			continue
		}
		n++
		if n <= batchLineLimit {
			lines = append(lines, raw)
		}
	}
	if err := sc.Err(); err != nil {
		return nil, fmt.Errorf("reading --batch stdin: %w", err)
	}
	if n > batchLineLimit {
		return nil, fmt.Errorf("--batch accepts at most %d lines (an origin API courtesy, not a gadak database limit); got %d", batchLineLimit, n)
	}
	return lines, nil
}

func emitBatchResults(asJSON bool, results []batchResult) error {
	if asJSON {
		enc := json.NewEncoder(os.Stdout)
		for _, r := range results {
			if err := enc.Encode(r); err != nil {
				return err
			}
		}
		return nil
	}
	fmt.Fprint(os.Stdout, "key\tok\tchanged\terror\n")
	for _, r := range results {
		fmt.Fprintf(os.Stdout, "%s\t%s\t%s\t%s\n",
			r.Key, strconv.FormatBool(r.OK), strconv.FormatBool(r.Changed), r.Error)
	}
	return nil
}

func batchOK(key string, changed bool) batchResult {
	return batchResult{Key: key, OK: true, Changed: changed}
}

// batchStale is batchOK for a landed write whose mirror re-read failed.
func batchStale(key string, changed bool) batchResult {
	return batchResult{Key: key, OK: true, Changed: changed, MirrorStale: true}
}

func batchDryRun(key, transitionID string, changed bool) batchResult {
	return batchResult{Key: key, OK: true, Changed: changed, TransitionID: transitionID}
}

func batchErr(key string, changed bool, err error) batchResult {
	return batchResult{Key: key, OK: false, Changed: changed, Error: foldBatchError(err)}
}

func foldBatchError(err error) string {
	if err == nil {
		return ""
	}
	s := strings.ReplaceAll(err.Error(), "\n", " ")
	s = strings.ReplaceAll(s, "\r", " ")
	return strings.TrimSpace(s)
}

// parseBatchLine unmarshals one object, refuses unknown fields, and peeks
// key even when the line is a failure so the envelope can name it.
func parseBatchLine(raw string, accepted []string) (obj map[string]json.RawMessage, key string, err error) {
	obj, err = decodeBatchObject(raw, accepted)
	key = batchPeekKey(raw, obj)
	if err != nil {
		return obj, key, err
	}
	if key == "" {
		return obj, key, fmt.Errorf("missing key")
	}
	return obj, key, nil
}

func decodeBatchObject(raw string, accepted []string) (map[string]json.RawMessage, error) {
	dec := json.NewDecoder(strings.NewReader(raw))
	var obj map[string]json.RawMessage
	if err := dec.Decode(&obj); err != nil {
		return nil, fmt.Errorf("invalid JSON: %w", err)
	}
	if obj == nil {
		return nil, fmt.Errorf("invalid JSON: not an object")
	}
	allow := make(map[string]struct{}, len(accepted))
	for _, k := range accepted {
		allow[k] = struct{}{}
	}
	var unknown []string
	for k := range obj {
		if _, ok := allow[k]; !ok {
			unknown = append(unknown, k)
		}
	}
	if len(unknown) > 0 {
		sort.Strings(unknown)
		return obj, fmt.Errorf("unknown field %s — accepted: %s", strings.Join(unknown, ", "), strings.Join(accepted, ", "))
	}
	return obj, nil
}

func batchPeekKey(raw string, obj map[string]json.RawMessage) string {
	if obj != nil {
		if s, ok, err := jsonStringField(obj, "key"); err == nil && ok {
			return normalizeKey(s)
		}
	}
	var peek struct {
		Key string `json:"key"`
	}
	_ = json.Unmarshal([]byte(raw), &peek)
	return normalizeKey(peek.Key)
}

func jsonStringField(obj map[string]json.RawMessage, name string) (string, bool, error) {
	raw, ok := obj[name]
	if !ok {
		return "", false, nil
	}
	var s string
	if err := json.Unmarshal(raw, &s); err != nil {
		return "", true, fmt.Errorf("%s must be a string", name)
	}
	return s, true, nil
}

func jsonBoolField(obj map[string]json.RawMessage, name string) (bool, bool, error) {
	raw, ok := obj[name]
	if !ok {
		return false, false, nil
	}
	var v bool
	if err := json.Unmarshal(raw, &v); err != nil {
		return false, true, fmt.Errorf("%s must be a boolean", name)
	}
	return v, true, nil
}

func jsonStringArrayField(obj map[string]json.RawMessage, name string) ([]string, bool, error) {
	raw, ok := obj[name]
	if !ok {
		return nil, false, nil
	}
	var v []string
	if err := json.Unmarshal(raw, &v); err != nil {
		return nil, true, fmt.Errorf("%s must be an array of strings", name)
	}
	return v, true, nil
}

func jsonObjectField(obj map[string]json.RawMessage, name string) (map[string]json.RawMessage, bool, error) {
	raw, ok := obj[name]
	if !ok {
		return nil, false, nil
	}
	var v map[string]json.RawMessage
	if err := json.Unmarshal(raw, &v); err != nil {
		return nil, true, fmt.Errorf("%s must be an object", name)
	}
	if v == nil {
		v = map[string]json.RawMessage{}
	}
	return v, true, nil
}

func jsonAnyObjectField(obj map[string]json.RawMessage, name string) (map[string]any, bool, error) {
	raw, ok := obj[name]
	if !ok {
		return nil, false, nil
	}
	var v map[string]any
	if err := json.Unmarshal(raw, &v); err != nil {
		return nil, true, fmt.Errorf("%s must be an object", name)
	}
	if v == nil {
		v = map[string]any{}
	}
	return v, true, nil
}
