.PHONY: dev runtime login licenses check-licenses test-licenses build build-app test test-runtime test-login check

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

licenses: runtime login
	python3 scripts/licensing/prepare.py

check-licenses: licenses
	python3 scripts/licensing/prepare.py --verify

test-licenses:
	python3 -m unittest discover -s scripts/licensing -p 'test_*.py'

build: licenses
	python3 scripts/login/prepare.py --verify
	CGO_ENABLED=0 go tool mygo build
	python3 scripts/licensing/prepare.py --verify-build

build-app: licenses
	python3 scripts/login/prepare.py --verify
	CGO_ENABLED=0 go tool mygo build -skip-dmg
	python3 scripts/licensing/prepare.py --verify-build
	CGO_ENABLED=0 go build -o build/release/update-package ./cmd/update-package

test:
	go test ./...

test-runtime: runtime
	python3 scripts/runtime/test_runtime_manifest.py
	python3 scripts/runtime/test_protocol_version.py
	python3 scripts/runtime/run_tests.py

test-login:
	python3 -m unittest discover -s scripts/login -p 'test_*.py'

check: test test-runtime test-login test-licenses check-licenses
	go vet ./...
	go tool mygo i18n check

# Website: render the real UI with the fictional site catalogue, encode the
# screenshots, then build one static page per language into site/dist.
site-shots:
	rm -rf build/site-shots
	YTMGO_SITE_SHOTS_DIR=$(CURDIR)/build/site-shots go test -run TestSiteScreenshots -count=1 .
	python3 -I site/tools/encode_shots.py build/site-shots

site:
	go run ./site/gen

.PHONY: site site-shots
