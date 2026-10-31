package cli

import (
	"fmt"
	"github.com/mansus-org/myself/internal/model"
	"io"
)

// printJSON writes v as one compact JSON value followed by a newline.
func printJSON(w io.Writer, v any) error {
	b, err := model.Marshal(v)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintf(w, "%s\n", b)
	return err
}

// printError writes the single-line error format of SPEC section 6.
func printError(w io.Writer, err error) {
	fmt.Fprintf(w, "error: %s\n", err)
}

// Warnf writes a "warning: ..." line.
func Warnf(w io.Writer, format string, a ...any) {
	fmt.Fprintf(w, "warning: "+format+"\n", a...)
}
