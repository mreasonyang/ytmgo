// A build-only bundler. The application has no Node/npm build or runtime need.
package main

import (
	"fmt"
	"github.com/evanw/esbuild/pkg/api"
	"os"
	"path/filepath"
	"strings"
)

func main() {
	if len(os.Args) != 4 {
		os.Exit(2)
	}
	entry, modules, output := os.Args[1], os.Args[2], os.Args[3]
	plugin := api.Plugin{Name: "pinned-package-files", Setup: func(build api.PluginBuild) {
		build.OnResolve(api.OnResolveOptions{Filter: `^(youtubei\.js|meriyah)/`}, func(args api.OnResolveArgs) (api.OnResolveResult, error) {
			return api.OnResolveResult{Path: filepath.Join(modules, filepath.FromSlash(args.Path))}, nil
		})
		build.OnResolve(api.OnResolveOptions{Filter: `^meriyah$`}, func(args api.OnResolveArgs) (api.OnResolveResult, error) {
			adapter, _ := filepath.Abs(filepath.Join(filepath.Dir(entry), "parser.mjs"))
			return api.OnResolveResult{Path: adapter}, nil
		})
	}}
	r := api.Build(api.BuildOptions{EntryPoints: []string{entry}, Bundle: true, Write: true, Outfile: output, Format: api.FormatIIFE, Platform: api.PlatformBrowser, Target: api.ES2020, MinifyWhitespace: true, MinifyIdentifiers: true, MinifySyntax: true, LegalComments: api.LegalCommentsInline, NodePaths: []string{modules}, Plugins: []api.Plugin{plugin}, LogLevel: api.LogLevelSilent})
	if len(r.Errors) > 0 {
		fmt.Fprintln(os.Stderr, "script_bundle_failed")
		for _, e := range r.Errors {
			if strings.Contains(e.Text, "resolve") {
				fmt.Fprintln(os.Stderr, "dependency_resolution_failed")
			}
		}
		os.Exit(1)
	}
}
