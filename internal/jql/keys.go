package jql

import (
	"fmt"
	"strings"
	"unicode"
)

// SplitKeys splits a --keys payload on commas or any unicode space, then
// uppercases, trims, drops empties, and de-dupes while keeping first-seen order.
func SplitKeys(raw string) []string {
	parts := strings.FieldsFunc(raw, func(r rune) bool {
		return r == ',' || unicode.IsSpace(r)
	})
	return mergeUniqueUpper(nil, parts)
}

// KeyLimitMessage is the compile / --keys error text. It always includes the
// actual count so a 501-key paste is diagnosable without a second run.
func KeyLimitMessage(n int) string {
	return fmt.Sprintf("key list has %d values; the limit is %d", n, MaxKeys)
}

// CheckKeyLimit returns an error when n is above MaxKeys.
func CheckKeyLimit(n int) error {
	if n > MaxKeys {
		return fmt.Errorf("%s", KeyLimitMessage(n))
	}
	return nil
}
