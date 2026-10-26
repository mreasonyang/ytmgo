.PHONY: dev build test check

dev:
	go tool mygo dev

build:
	CGO_ENABLED=0 go tool mygo build

test:
	go test ./...
	node --test web/*test.mjs

check: test
	go vet ./...
