package cli

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"runtime"
	"strconv"
	"syscall"
	"time"

	"github.com/mansus-org/myself"
	"github.com/mansus-org/myself/internal/brain"
	"github.com/mansus-org/myself/internal/server"
	"github.com/spf13/cobra"
)

// openURL opens a URL in the user's browser; a variable so tests can stub it.
var openURL = func(url string) error {
	name, ok := browserOpener()
	if !ok {
		return errors.New("no browser opener found (xdg-open, open)")
	}
	c := exec.Command(name, url)
	if err := c.Start(); err != nil {
		return err
	}
	go c.Wait()
	return nil
}

func browserOpener() (string, bool) {
	name := "xdg-open"
	if runtime.GOOS == "darwin" {
		name = "open"
	}
	p, err := exec.LookPath(name)
	return p, err == nil
}

func isTerminal(w io.Writer) bool {
	f, ok := w.(*os.File)
	if !ok {
		return false
	}
	fi, err := f.Stat()
	return err == nil && fi.Mode()&os.ModeCharDevice != 0
}

func newGraphCmd(cfg *Config) *cobra.Command {
	var web, open bool
	var addr string
	cmd := &cobra.Command{
		Use:   "graph --web [--addr HOST:PORT] [--open]",
		Short: "Open the graph view of the web app in the browser with one link",
		Long: "Print a link that opens the graph in the web app, signed in, with no token to paste.\n" +
			"Without MYSELF_URL it serves the local store on 127.0.0.1 (a free port, or --addr)\n" +
			"with a throwaway admin token that lives only in memory, until Ctrl-C.\n" +
			"With MYSELF_URL and MYSELF_TOKEN it asks that server for a login code and exits.\n" +
			"The link carries a one-time code (valid 2 minutes), not a token.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if !web {
				return Usagef("graph needs --web: the browser is the only way to see the graph for now")
			}
			if !cmd.Flags().Changed("open") {
				_, has := browserOpener()
				open = has && isTerminal(cmd.OutOrStdout())
			}
			if cfg.Remote() {
				return graphRemote(cmd, cfg, open)
			}
			return graphLocal(cmd, cfg, addr, open)
		},
	}
	cmd.Flags().BoolVar(&web, "web", false, "show the graph in the web app")
	cmd.Flags().StringVar(&addr, "addr", "127.0.0.1:0", "address to serve on in local mode (default: a free port)")
	cmd.Flags().BoolVar(&open, "open", false, "open the link in a browser (default: when a browser opener exists and stdout is a terminal)")
	return cmd
}

// publish prints the link on stdout and opens it if asked.
func publish(cmd *cobra.Command, link string, open bool) {
	fmt.Fprintln(cmd.OutOrStdout(), link)
	if !open {
		return
	}
	if err := openURL(link); err != nil {
		Warnf(cmd.ErrOrStderr(), "cannot open the browser: %v", err)
	}
}

func graphLink(base, code string) string { return base + "/app/#login=" + code }

func graphRemote(cmd *cobra.Command, cfg *Config, open bool) error {
	if cfg.Token == "" {
		return Usagef("graph --web needs a token for %s: set %s", cfg.URL, EnvToken)
	}
	lc, err := newClient(cmd, cfg).LoginCode()
	if err != nil {
		if brain.Code(err) == brain.ExitNotFound {
			return StoreErr(fmt.Errorf("%s does not support web login links; upgrade the server to a version with POST /api/v1/login-codes", cfg.URL))
		}
		return err
	}
	publish(cmd, graphLink(cfg.URL, lc.Code), open)
	fmt.Fprintf(cmd.ErrOrStderr(), "the link signs in once and expires at %s\n", lc.Expires)
	return nil
}

func graphLocal(cmd *cobra.Command, cfg *Config, addr string, open bool) error {
	stderr := cmd.ErrOrStderr()
	// The admin token exists only as its hash: nobody needs the plaintext,
	// because the login code is minted inside the process.
	var raw [32]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return StoreErr(err)
	}
	sum := sha256.Sum256(raw[:])
	site, err := fs.Sub(myself.Site, "site")
	if err != nil {
		return StoreErr(err)
	}
	srv, err := server.New(server.Config{File: cfg.File, Site: site, Skill: myself.Skill, AdminHash: hex.EncodeToString(sum[:]), Log: io.Discard, Version: Version})
	if err != nil {
		return StoreErr(err)
	}
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		if errors.Is(err, syscall.EADDRINUSE) && isMyselfServer(addr) {
			return Usagef("a myself server is already listening on %s; point at it with %s and %s instead of --addr", addr, EnvURL, EnvToken)
		}
		return StoreErr(err)
	}
	code, expires, err := srv.NewLoginCode(server.Identity{Name: server.AdminName, Scope: server.ScopeWrite, Admin: true})
	if err != nil {
		ln.Close()
		return err
	}
	base := "http://" + localHost(ln.Addr())
	fmt.Fprintf(stderr, "serving %s on %s until Ctrl-C\n", cfg.File, base)
	publish(cmd, graphLink(base, code), open)
	fmt.Fprintf(stderr, "the link signs in once and expires at %s; run the command again for a new one\n", expires.Format("15:04:05"))

	ctx, stop := signal.NotifyContext(cmd.Context(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	return runServer(ctx, srv, ln)
}

// localHost is the host:port to put in a link for a listener: an unspecified
// address (":8080") is reached as 127.0.0.1.
func localHost(a net.Addr) string {
	ta, ok := a.(*net.TCPAddr)
	if !ok {
		return a.String()
	}
	if ta.IP.IsUnspecified() {
		return net.JoinHostPort("127.0.0.1", strconv.Itoa(ta.Port))
	}
	return ta.String()
}

// isMyselfServer reports whether something at addr answers /healthz like
// `myself serve` does.
func isMyselfServer(addr string) bool {
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		return false
	}
	if host == "" || host == "0.0.0.0" || host == "::" {
		host = "127.0.0.1"
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://"+net.JoinHostPort(host, port)+"/healthz", nil)
	if err != nil {
		return false
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(io.LimitReader(resp.Body, 16))
	return resp.StatusCode == http.StatusOK && string(b) == "ok\n"
}
