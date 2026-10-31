package cli

import (
	"fmt"
	"io"
	"time"

	"github.com/mansus-org/myself/internal/brain"
	"github.com/mansus-org/myself/internal/model"
	"github.com/spf13/cobra"
)

func newForgetCmd(cfg *Config) *cobra.Command {
	var reason string
	cmd := &cobra.Command{
		Use:   "forget <uuid> | <subject> <relation> [<object...>]",
		Short: "Retract a record by uuid, or every live record with a subject and relation",
		Long: "Retract a record. With one uuid argument, retract that record. Otherwise retract\n" +
			"every live record with that subject and relation, narrowed to those whose object\n" +
			"matches when one is given. Exits 2 if nothing matches.",
		RunE: func(cmd *cobra.Command, args []string) error {
			a, err := forgetArgs(args)
			if err != nil {
				return err
			}
			a.Reason = reason
			res, err := brain.Forget(storeFor(cmd, cfg), a, time.Now())
			if err != nil {
				return err
			}
			return emit(cmd, cfg, res, func(w io.Writer) error {
				for _, f := range res.Forgotten {
					if _, err := fmt.Fprintf(w, "forgot %s\n", f.Subject); err != nil {
						return err
					}
				}
				return nil
			})
		},
	}
	cmd.Flags().StringVar(&reason, "reason", "", "why the record is retracted, stored as the forget record's object")
	return cmd
}

// forgetArgs picks the form of SPEC section 5: `forget <uuid>` when there is
// exactly one argument and it is a uuid, else `<subject> <relation>
// [<object...>]`.
func forgetArgs(args []string) (brain.ForgetArgs, error) {
	if len(args) == 1 && model.IsUUID(args[0]) {
		return brain.ForgetArgs{UUID: args[0]}, nil
	}
	if len(args) < 2 {
		return brain.ForgetArgs{}, Usagef("%s", brain.ForgetUsage)
	}
	a := brain.ForgetArgs{Subject: args[0], Relation: args[1]}
	if len(args) > 2 {
		obj := joinObject(args[2:])
		a.Object = &obj
	}
	return a, nil
}
