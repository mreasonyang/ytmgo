package main

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"math"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// The app's look comes from the design guidebook (design/README.md) through
// the generated tokens. This keeps literal values out of the view code:
// colours, corners, type sizes and shadows come from tokens or the palette,
// and every literal spatial value (padding, margin, gap, size, position) is a
// multiple of the 4 px base unit; 1 px hairlines are the one exception.
func TestNativeViewsUseDesignTokens(t *testing.T) {
	onGrid := func(v float64) bool { return v == math.Trunc(v) && int(v)%4 == 0 }
	spaceCalls := map[string]bool{"Padding": true, "PaddingX": true, "PaddingY": true, "PaddingStart": true, "PaddingEnd": true,
		"PaddingTop": true, "PaddingBottom": true, "Gap": true, "Margin": true, "MarginTop": true, "MarginBottom": true,
		"Size": true, "Width": true, "Height": true, "MinWidth": true, "MinHeight": true, "MaxWidth": true, "MaxHeight": true,
		"Left": true, "Top": true, "Right": true, "Bottom": true}
	hairline := map[string]bool{"Size": true, "Width": true, "Height": true}
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
		// Display-size text must be styled by nativeDisplay: note the
		// spans of its first arguments.
		var display [][2]token.Pos
		ast.Inspect(file, func(n ast.Node) bool {
			if call, ok := n.(*ast.CallExpr); ok {
				if id, ok := call.Fun.(*ast.Ident); ok && id.Name == "nativeDisplay" && len(call.Args) > 0 {
					display = append(display, [2]token.Pos{call.Args[0].Pos(), call.Args[0].End()})
				}
			}
			return true
		})
		inDisplay := func(pos token.Pos) bool {
			for _, d := range display {
				if pos >= d[0] && pos < d[1] {
					return true
				}
			}
			return false
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
				if id, ok := call.Args[0].(*ast.Ident); ok && (id.Name == "tokenSizeH2" || id.Name == "tokenSizeH3") && !inDisplay(call.Pos()) {
					report("%s heading outside nativeDisplay; headings use the display face", id.Name)
				}
			case sel.Sel.Name == "LetterSpacing":
				if _, ok := literal(call.Args[0]); ok {
					report("literal LetterSpacing; tracking belongs to the display face")
				}
			case sel.Sel.Name == "Shadow" && len(call.Args) != 1:
				report("Shadow with literal values; use a tokenShadow elevation")
			case spaceCalls[sel.Sel.Name]:
				for _, arg := range call.Args {
					if v, ok := literal(arg); ok && !onGrid(v) && !(v == 1 && hairline[sel.Sel.Name]) {
						report("%s(%g) is off the 4 px grid", sel.Sel.Name, v)
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
