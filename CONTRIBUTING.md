# Contributions and licensing

Contributions of original code, documentation and project assets are submitted
under Apache-2.0, unless explicitly agreed otherwise. See `LICENSE` and `NOTICE`.
New original source files may use `SPDX-License-Identifier: Apache-2.0` in the
appropriate comment syntax.

Keep upstream licenses and attribution when importing or adapting third-party
code. Record its origin and applicable terms in `THIRD_PARTY_NOTICES.md`; do not
replace third-party license headers with the project license. Generated bundles
containing third-party code retain the upstream terms for those portions.

When changing dependencies, regenerate and inspect the distribution resources
with `make check-licenses`, and run `make test-licenses`. Supply corresponding
source and build instructions for components whose licenses require them.
