package cli

import (
	"runtime/debug"
	"testing"
)

func TestResolveVersion(t *testing.T) {
	oldV, oldC, oldD := Version, Commit, BuildDate
	t.Cleanup(func() { Version, Commit, BuildDate = oldV, oldC, oldD })

	info := &debug.BuildInfo{
		Main: debug.Module{Version: "v0.1.0"},
		Settings: []debug.BuildSetting{
			{Key: "vcs.revision", Value: "0123456789abcdef"},
			{Key: "vcs.time", Value: "2026-10-31T10:00:00Z"},
		},
	}
	read := func(bi *debug.BuildInfo, ok bool) func() (*debug.BuildInfo, bool) {
		return func() (*debug.BuildInfo, bool) { return bi, ok }
	}

	tests := []struct {
		name                   string
		ver, commit, date      string
		read                   func() (*debug.BuildInfo, bool)
		wantVer, wantC, wantDt string
	}{
		{"fallback", "dev", "none", "unknown", read(info, true), "v0.1.0", "0123456", "2026-10-31T10:00:00Z"},
		{"ldflags win", "v9", "abc", "today", read(info, true), "v9", "abc", "today"},
		{"partial ldflags", "v9", "none", "unknown", read(info, true), "v9", "0123456", "2026-10-31T10:00:00Z"},
		{"devel main", "dev", "none", "unknown", read(&debug.BuildInfo{Main: debug.Module{Version: "(devel)"}}, true), "dev", "none", "unknown"},
		{"no build info", "dev", "none", "unknown", read(nil, false), "dev", "none", "unknown"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			Version, Commit, BuildDate = tt.ver, tt.commit, tt.date
			got := resolveVersion(tt.read)
			if got.Version != tt.wantVer || got.Commit != tt.wantC || got.Date != tt.wantDt {
				t.Errorf("got %+v", got)
			}
		})
	}
}
