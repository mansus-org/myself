package export

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/mansus-org/myself/internal/graph"
	"github.com/mansus-org/myself/internal/model"
)

// Note renders the note for entity name: `# name`, its outgoing records with
// objects that equal an entity as wikilinks, then `## Linked from` with the
// incoming records. Empty sections are omitted.
func Note(g *graph.Graph, name string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# %s\n", name)
	if out := g.Out(name); len(out) > 0 {
		b.WriteString("\n")
		for _, r := range out {
			obj := r.Object
			if n := model.NormalizeName(obj); n != "" && g.Has(n) {
				obj = "[[" + n + "]]"
			}
			fmt.Fprintf(&b, "- %s %s\n", r.Relation, obj)
		}
	}
	if in := g.In(name); len(in) > 0 {
		b.WriteString("\n## Linked from\n")
		for _, r := range in {
			fmt.Fprintf(&b, "- [[%s]] %s\n", r.Subject, r.Relation)
		}
	}
	return b.String()
}

// FileName is the note file name for an entity. Normalized names are already
// safe; `/` and a leading `.` are escaped anyway so a note can never leave
// dir or be hidden.
func FileName(name string) string {
	name = strings.ReplaceAll(name, "/", "_")
	name = strings.ReplaceAll(name, string(filepath.Separator), "_")
	if strings.HasPrefix(name, ".") {
		name = "_" + name[1:]
	}
	return name + ".md"
}

// Write creates dir if needed and writes one note per entity of g,
// overwriting existing files of the same name. It returns the file names,
// sorted.
func Write(dir string, g *graph.Graph) ([]string, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	files := []string{}
	for _, name := range g.Entities() {
		f := FileName(name)
		if err := os.WriteFile(filepath.Join(dir, f), []byte(Note(g, name)), 0o644); err != nil {
			return nil, err
		}
		files = append(files, f)
	}
	return files, nil
}
