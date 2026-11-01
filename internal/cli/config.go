package cli

import (
	"strconv"
	"strings"
	"time"

	"github.com/mansus-org/myself/internal/client"
	"github.com/mansus-org/myself/internal/store"
)

// Config is the resolved global configuration handed to every command.
type Config struct {
	File string
	JSON bool
	// URL is the server base URL; non-empty means remote mode (SPEC section 10).
	URL     string
	Token   string
	Timeout time.Duration
}

// Environment variables of the CLI.
const (
	// EnvFile overrides the default store path.
	EnvFile = store.EnvFile
	// EnvURL selects remote mode; --url overrides it.
	EnvURL = "MYSELF_URL"
	// EnvToken is the bearer token of remote mode; --token overrides it.
	EnvToken = "MYSELF_TOKEN"
	// EnvTimeout is the request timeout of remote mode: seconds, or a Go
	// duration such as 500ms. Default 10s.
	EnvTimeout = "MYSELF_TIMEOUT"
)

// Remote reports whether commands run against a server.
func (c *Config) Remote() bool { return c.URL != "" }

// resolveFile applies the precedence --file > MYSELF_FILE > default.
func resolveFile(flagValue string, flagSet bool) (string, error) {
	return store.ResolvePath(flagValue, flagSet)
}

// parseTimeout reads MYSELF_TIMEOUT: whole or fractional seconds, or a Go
// duration. Empty means the default.
func parseTimeout(s string) (time.Duration, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return client.DefaultTimeout, nil
	}
	if f, err := strconv.ParseFloat(s, 64); err == nil && f > 0 {
		return time.Duration(f * float64(time.Second)), nil
	}
	if d, err := time.ParseDuration(s); err == nil && d > 0 {
		return d, nil
	}
	return 0, Usagef("%s %q: want seconds (30) or a duration (500ms, 1m)", EnvTimeout, s)
}
