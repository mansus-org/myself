package cli

import (
	"fmt"
	"io"
	"strings"

	"github.com/mansus-org/myself/internal/brain"
	"github.com/mansus-org/myself/internal/model"
	"github.com/spf13/cobra"
)

func writeLines(w io.Writer, lines []string) error {
	for _, l := range lines {
		if _, err := fmt.Fprintln(w, l); err != nil {
			return err
		}
	}
	return nil
}

// recordLine renders a record as a log line (SPEC section 9). A forget record
// reads `<uuid> <time> forget <target-uuid> <reason>`.
func recordLine(r model.Record) string {
	if r.IsForget() {
		return strings.TrimSpace(fmt.Sprintf("%s %s forget %s %s", r.UUID, r.Time, r.Subject, r.Object))
	}
	return fmt.Sprintf("%s %s %s %s %s", r.UUID, r.Time, r.Subject, r.Relation, r.Object)
}

func writeRecordLines(w io.Writer, recs []model.Record) error {
	lines := make([]string, len(recs))
	for i, r := range recs {
		lines[i] = recordLine(r)
	}
	return writeLines(w, lines)
}

func newDescribeCmd(cfg *Config) *cobra.Command {
	var depth int
	cmd := &cobra.Command{
		Use:   "describe <entity>",
		Short: "Show the live records of an entity, and of its neighbours with --depth",
		Args: func(cmd *cobra.Command, args []string) error {
			if len(args) != 1 {
				return Usagef("describe needs exactly one <entity>")
			}
			return nil
		},
	}
	at := addAtFlag(cmd)
	cmd.Flags().IntVar(&depth, "depth", 0, "also show entities reachable within N hops")
	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		res, err := brain.Describe(storeFor(cmd, cfg), args[0], depth, at.value())
		if err != nil {
			return err
		}
		return emit(cmd, cfg, res, func(w io.Writer) error {
			var lines []string
			for _, e := range res.Entities {
				lines = append(lines, brain.RenderEntity(e.Name, e.Out, e.In)...)
			}
			return writeLines(w, lines)
		})
	}
	return cmd
}

func newLsCmd(cfg *Config) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "ls [<type>]",
		Short: "List the types, or the entities that are of a type",
		Args: func(cmd *cobra.Command, args []string) error {
			if len(args) > 1 {
				return Usagef("ls takes at most one <type>")
			}
			return nil
		},
	}
	at := addAtFlag(cmd)
	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		var typ *string
		if len(args) == 1 {
			typ = &args[0]
		}
		res, err := brain.Ls(storeFor(cmd, cfg), typ, at.value())
		if err != nil {
			return err
		}
		return emit(cmd, cfg, res, func(w io.Writer) error { return writeLines(w, res.Entities) })
	}
	return cmd
}

func newRelationsCmd(cfg *Config) *cobra.Command {
	return &cobra.Command{
		Use:   "relations",
		Short: "List the relations you can use",
		Args: func(cmd *cobra.Command, args []string) error {
			if len(args) != 0 {
				return Usagef("relations takes no arguments")
			}
			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			res := brain.Relations()
			return emit(cmd, cfg, res, func(w io.Writer) error { return writeLines(w, res.Relations) })
		},
	}
}

func newSearchCmd(cfg *Config) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "search <text...>",
		Short: "Find live records whose subject, relation or object contains text",
		Args: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 {
				return Usagef("search needs <text>")
			}
			return nil
		},
	}
	at := addAtFlag(cmd)
	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		res, err := brain.Search(storeFor(cmd, cfg), joinObject(args), at.value())
		if err != nil {
			return err
		}
		return emit(cmd, cfg, res, func(w io.Writer) error { return writeRecordLines(w, res.Matches) })
	}
	return cmd
}

func newPathCmd(cfg *Config) *cobra.Command {
	var maxDepth int
	cmd := &cobra.Command{
		Use:   "path <a> <b>",
		Short: "Show the shortest chain of live records between two entities",
		Args: func(cmd *cobra.Command, args []string) error {
			if len(args) != 2 {
				return Usagef("path needs <a> and <b>")
			}
			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			res, err := brain.Path(storeFor(cmd, cfg), args[0], args[1], maxDepth)
			if err != nil {
				return err
			}
			return emit(cmd, cfg, res, func(w io.Writer) error {
				for _, r := range res.Path {
					if _, err := fmt.Fprintf(w, "%s %s %s\n", r.Subject, r.Relation, r.Object); err != nil {
						return err
					}
				}
				return nil
			})
		},
	}
	cmd.Flags().IntVar(&maxDepth, "max-depth", brain.DefaultMaxDepth, "longest chain to look for")
	return cmd
}
