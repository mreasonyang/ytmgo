// Command gen writes the app's native_tokens_gen.go from design/tokens.json.
//
//	go run ./design/gen
package main

import (
	"log"
	"os"

	"ytmgo/internal/designtokens"
)

func main() {
	raw, err := os.ReadFile("design/tokens.json")
	if err != nil {
		log.Fatal(err)
	}
	src, err := designtokens.Generate(raw)
	if err != nil {
		log.Fatal(err)
	}
	if err := os.WriteFile("native_tokens_gen.go", src, 0o644); err != nil {
		log.Fatal(err)
	}
}
