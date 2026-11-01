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

func TestServeWarnsAboutClientSettings(t *testing.T) {
	t.Setenv(EnvAdminHash, "not-hex")
	t.Setenv(EnvToken, "")
	t.Setenv(EnvURL, "")
	file := filepath.Join(t.TempDir(), "b.jsonl")
	tokenWarn := "warning: --token / MYSELF_TOKEN is the client token for --url; serve checks the admin token against MYSELF_ADMIN_TOKEN_SHA256 (the sha256 of the token)\n"
	urlWarn := "warning: --url / MYSELF_URL is ignored; serve always uses the local store\n"
	bad := "error: MYSELF_ADMIN_TOKEN_SHA256 must be 64 hex characters (the sha256 of the admin token)\n"
	for _, tc := range []struct {
		name string
		env  map[string]string
		args []string
		want string
	}{
		{"none", nil, nil, bad},
		{"token flag", nil, []string{"--token", "mys_1111"}, tokenWarn + bad},
		{"token env", map[string]string{EnvToken: "mys_1111"}, nil, tokenWarn + bad},
		{"url flag", nil, []string{"--url", "http://localhost:9"}, urlWarn + bad},
		{"url env", map[string]string{EnvURL: "http://localhost:9"}, nil, urlWarn + bad},
		{"both", map[string]string{EnvToken: "x", EnvURL: "http://localhost:9"}, nil, tokenWarn + urlWarn + bad},
	} {
		t.Run(tc.name, func(t *testing.T) {
			for k, v := range tc.env {
				t.Setenv(k, v)
			}
			var out, errb bytes.Buffer
			args := append([]string{"serve", "--addr", "127.0.0.1:0", "--file", file}, tc.args...)
			code := Execute(args, &out, &errb)
			if code != ExitUsage || errb.String() != tc.want || out.Len() != 0 {
				t.Fatalf("exit %d, stderr %q", code, errb.String())
			}
		})
	}
}
