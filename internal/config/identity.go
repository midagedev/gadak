package config

import "os"

const (
	// Name is the CLI binary, desktop app, and user-facing product name.
	Name = "gadak"
	// DirName is the directory under $HOME that holds the default profile.
	DirName = ".gadak"
	// DBFile is the SQLite filename inside a profile directory.
	DBFile = "gadak.db"
	// EnvPrefix is prepended to HOME, PROFILE, TOKEN, SITE, EMAIL, PROJECTS.
	EnvPrefix = "GADAK_"

	// Legacy names from the 2026-08 rename (scry → gadak). Still accepted so
	// an existing install keeps working until the user next launches gadak.
	LegacyName      = "scry"
	LegacyDirName   = ".scry"
	LegacyDBFile    = "scry.db"
	LegacyEnvPrefix = "SCRY_"
)

// Env returns GADAK_<suffix>, then SCRY_<suffix> if the new name is unset.
func Env(suffix string) string {
	if v, ok := os.LookupEnv(EnvPrefix + suffix); ok {
		return v
	}
	return os.Getenv(LegacyEnvPrefix + suffix)
}
