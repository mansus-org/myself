package cli

import (
	"os"
	"path/filepath"
)

// Config is the resolved global configuration handed to every command.
type Config struct {
	File string
	JSON bool
}

// EnvFile is the environment variable that overrides the default store path.
const EnvFile = "MYSELF_FILE"

// defaultFile returns ~/.myself/brain.jsonl.
func defaultFile() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".myself", "brain.jsonl"), nil
}

// resolveFile applies the precedence --file > MYSELF_FILE > default.
func resolveFile(flagValue string, flagSet bool) (string, error) {
	if flagSet {
		return flagValue, nil
	}
	if v := os.Getenv(EnvFile); v != "" {
		return v, nil
	}
	return defaultFile()
}
