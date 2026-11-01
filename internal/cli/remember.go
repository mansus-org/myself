package cli

import (
	"bufio"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/mansus-org/myself/internal/brain"
	"github.com/spf13/cobra"
)

func newRememberCmd(cfg *Config) *cobra.Command {
	var replace bool
	cmd := &cobra.Command{
		Use:   "remember <subject> <relation> <object...> | -",
		Short: "Write a fact, or one per stdin line with -",
		Long: "Write a fact. Everything after the relation is the object. With - read one\n" +
			"\"subject relation object...\" per stdin line; nothing is written if any line is bad.\n" +
			"--replace first forgets every live record with the same subject and relation.",
		RunE: func(cmd *cobra.Command, args []string) error {
			now := time.Now()
			var facts []brain.Fact
			switch {
			case len(args) == 1 && args[0] == "-":
				var err error
				if facts, err = readBulk(cmd.InOrStdin(), now); err != nil {
					return err
				}
			case len(args) >= 2:
				facts = []brain.Fact{{Subject: args[0], Relation: args[1], Object: joinObject(args[2:])}}
			default:
				return Usagef("remember needs <subject> <relation> <object...>, or - to read stdin")
			}
			res, err := backendFor(cmd, cfg).Remember(facts, replace)
			if err != nil {
				return err
			}
			return emit(cmd, cfg, res, func(w io.Writer) error { return writeLines(w, res.Lines()) })
		},
	}
	cmd.Flags().BoolVar(&replace, "replace", false, "forget live records with the same subject and relation first")
	return cmd
}

// readBulk parses `remember -` input: one "subject relation object..." per
// line, blank lines skipped. Every line is validated before anything is
// written; the first bad one is reported with its line number.
func readBulk(r io.Reader, now time.Time) ([]brain.Fact, error) {
	var facts []brain.Fact
	br := bufio.NewReader(r)
	for n := 1; ; n++ {
		line, err := br.ReadString('\n')
		if err != nil && err != io.EOF {
			return nil, StoreErr(fmt.Errorf("reading stdin: %w", err))
		}
		if words := strings.Fields(line); len(words) > 0 {
			if len(words) < 2 {
				return nil, Usagef("line %d: want \"subject relation object...\"", n)
			}
			f := brain.Fact{Subject: words[0], Relation: words[1], Object: unquote(joinObject(words[2:]))}
			if _, ferr := f.Record(now); ferr != nil {
				return nil, Usagef("line %d: %w", n, ferr)
			}
			facts = append(facts, f)
		}
		if err == io.EOF {
			return facts, nil
		}
	}
}

// unquote strips one matching pair of surrounding '...' or "..." from a bulk
// object, which the shell would have stripped in single-argument mode.
func unquote(s string) string {
	if n := len(s); n >= 2 && (s[0] == '\'' || s[0] == '"') && s[n-1] == s[0] {
		return s[1 : n-1]
	}
	return s
}
