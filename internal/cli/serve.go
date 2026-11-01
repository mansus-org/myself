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
		Warnf(stderr, "--url / %s is ignored; serve always uses the local store", EnvURL)
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
			srv, err := server.New(server.Config{File: cfg.File, Site: siteFS, Skill: myself.Skill, AdminHash: hash, Log: stderr, Version: Version})
			if err != nil {
				return StoreErr(err)
			}
			ln, err := net.Listen("tcp", addr)
			if err != nil {
				return StoreErr(err)
			}
			fmt.Fprintf(stderr, "serving %s on %s\n", cfg.File, ln.Addr())

			hs := &http.Server{Handler: srv, ReadHeaderTimeout: 10 * time.Second}
			ctx, stop := signal.NotifyContext(cmd.Context(), os.Interrupt, syscall.SIGTERM)
			defer stop()
			errc := make(chan error, 1)
			go func() { errc <- hs.Serve(ln) }()
			select {
			case err := <-errc:
				return StoreErr(err)
			case <-ctx.Done():
			}
			shutdown, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			if err := hs.Shutdown(shutdown); err != nil && !errors.Is(err, http.ErrServerClosed) {
				return StoreErr(err)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&addr, "addr", ":8080", "address to listen on")
	cmd.Flags().StringVar(&site, "site", "", "directory served at / (default: the embedded landing page)")
	return cmd
}
