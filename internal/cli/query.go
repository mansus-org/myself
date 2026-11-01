package cli

import (
	"fmt"
	"io"

	"github.com/mansus-org/myself/internal/brain"
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
		res, err := backendFor(cmd, cfg).Describe(args[0], depth, at.value())
		if err != nil {
			return err
		}
		return emit(cmd, cfg, res, func(w io.Writer) error { return writeLines(w, res.Lines()) })
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
		res, err := backendFor(cmd, cfg).Ls(typ, at.value())
		if err != nil {
			return err
		}
		return emit(cmd, cfg, res, func(w io.Writer) error { return writeLines(w, res.Lines()) })
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
			res, err := backendFor(cmd, cfg).Relations()
			if err != nil {
				return err
			}
			return emit(cmd, cfg, res, func(w io.Writer) error { return writeLines(w, res.Lines()) })
		},
	}
}

func newSearchCmd(cfg *Config) *cobra.Command {
	var n int
	cmd := &cobra.Command{
		Use:   "search <text...> [-n N]",
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
		var nP *int
		if cmd.Flags().Changed("n") {
			nP = &n
		}
		res, err := backendFor(cmd, cfg).Search(joinObject(args), at.value(), nP)
		if err != nil {
			return err
		}
		return emit(cmd, cfg, res, func(w io.Writer) error { return writeLines(w, res.Lines()) })
	}
	cmd.Flags().IntVarP(&n, "n", "n", 0, "keep only the first N matches")
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
			res, err := backendFor(cmd, cfg).Path(args[0], args[1], maxDepth)
			if err != nil {
				return err
			}
			return emit(cmd, cfg, res, func(w io.Writer) error { return writeLines(w, res.Lines()) })
		},
	}
	cmd.Flags().IntVar(&maxDepth, "max-depth", brain.DefaultMaxDepth, "longest chain to look for")
	return cmd
}
