GO ?= $(CURDIR)/.tools/go/bin/go
# The platforms this build is released for; every reading gate reads both.
PLATFORMS ?= linux darwin
GOFMT ?= $(dir $(GO))gofmt
TEST_TIMEOUT ?= 20m
RACE_TIMEOUT ?= 60m
export GOTOOLCHAIN := local
export GOTELEMETRY := off
export GOCACHE := $(CURDIR)/.cache/go-build
export GOMODCACHE := $(CURDIR)/.cache/go-mod

.PHONY: build test race vet check ci-check fmt schemas schemas-check release-ci-check staticcheck-check vuln-check e2e examples release
build:
	$(GO) build -trimpath -buildvcs=false -o bin/prifly ./cmd/prifly
test:
	$(GO) test -timeout $(TEST_TIMEOUT) ./...
race:
	$(GO) test -race -timeout $(RACE_TIMEOUT) ./...
vet:
	$(GO) vet ./...
# The authority needs cgo to run, but nothing else in this build should need it
# to be readable: a package that cannot be type-checked without the driver is a
# package that has the driver's types in its own contracts.
# A gate that reads only the platform it runs on cannot see a file built for
# the other one. staticcheck joined this gate on 2026-09-17 with its findings
# closed on darwin, and CI was red for eleven hours on a finding in a
# linux-only file that no darwin run could have read. Both released platforms
# are read, and each pass says which one it was.
	@for target in $(PLATFORMS); do \
		CGO_ENABLED=0 GOOS=$$target $(GO) vet ./... || exit 1; \
		echo "vet: $$target read"; \
	done
check: test race vet fmt-check refusal-check staticcheck-check vuln-check schemas-check release-ci-check
ci-check: test vet fmt-check refusal-check staticcheck-check vuln-check schemas-check release-ci-check
fmt:
	$(GO) fmt ./...
# Formatting drifted unnoticed because nothing checked it. A gate that does not
# look is how eleven files ended up unformatted without anyone deciding that.
# A gate that finds nothing and a gate that looked at nothing report the same
# green. Both say how many files they read, so the second cannot hide as the
# first when a path is renamed out from under them.
fmt-check:
	@files=$$(find ./cmd ./internal -name '*.go' -type f | wc -l | tr -d ' '); \
	if [ "$$files" -lt 1 ]; then echo "fmt-check read no Go files under ./cmd ./internal"; exit 1; fi; \
	unformatted=$$($(GOFMT) -l ./cmd ./internal); \
	if [ -n "$$unformatted" ]; then echo "not gofmt-clean:"; echo "$$unformatted"; exit 1; fi; \
	echo "fmt-check: $$files files read, all gofmt-clean"
# A refusal carries its code in a typed Fault, not inside the text of an error
# that every reader has to split apart again. Tests still build text-shaped
# errors on purpose, to prove such an error is still read correctly.
refusal-check:
	@files=$$(find internal cmd -name '*.go' -type f ! -name '*_test.go' | wc -l | tr -d ' '); \
	if [ "$$files" -lt 1 ]; then echo "refusal-check read no Go files under internal cmd"; exit 1; fi; \
	sites=$$(grep -rEn --include='*.go' --exclude='*_test.go' 'errors\.New\("[a-z_]+:|fmt\.Errorf\("[a-z_]+:' internal cmd); status=$$?; \
	if [ $$status -ge 2 ]; then echo "refusal-check could not search: grep exited $$status"; exit 1; fi; \
	if [ -n "$$sites" ]; then echo "refusal code inside error text:"; echo "$$sites"; exit 1; fi; \
	echo "refusal-check: $$files files read, no refusal code inside error text"
# Pinned, run from the module cache: a floating @latest would make the gate
# change under a commit that changed nothing. The first real bug staticcheck
# read here (a shadowed err that left a nil plan for the caller) had passed
# vet, the tests and two reviews. Both print what they read, so a run that
# listed no packages cannot pass as a clean one.
STATICCHECK ?= honnef.co/go/tools/cmd/staticcheck@v0.8.1
GOVULNCHECK ?= golang.org/x/vuln/cmd/govulncheck@v1.8.0
staticcheck-check:
	@packages=$$($(GO) list ./... | wc -l | tr -d ' '); \
	if [ "$$packages" -lt 1 ]; then echo "staticcheck-check listed no packages"; exit 1; fi; \
	$(GO) run $(STATICCHECK) ./... || exit 1; \
	bin="$$(mktemp -d)"; \
	GOBIN="$$bin" $(GO) install $(STATICCHECK) || { rm -rf "$$bin"; exit 1; }; \
	for target in $(PLATFORMS); do \
		CGO_ENABLED=0 GOOS=$$target "$$bin/staticcheck" ./... || { rm -rf "$$bin"; exit 1; }; \
		echo "staticcheck: $$packages packages read for $$target, no findings"; \
	done; \
	rm -rf "$$bin"
vuln-check:
	@packages=$$($(GO) list ./... | wc -l | tr -d ' '); \
	if [ "$$packages" -lt 1 ]; then echo "vuln-check listed no packages"; exit 1; fi; \
	$(GO) run $(GOVULNCHECK) ./... || exit 1; \
	echo "vuln-check: $$packages packages read"
schemas:
	python3 scripts/check-schema.py --go "$(GO)" --write
schemas-check:
	python3 scripts/check-schema.py --go "$(GO)"
release-ci-check:
	python3 -B test/e2e/verify-release-ci.py
# test/e2e/run.sh owns the throwaway HOME and every fixture directory, and
# removes them on exit; a mktemp evaluated here ran on every make invocation
# and its directory was never removed.
e2e: build
	sh test/e2e/run.sh bin/prifly
examples: e2e
release: build
	python3 scripts/release.py --go "$(GO)" --binary bin/prifly
