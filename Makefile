.PHONY: dev runtime build test test-runtime check

dev: runtime
	YTMGO_RUNTIME_DIR="$(CURDIR)/build/runtime" go tool mygo dev

runtime:
	python3 scripts/runtime/prepare.py

build: runtime
	CGO_ENABLED=0 go tool mygo build

test:
	go test ./...

test-runtime: runtime
	python3 scripts/runtime/test_runtime_manifest.py
	python3 scripts/runtime/test_protocol_version.py
	python3 scripts/runtime/run_tests.py

check: test test-runtime
	go vet ./...
	go tool mygo i18n check
