package cli

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/mansus-org/myself"
	"github.com/mansus-org/myself/internal/brain"
	"github.com/mansus-org/myself/internal/model"
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
			"If a myself server already serves this store on --addr, it is reused instead.\n" +
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

// graphLink is the URL that opens the graph and signs in with a one-time code.
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
	fmt.Fprintf(cmd.ErrOrStderr(), "the link signs in once and expires at %s\n", clockOf(lc.Expires))
	return nil
}

func graphLocal(cmd *cobra.Command, cfg *Config, addr string, open bool) error {
	stderr := cmd.ErrOrStderr()
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		if errors.Is(err, syscall.EADDRINUSE) {
			return graphReuse(cmd, cfg, addr, open)
		}
		return StoreErr(err)
	}
	// The admin token exists only as its hash: nobody needs the plaintext,
	// because the login code is minted inside the process.
	var raw [32]byte
	if _, err := rand.Read(raw[:]); err != nil {
		ln.Close()
		return StoreErr(err)
	}
	sum := sha256.Sum256(raw[:])
	// The local login key is what lets the next `graph --web` on this port ask
	// this server for a link instead of failing on a taken port.
	key, err := server.NewLocalKey()
	if err != nil {
		ln.Close()
		return StoreErr(err)
	}
	site, err := fs.Sub(myself.Site, "site")
	if err != nil {
		ln.Close()
		return StoreErr(err)
	}
	srv, err := server.New(server.Config{File: cfg.File, Site: site, Skill: myself.Skill, AdminHash: hex.EncodeToString(sum[:]), LocalKey: key, Log: io.Discard, Version: Version})
	if err != nil {
		ln.Close()
		return err
	}
	unrecord, err := localRecord(cfg.File, ln.Addr(), key)
	if err != nil {
		Warnf(stderr, "the next graph --web on this port cannot reuse this server: %v", err)
	} else {
		defer unrecord()
	}
	code, expires, err := srv.NewLoginCode(server.Identity{Name: server.AdminName, Scope: server.ScopeWrite, Admin: true})
	if err != nil {
		ln.Close()
		return err
	}
	base := "http://" + loopbackHost(ln.Addr().String())
	fmt.Fprintf(stderr, "serving %s on %s until Ctrl-C\n", cfg.File, base)
	publish(cmd, graphLink(base, code), open)
	fmt.Fprintf(stderr, "the link signs in once and expires at %s; run the command again for a new one\n", expires.Format("15:04:05"))

	ctx, stop := signal.NotifyContext(cmd.Context(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	return runServer(ctx, srv, ln)
}

// localRecord leaves the record of a server that has just started next to its
// store (server.LocalInfo) and returns the func that clears it again.
func localRecord(file string, a net.Addr, key string) (func(), error) {
	return server.WriteLocalInfo(server.LocalInfo{Store: file, Addr: a.String(), PID: os.Getpid(), Key: key})
}

// graphReuse prints a link from the myself server already listening on addr
// instead of starting a second one on a port that is taken. It reuses a server
// of this same store and no other: a link into a different brain is not what
// --file asked for, so that stays a usage error. Nothing here disturbs the
// server it reuses; it keeps running after this command exits.
func graphReuse(cmd *cobra.Command, cfg *Config, addr string, open bool) error {
	stderr := cmd.ErrOrStderr()
	base := "http://" + loopbackHost(addr)
	if !isMyselfServer(base) {
		return Usagef("address %s is taken by something that is not a myself server; stop it or choose another --addr", addr)
	}
	port, err := netPort(addr)
	if err != nil {
		return Usagef("--addr %s has no port to look for a running myself server by", addr)
	}
	info, err := server.ReadLocalInfo(cfg.File, port)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		return Usagef("no myself server on %s is recorded as serving %s; the server there is an older myself, or it serves another store; choose another --addr, or point --file at the store it serves", addr, cfg.File)
	case err != nil:
		return StoreErr(err)
	}
	if !sameStore(info.Store, cfg.File) {
		return Usagef("the myself server on %s serves %s, not %s; open it with graph --web --file %s, or choose another --addr", addr, info.Store, cfg.File, info.Store)
	}
	code, expires, err := loginCode(addr, info.Key)
	if err != nil {
		return err
	}
	publish(cmd, graphLink(base, code), open)
	fmt.Fprintf(stderr, "the myself server already on %s serves %s; it keeps running after this command exits\n", addr, info.Store)
	fmt.Fprintf(stderr, "the link signs in once and expires at %s\n", clockOf(expires))
	return nil
}

// clockOf is a timestamp from the server as a time of day, for a line a person
// has to read before the link goes stale.
func clockOf(stamp string) string {
	t, err := time.Parse(model.TimeLayout, stamp)
	if err != nil {
		return stamp
	}
	return t.Format("15:04:05")
}

// loginCode asks the myself server on addr for a one-time code with its local
// login key. The key travels in the header only, never in a URL or a command
// line.
func loginCode(addr, key string) (code, expires string, err error) {
	base := "http://" + loopbackHost(addr)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, base+"/api/v1/login-codes", strings.NewReader("{}"))
	if err != nil {
		return "", "", StoreErr(err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+key)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", "", StoreErr(fmt.Errorf("cannot reach the myself server on %s: %w", base, err))
	}
	defer resp.Body.Close()
	var reply struct {
		Code    string `json:"code"`
		Expires string `json:"expires"`
		Error   string `json:"error"`
	}
	// The status decides below; the body only adds the reason it says.
	json.NewDecoder(io.LimitReader(resp.Body, 1<<16)).Decode(&reply)
	switch {
	case resp.StatusCode == http.StatusCreated && reply.Code != "":
		return reply.Code, reply.Expires, nil
	case resp.StatusCode == http.StatusServiceUnavailable:
		return "", "", Usagef("the myself server on %s has its API disabled: start it with %s set, or choose another --addr", addr, EnvAdminHash)
	case resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden:
		return "", "", Usagef("the myself server on %s no longer accepts its login key; it has probably been restarted since, so run graph --web again", addr)
	}
	return "", "", StoreErr(fmt.Errorf("the myself server on %s refused a login link: %s", addr, reply.Error))
}

// loopbackHost is addr with an empty or unspecified host filled in as
// 127.0.0.1, for a URL to a server on this machine.
func loopbackHost(addr string) string {
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		return addr
	}
	if host == "" || host == "0.0.0.0" || host == "::" {
		host = "127.0.0.1"
	}
	return net.JoinHostPort(host, port)
}

// netPort is the port of an --addr value.
func netPort(addr string) (int, error) {
	_, p, err := net.SplitHostPort(addr)
	if err != nil {
		return 0, err
	}
	return strconv.Atoi(p)
}

// sameStore reports whether two paths are the same store file, so that a
// relative or symlinked --file still recognises the server that has it.
func sameStore(a, b string) bool {
	if a == "" || b == "" {
		return false
	}
	real := func(p string) string {
		if abs, err := filepath.Abs(p); err == nil {
			p = abs
		}
		if r, err := filepath.EvalSymlinks(p); err == nil {
			p = r
		}
		return p
	}
	return real(a) == real(b)
}

// isMyselfServer reports whether base answers /healthz the way `myself serve`
// does.
func isMyselfServer(base string) bool {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, base+"/healthz", nil)
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
