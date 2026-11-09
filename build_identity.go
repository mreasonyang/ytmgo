package main

import (
	_ "embed"
	"encoding/json"
	"runtime/debug"
	"strings"

	"github.com/egoist/mygo"
)

// The checked-in config is the only fallback version source. Packaged builds
// use MyGo's injected version; releases must opt in through scripts/release/build.py.
//
//go:embed mygo.json
var buildConfig []byte

var (
	version     = configVersion()
	buildKind   = "development"
	buildCommit = ""
	buildBranch = "unknown"
	buildDirty  = ""
)

type buildIdentity struct {
	Version        string `json:"version"`
	BuildNumber    string `json:"build_number"`
	Kind           string `json:"kind"`
	Commit         string `json:"commit"`
	Branch         string `json:"branch"`
	Dirty          bool   `json:"dirty"`
	UpdatesEnabled bool   `json:"updates_enabled"`
}

func configVersion() string {
	var config struct {
		Version string `json:"version"`
	}
	if json.Unmarshal(buildConfig, &config) != nil || config.Version == "" {
		panic("ytmGO: invalid embedded build version")
	}
	return config.Version
}

func currentBuildIdentity() buildIdentity {
	b := buildIdentity{Version: mygo.App.Version(), Kind: buildKind, Commit: buildCommit,
		Branch: buildBranch, Dirty: buildDirty == "true", UpdatesEnabled: updatesEnabled()}
	if b.Version == "" {
		b.Version = version
	}
	var config struct {
		MacOS struct {
			InfoPlist map[string]any `json:"infoPlist"`
		} `json:"macos"`
	}
	_ = json.Unmarshal(buildConfig, &config)
	b.BuildNumber, _ = config.MacOS.InfoPlist["CFBundleVersion"].(string)
	if b.BuildNumber == "" {
		b.BuildNumber = b.Version
	}
	if info, ok := debug.ReadBuildInfo(); ok && b.Commit == "" {
		for _, setting := range info.Settings {
			switch setting.Key {
			case "vcs.revision":
				b.Commit = setting.Value
			case "vcs.modified":
				b.Dirty = setting.Value == "true"
			}
		}
	}
	return b
}

func (b buildIdentity) displayVersion() string {
	if b.Kind == "release" {
		return b.Version
	}
	commit := b.Commit
	if len(commit) > 12 {
		commit = commit[:12]
	}
	if commit == "" {
		commit = "unknown"
	}
	parts := []string{b.Version, "DEV", b.Branch, commit}
	if b.Dirty {
		parts = append(parts, "dirty")
	}
	return strings.Join(parts, " · ")
}

func updatesEnabled() bool {
	return buildKind == "release" && mygo.Updater.Enabled()
}
