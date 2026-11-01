package cli

import (
	"bytes"
	"path/filepath"
	"testing"
)

func TestServeRejectsBadAdminHash(t *testing.T) {
	t.Setenv(EnvAdminHash, "not-hex")
	var out, errb bytes.Buffer
	code := Execute([]string{"serve", "--addr", "127.0.0.1:0", "--file", filepath.Join(t.TempDir(), "b.jsonl")}, &out, &errb)
	want := "error: MYSELF_ADMIN_TOKEN_SHA256 must be 64 hex characters (the sha256 of the admin token)\n"
	if code != ExitUsage || errb.String() != want || out.Len() != 0 {
		t.Fatalf("exit %d, stderr %q", code, errb.String())
	}
}
