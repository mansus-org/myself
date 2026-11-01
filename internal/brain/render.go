package brain

import (
	"fmt"
	"strings"

	"github.com/mansus-org/myself/internal/model"
)

// The Lines methods render a result as the plain-text lines the CLI prints
// without --json (SPEC sections 6 and 9). The CLI and the MCP tools share
// them.

// RecordLine renders a record as a log line (SPEC section 9). A forget record
// reads `<uuid> <time> forget <target-uuid> <reason>`.
func RecordLine(r model.Record) string {
	if r.IsForget() {
		return strings.TrimSpace(fmt.Sprintf("%s %s forget %s %s", r.UUID, r.Time, r.Subject, r.Object))
	}
	return fmt.Sprintf("%s %s %s %s %s", r.UUID, r.Time, r.Subject, r.Relation, r.Object)
}

// RecordLines renders each record with RecordLine.
func RecordLines(recs []model.Record) []string {
	lines := make([]string, len(recs))
	for i, r := range recs {
		lines[i] = RecordLine(r)
	}
	return lines
}

// Lines renders each described entity with RenderEntity.
func (r DescribeResult) Lines() []string {
	var lines []string
	for _, e := range r.Entities {
		lines = append(lines, RenderEntity(e.Name, e.Out, e.In)...)
	}
	return lines
}

// Lines is one entity name per line.
func (r LsResult) Lines() []string { return r.Entities }

// Lines is one relation per line.
func (r RelationsResult) Lines() []string { return r.Relations }

// Lines is one log line per match.
func (r SearchResult) Lines() []string { return RecordLines(r.Matches) }

// Lines is one `subject relation object` per hop.
func (r PathResult) Lines() []string {
	lines := make([]string, len(r.Path))
	for i, rec := range r.Path {
		lines[i] = fmt.Sprintf("%s %s %s", rec.Subject, rec.Relation, rec.Object)
	}
	return lines
}

// Lines is one log line per record.
func (r RecordsResult) Lines() []string { return RecordLines(r.Records) }

// Lines is one line of space-joined names per group.
func (r DreamResult) Lines() []string {
	lines := make([]string, len(r.NearDuplicates))
	for i, grp := range r.NearDuplicates {
		lines[i] = strings.Join(grp, " ")
	}
	return lines
}

// Lines is `forgot <subject>` per forgotten record, then
// `remembered <uuid>` per new one.
func (r RememberResult) Lines() []string {
	lines := forgotLines(r.Forgotten)
	for _, rec := range r.Remembered {
		lines = append(lines, "remembered "+rec.UUID)
	}
	return lines
}

// Lines is `forgot <subject>` per forgotten record.
func (r ForgetResult) Lines() []string { return forgotLines(r.Forgotten) }

func forgotLines(recs []model.Record) []string {
	var lines []string
	for _, f := range recs {
		lines = append(lines, "forgot "+f.Subject)
	}
	return lines
}
