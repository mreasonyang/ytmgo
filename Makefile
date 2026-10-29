.PHONY: dev runtime login build build-app test test-runtime test-login check

LOGIN_OS := $(shell go env GOOS)
ifeq ($(LOGIN_OS),darwin)
LOGIN_HELPER = $(CURDIR)/build/login/ytmgo Account Login.app/Contents/MacOS/ytmgo-account-login
else
LOGIN_HELPER = $(CURDIR)/build/login/ytmgo-account-login$(if $(filter windows,$(LOGIN_OS)),.exe,)
endif

dev: runtime login
	YTMGO_RUNTIME_DIR="$(CURDIR)/build/runtime" YTMGO_LOGIN_HELPER="$(LOGIN_HELPER)" go tool mygo dev

runtime:
	python3 scripts/runtime/prepare.py

login:
	python3 scripts/login/prepare.py

build: runtime login
	python3 scripts/login/prepare.py --verify
	CGO_ENABLED=0 go tool mygo build

build-app: runtime login
	python3 scripts/login/prepare.py --verify
	CGO_ENABLED=0 go tool mygo build -skip-dmg

test:
	go test ./...

test-runtime: runtime
	python3 scripts/runtime/test_runtime_manifest.py
	python3 scripts/runtime/test_protocol_version.py
	python3 scripts/runtime/run_tests.py

test-login:
	python3 -m unittest discover -s scripts/login -p 'test_*.py'

check: test test-runtime test-login
	go vet ./...
	go tool mygo i18n check
