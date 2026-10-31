package cli

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/mansus-org/myself/internal/brain"
	"github.com/mansus-org/myself/internal/gitsync"
	"github.com/spf13/cobra"
)

const mergeDriverCmd = "myself merge-driver %O %A %B"

type syncResult struct {
	Committed int  `json:"committed"`
	Pulled    bool `json:"pulled"`
	Pushed    bool `json:"pushed"`
	Exported  *int `json:"exported"`

	hasRemote bool
	exportDir string
}

func newSyncCmd(cfg *Config) *cobra.Command {
	var exportDir string
	cmd := &cobra.Command{
		Use:   "sync [--export DIR]",
		Short: "Commit the store to git, pull and push, optionally export notes",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			res, err := runSync(cmd, cfg, exportDir)
			if err != nil {
				return err
			}
			return emit(cmd, cfg, res, func(w io.Writer) error {
				var lines []string
				if res.Committed > 0 {
					lines = append(lines, fmt.Sprintf("committed %d records", res.Committed))
				} else {
					lines = append(lines, "nothing to commit")
				}
				if !res.hasRemote {
					lines = append(lines, "no remote configured, skipped pull and push")
				}
				if res.Pulled {
					lines = append(lines, "pulled")
				}
				if res.Exported != nil {
					lines = append(lines, fmt.Sprintf("exported %d entities to %s", *res.Exported, res.exportDir))
				}
				if res.Pushed {
					lines = append(lines, "pushed")
				}
				return writeLines(w, lines)
			})
		},
	}
	cmd.Flags().StringVar(&exportDir, "export", "", "after pulling, write the notes of `export --obsidian` to DIR")
	return cmd
}

const setupHelp = `%s is not inside a git work tree. set up the backup once:
  cd %s
  git init
  echo "%s merge=myself" >> .gitattributes
  git config merge.myself.driver "` + "myself merge-driver %%O %%A %%B" + `"
  git remote add origin <url of a private repository>
then run myself sync again`

func runSync(cmd *cobra.Command, cfg *Config, exportDir string) (*syncResult, error) {
	if err := gitsync.Available(); err != nil {
		return nil, StoreErr(err)
	}
	file, err := filepath.Abs(cfg.File)
	if err != nil {
		return nil, StoreErr(err)
	}
	dir, base := filepath.Dir(file), filepath.Base(file)
	g := gitsync.Git{Dir: dir}
	if out, err := g.Run("rev-parse", "--is-inside-work-tree"); err != nil || out != "true" {
		return nil, &ExitError{Code: ExitUsage, Err: fmt.Errorf(setupHelp, dir, dir, base)}
	}

	if err := ensureDriver(g); err != nil {
		return nil, StoreErr(err)
	}
	attrChanged, err := ensureAttributes(dir, base)
	if err != nil {
		return nil, StoreErr(err)
	}
	res := &syncResult{}
	if err := commitAll(g, dir, base, attrChanged, res); err != nil {
		return nil, StoreErr(err)
	}

	remote, hasRemote := pickRemote(g)
	res.hasRemote = hasRemote
	if hasRemote {
		if err := pull(g, remote); err != nil {
			return nil, StoreErr(err)
		}
		res.Pulled = true
	}
	if exportDir != "" {
		ex, err := brain.ExportObsidian(storeFor(cmd, cfg), exportDir)
		if err != nil {
			return nil, err
		}
		res.Exported, res.exportDir = &ex.Entities, exportDir
	}
	if hasRemote {
		if err := push(g, remote); err != nil {
			return nil, StoreErr(err)
		}
		res.Pushed = true
	}
	return res, nil
}

func ensureDriver(g gitsync.Git) error {
	cur, _ := g.Run("config", "--local", "--get", "merge.myself.driver")
	if cur == mergeDriverCmd {
		return nil
	}
	if _, err := g.Run("config", "--local", "merge.myself.name", "myself append-only log"); err != nil {
		return err
	}
	_, err := g.Run("config", "--local", "merge.myself.driver", mergeDriverCmd)
	return err
}

// ensureAttributes makes sure dir/.gitattributes routes the store file to the
// merge driver. It reports whether the file was written.
func ensureAttributes(dir, base string) (bool, error) {
	path := filepath.Join(dir, ".gitattributes")
	want := base + " merge=myself"
	b, err := os.ReadFile(path)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return false, err
	}
	for _, l := range strings.Split(string(b), "\n") {
		if strings.Join(strings.Fields(l), " ") == want {
			return false, nil
		}
	}
	text := string(b)
	if text != "" && !strings.HasSuffix(text, "\n") {
		text += "\n"
	}
	return true, os.WriteFile(path, []byte(text+want+"\n"), 0o644)
}

// commitAll stages the store file (and .gitattributes if it was written) and
// commits when something changed. Only those paths are committed.
func commitAll(g gitsync.Git, dir, base string, attrChanged bool, res *syncResult) error {
	paths := []string{}
	if _, err := os.Stat(filepath.Join(dir, base)); err == nil {
		paths = append(paths, base)
	}
	if attrChanged || g.Ok("ls-files", "--error-unmatch", ".gitattributes") {
		paths = append(paths, ".gitattributes")
	}
	if len(paths) == 0 {
		return nil
	}
	if _, err := g.Run(append([]string{"add", "--"}, paths...)...); err != nil {
		return err
	}
	if g.Ok(append([]string{"diff", "--cached", "--quiet", "--"}, paths...)...) {
		return nil
	}
	n := 0
	out, err := g.Run("diff", "--cached", "--numstat", "--", base)
	if err != nil {
		return err
	}
	if f := strings.Fields(out); len(f) > 0 {
		n, _ = strconv.Atoi(f[0])
	}
	msg := fmt.Sprintf("myself: %d records since last sync", n)
	if _, err := g.Run(append([]string{"commit", "-m", msg, "--"}, paths...)...); err != nil {
		return err
	}
	res.Committed = n
	return nil
}

// pickRemote returns the remote to sync with: origin if it exists, else the
// first one.
func pickRemote(g gitsync.Git) (string, bool) {
	out, err := g.Run("remote")
	if err != nil || out == "" {
		return "", false
	}
	names := strings.Fields(out)
	for _, n := range names {
		if n == "origin" {
			return n, true
		}
	}
	return names[0], true
}

func branch(g gitsync.Git) (string, error) {
	b, err := g.Run("symbolic-ref", "--short", "HEAD")
	if err != nil {
		return "", errors.New("git: HEAD is detached, check out a branch first")
	}
	return b, nil
}

func hasUpstream(g gitsync.Git) bool {
	return g.Ok("rev-parse", "--abbrev-ref", "--symbolic-full-name", "@{u}")
}

// pull rebases onto the remote branch (the merge driver resolves the store
// file). If the rebase fails it is aborted and a plain merge is tried; if
// that fails too the merge is aborted and an error is returned.
func pull(g gitsync.Git, remote string) error {
	var target []string
	if !hasUpstream(g) {
		b, err := branch(g)
		if err != nil {
			return err
		}
		if !g.Ok("ls-remote", "--exit-code", "--heads", remote, b) {
			return nil // nothing on the remote yet, the push creates it
		}
		target = []string{remote, b}
	}
	_, rerr := g.Run(append([]string{"pull", "--rebase"}, target...)...)
	if rerr == nil {
		return nil
	}
	g.Run("rebase", "--abort")
	_, merr := g.Run(append([]string{"pull", "--no-rebase", "--no-edit", "--allow-unrelated-histories"}, target...)...)
	if merr == nil {
		return nil
	}
	g.Run("merge", "--abort")
	return fmt.Errorf("could not merge the remote changes: rebase failed (%v); merge failed (%v). resolve it by hand in %s", rerr, merr, g.Dir)
}

func push(g gitsync.Git, remote string) error {
	if hasUpstream(g) {
		_, err := g.Run("push")
		return err
	}
	_, err := g.Run("push", "-u", remote, "HEAD")
	return err
}
