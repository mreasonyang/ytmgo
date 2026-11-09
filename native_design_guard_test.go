package main

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// The app's look comes from the design guidebook (design/README.md) through
// the generated tokens. This keeps literal values out of the view code:
// colours, corners, type sizes and shadows come from tokens or the palette,
// and literal spacing sits on the guidebook's scale.
func TestNativeViewsUseDesignTokens(t *testing.T) {
	spacing := map[float64]bool{0: true, 4: true, 8: true, 12: true, 16: true, 24: true, 32: true, 48: true, 72: true}
	spaceCalls := map[string]bool{"Padding": true, "PaddingX": true, "PaddingY": true, "PaddingStart": true, "PaddingEnd": true,
		"PaddingTop": true, "PaddingBottom": true, "Gap": true, "Margin": true, "MarginTop": true, "MarginBottom": true}
	files, err := filepath.Glob("native_*.go")
	if err != nil {
		t.Fatal(err)
	}
	fset := token.NewFileSet()
	var problems []string
	for _, name := range files {
		// The palette and the generated tokens are where values live.
		if strings.HasSuffix(name, "_test.go") || name == "native_tokens_gen.go" || name == "native_style.go" {
			continue
		}
		file, err := parser.ParseFile(fset, name, nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		ast.Inspect(file, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			sel, ok := call.Fun.(*ast.SelectorExpr)
			if !ok {
				return true
			}
			at := fset.Position(call.Pos())
			report := func(format string, args ...any) {
				problems = append(problems, fmt.Sprintf("%s:%d: %s", at.Filename, at.Line, fmt.Sprintf(format, args...)))
			}
			literal := func(e ast.Expr) (float64, bool) {
				lit, ok := e.(*ast.BasicLit)
				if !ok {
					return 0, false
				}
				v, err := strconv.ParseFloat(lit.Value, 64)
				return v, err == nil
			}
			pkg, _ := sel.X.(*ast.Ident)
			switch {
			case pkg != nil && pkg.Name == "ui" && (sel.Sel.Name == "Hex" || sel.Sel.Name == "RGBA"):
				report("literal colour ui.%s; use a palette role", sel.Sel.Name)
			case sel.Sel.Name == "Radius" && len(call.Args) == 1:
				if v, ok := literal(call.Args[0]); ok && v != 0 {
					report("Radius(%g); use a radius token", v)
				}
			case sel.Sel.Name == "FontSize" && len(call.Args) == 1:
				if v, ok := literal(call.Args[0]); ok {
					report("FontSize(%g); use a type token", v)
				}
			case sel.Sel.Name == "Shadow" && len(call.Args) != 1:
				report("Shadow with literal values; use a tokenShadow elevation")
			case spaceCalls[sel.Sel.Name]:
				for _, arg := range call.Args {
					if v, ok := literal(arg); ok && !spacing[v] {
						report("%s(%g) is off the spacing scale", sel.Sel.Name, v)
					}
				}
			}
			return true
		})
	}
	if len(problems) > 0 {
		t.Fatalf("%d values stray from the design tokens:\n  %s", len(problems), strings.Join(problems, "\n  "))
	}
}
