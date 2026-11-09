package main

import (
	"bytes"
	"os"
	"testing"

	"ytmgo/internal/designtokens"
)

// The app's colours and measures come from the design guidebook's tokens.
// Edit design/tokens.json and run go generate, never the generated file.
func TestDesignTokensAreGenerated(t *testing.T) {
	raw, err := os.ReadFile("design/tokens.json")
	if err != nil {
		t.Fatal(err)
	}
	want, err := designtokens.Generate(raw)
	if err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile("native_tokens_gen.go")
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, want) {
		t.Fatal("native_tokens_gen.go is stale: run go generate")
	}
}
