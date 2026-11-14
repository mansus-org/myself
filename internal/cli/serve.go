package cli

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/mansus-org/myself"
	"github.com/mansus-org/myself/internal/server"
	"github.com/spf13/cobra"
)

// EnvAdminHash holds the sha256 (hex) of the admin token for `serve`.
const EnvAdminHash = "MYSELF_ADMIN_TOKEN_SHA256"

// warnClientSettings tells the user that --token/--url (and their env vars) are
// client settings that serve ignores.
func warnClientSettings(cmd *cobra.Command) {
	stderr := cmd.ErrOrStderr()
	if cmd.Flags().Changed("token") || strings.TrimSpace(os.Getenv(EnvToken)) != "" {
		Warnf(stderr, "--token / %s is the client token for --url; serve checks the admin token against %s (the sha256 of the token)", EnvToken, EnvAdminHash)
	}
	if cmd.Flags().Changed("url") || strings.TrimSpace(os.Getenv(EnvURL)) != "" {
		Warnf(stderr, "--url / %s is ignored; serve always serves this machine's store (the file, or the backend %s selects)", EnvURL, "MYSELF_NEO4J_URL")
	}
}

func newServeCmd(cfg *Config) *cobra.Command {
	var addr, site string
	cmd := &cobra.Command{
		Use:   "serve [--addr :8080] [--site DIR]",
		Short: "Serve the brain over HTTP: the REST API at /api/v1 and the landing page at /",
		Long: "Serve the store over HTTP (SPEC section 10). The admin token is checked against\n" +
			EnvAdminHash + " (hex sha256); without it the API answers 503.\n" +
			"Agent tokens are kept hashed in tokens.json next to the store.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			warnClientSettings(cmd)
			hash := strings.ToLower(strings.TrimSpace(os.Getenv(EnvAdminHash)))
			if b, err := hex.DecodeString(hash); hash != "" && (err != nil || len(b) != 32) {
				return Usagef("%s must be 64 hex characters (the sha256 of the admin token)", EnvAdminHash)
			}
			var siteFS fs.FS
			if cmd.Flags().Changed("site") {
				siteFS = os.DirFS(site)
			} else {
				sub, err := fs.Sub(myself.Site, "site")
				if err != nil {
					return StoreErr(err)
				}
				siteFS = sub
			}
			stderr := cmd.ErrOrStderr()
			if hash == "" {
				Warnf(stderr, "%s is not set; the API answers 503 to every request", EnvAdminHash)
			}
			key, err := server.NewLocalKey()
			if err != nil {
				return StoreErr(err)
			}
			// The backend, if the environment selects one, replaces the file. The
			// schema is ensured here rather than at first write so a misconfigured or
			// unreachable database fails at startup, when it is obvious, instead of
			// quietly on the first remember hours later.
			backend, err := backendFromEnv()
			if err != nil {
				return err
			}
			if backend != nil {
				if schema, ok := backend.(interface{ EnsureSchema() error }); ok {
					if err := schema.EnsureSchema(); err != nil {
						return StoreErr(err)
					}
				}
			}
			srv, err := server.New(server.Config{File: cfg.File, Store: backend, Site: siteFS, Skill: myself.Skill, AdminHash: hash, LocalKey: key, Log: stderr, Version: Version})
			if err != nil {
				return StoreErr(err)
			}
			ln, err := net.Listen("tcp", addr)
			if err != nil {
				return StoreErr(err)
			}
			// The record of this server is what lets `myself graph --web` on the
			// same machine ask it for a link instead of failing on a taken port.
			unrecord, err := localRecord(cfg.File, ln.Addr(), key)
			if err != nil {
				Warnf(stderr, "graph --web cannot reuse this server: %v", err)
			} else {
				defer unrecord()
			}
			serving := cfg.File
			if backend != nil {
				serving = fmt.Sprint(backend)
			}
			fmt.Fprintf(stderr, "serving %s on %s\n", serving, ln.Addr())
			ctx, stop := signal.NotifyContext(cmd.Context(), os.Interrupt, syscall.SIGTERM)
			defer stop()
			return runServer(ctx, srv, ln)
		},
	}
	cmd.Flags().StringVar(&addr, "addr", ":8080", "address to listen on")
	cmd.Flags().StringVar(&site, "site", "", "directory served at / (default: the embedded landing page)")
	return cmd
}

// runServer serves srv on ln until ctx is done, then shuts down cleanly.
//
// Shutdown only closes idle connections, and it counts a connection that has
// not sent a request yet (a browser's spare preconnect, an HTTP client's extra
// dial) as idle after 5 seconds. Those are closed at once instead, or Ctrl-C
// would hang for 5 seconds.
func runServer(ctx context.Context, srv http.Handler, ln net.Listener) error {
	var mu sync.Mutex
	fresh := map[net.Conn]struct{}{} // accepted, no request yet
	hs := &http.Server{
		Handler:           srv,
		ReadHeaderTimeout: 10 * time.Second,
		ConnState: func(c net.Conn, st http.ConnState) {
			mu.Lock()
			defer mu.Unlock()
			if st == http.StateNew {
				fresh[c] = struct{}{}
			} else {
				delete(fresh, c)
			}
		},
	}
	errc := make(chan error, 1)
	go func() { errc <- hs.Serve(ln) }()
	select {
	case err := <-errc:
		return StoreErr(err)
	case <-ctx.Done():
	}
	mu.Lock()
	for c := range fresh {
		c.Close()
	}
	mu.Unlock()
	shutdown, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := hs.Shutdown(shutdown); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return StoreErr(err)
	}
	return nil
}
