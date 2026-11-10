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
// Corners come from the radius tokens (or are half a size: a circle or a
// capsule), including the corners handed to the artwork helpers, and tips
// go through nativeTip, as MyGo's own tooltip draws off the scale.
// artworkHelpers take a corner as their fourth argument.
var artworkHelpers = map[string]bool{"nativeArtwork": true, "nativePlayerArtwork": true, "nativeCover": true}

func TestNativeViewsUseDesignTokens(t *testing.T) {
	onGrid := func(v float64) bool { return v == math.Trunc(v) && int(v)%4 == 0 }
	spaceCalls := map[string]bool{"Padding": true, "PaddingX": true, "PaddingY": true, "PaddingStart": true, "PaddingEnd": true,
		"PaddingTop": true, "PaddingBottom": true, "Gap": true, "Margin": true, "MarginTop": true, "MarginBottom": true,
		"Size": true, "Width": true, "Height": true, "MinWidth": true, "MinHeight": true, "MaxWidth": true, "MaxHeight": true,
		"Left": true, "Top": true, "Right": true, "Bottom": true}
	hairline := map[string]bool{"Size": true, "Width": true, "Height": true}
	// Every file of the main package: view code is not only in native_*.go.
	files, err := filepath.Glob("*.go")
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
			at := fset.Position(call.Pos())
			if id, ok := call.Fun.(*ast.Ident); ok && artworkHelpers[id.Name] && len(call.Args) > 3 {
				// Not even 0: art flush with a rounded container takes the
				// container's corners itself, as the GPU renderer rounds only
				// the innermost clip.
				if lit, ok := call.Args[3].(*ast.BasicLit); ok {
					problems = append(problems, fmt.Sprintf("%s:%d: %s with corner %s; use a radius token", at.Filename, at.Line, id.Name, lit.Value))
				}
			}
			sel, ok := call.Fun.(*ast.SelectorExpr)
			if !ok {
				return true
			}
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
			case (sel.Sel.Name == "Alpha" || sel.Sel.Name == "Mix") && len(call.Args) >= 1:
				// Translucent colours are roles (line, hover, track, veil…);
				// only fading to nothing and the tokens' washes are computed.
				if v, ok := literal(call.Args[len(call.Args)-1]); ok && v != 0 {
					report("%s(%g); use a colour role", sel.Sel.Name, v)
				}
			case sel.Sel.Name == "Radius":
				for _, arg := range call.Args {
					if v, ok := literal(arg); ok && v != 0 {
						report("Radius(%g); use a radius token", v)
					}
				}
			case sel.Sel.Name == "Tooltip" && len(call.Args) == 1:
				report("Tooltip; use nativeTip, which draws the guidebook's tip")
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

// Every scroller draws its bars through nativeScrollbar (or
// nativeScrollEdges, which calls it): the theme hides MyGo's own, so one
// that didn't would show no bar at all.
func TestScrollersDrawTheirBars(t *testing.T) {
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	fset := token.NewFileSet()
	for _, name := range files {
		if strings.HasSuffix(name, "_test.go") {
			continue
		}
		file, err := parser.ParseFile(fset, name, nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Body == nil {
				continue
			}
			scrollers := map[string]token.Pos{}
			barred := map[string]bool{}
			ast.Inspect(fn.Body, func(n ast.Node) bool {
				switch n := n.(type) {
				case *ast.AssignStmt:
					if id, ok := n.Lhs[0].(*ast.Ident); ok && len(n.Rhs) == 1 && createsScroller(n.Rhs[0]) {
						scrollers[id.Name] = n.Pos()
					}
				case *ast.CallExpr:
					if id, ok := n.Fun.(*ast.Ident); ok && (id.Name == "nativeScrollbar" || id.Name == "nativeScrollEdges") && len(n.Args) > 1 {
						if arg, ok := n.Args[1].(*ast.Ident); ok {
							barred[arg.Name] = true
						}
					}
				}
				return true
			})
			for v, pos := range scrollers {
				if !barred[v] {
					t.Errorf("%s: scroller %s draws no bar; call nativeScrollbar", fset.Position(pos), v)
				}
			}
		}
	}
}

// createsScroller reports whether e builds a scroller: a chain starting at
// ui.Scroll, ui.ScrollHorizontal or ui.ScrollBoth.
func createsScroller(e ast.Expr) bool {
	for {
		call, ok := e.(*ast.CallExpr)
		if !ok {
			return false
		}
		sel, ok := call.Fun.(*ast.SelectorExpr)
		if !ok {
			return false
		}
		if pkg, ok := sel.X.(*ast.Ident); ok && pkg.Name == "ui" {
			return strings.HasPrefix(sel.Sel.Name, "Scroll")
		}
		e = sel.X
	}
}
