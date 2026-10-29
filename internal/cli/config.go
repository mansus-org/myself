package cli

import "github.com/mansus-org/myself/internal/store"

// Config is the resolved global configuration handed to every command.
type Config struct {
	File string
	JSON bool
}

// EnvFile is the environment variable that overrides the default store path.
const EnvFile = store.EnvFile

// resolveFile applies the precedence --file > MYSELF_FILE > default.
func resolveFile(flagValue string, flagSet bool) (string, error) {
	return store.ResolvePath(flagValue, flagSet)
}
