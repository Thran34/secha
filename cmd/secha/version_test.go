package main

import (
	"runtime/debug"
	"testing"
)

func TestLdflagsWinOverBuildInfo(t *testing.T) {
	info := &debug.BuildInfo{Main: debug.Module{Version: "v9.9.9"}}

	got := resolveStamp(buildStamp{"0.1.0", "abc1234", "2026-10-26"}, info)

	if got != (buildStamp{"0.1.0", "abc1234", "2026-10-26"}) {
		t.Errorf("resolveStamp = %+v, want the release build's ldflags untouched", got)
	}
}

func TestGoInstallBuildsTakeTheModuleVersionAndRevision(t *testing.T) {
	info := &debug.BuildInfo{
		Main: debug.Module{Version: "v0.1.0"},
		Settings: []debug.BuildSetting{
			{Key: "vcs.revision", Value: "38ec1df0123456789abcdef"},
			{Key: "vcs.time", Value: "2026-10-26T17:06:28Z"},
		},
	}

	got := resolveStamp(buildStamp{"dev", "none", "unknown"}, info)

	if got != (buildStamp{"0.1.0", "38ec1df", "2026-10-26T17:06:28Z"}) {
		t.Errorf("resolveStamp = %+v, want version, short commit and time from build info", got)
	}
}

func TestLocalDevelBuildStaysDev(t *testing.T) {
	info := &debug.BuildInfo{Main: debug.Module{Version: "(devel)"}}

	got := resolveStamp(buildStamp{"dev", "none", "unknown"}, info)

	if got.version != "dev" {
		t.Errorf("version = %q, want dev when the module version is (devel)", got.version)
	}
}

func TestNoBuildInfoKeepsTheDefaults(t *testing.T) {
	got := resolveStamp(buildStamp{"dev", "none", "unknown"}, nil)

	if got != (buildStamp{"dev", "none", "unknown"}) {
		t.Errorf("resolveStamp = %+v, want the defaults", got)
	}
}
