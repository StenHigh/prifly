GO ?= $(CURDIR)/.tools/go/bin/go
GOFMT ?= $(dir $(GO))gofmt
TEST_TIMEOUT ?= 20m
RACE_TIMEOUT ?= 60m
export GOTOOLCHAIN := local
export GOTELEMETRY := off
export GOCACHE := $(CURDIR)/.cache/go-build
export GOMODCACHE := $(CURDIR)/.cache/go-mod

.PHONY: build test race vet check ci-check fmt schemas schemas-check release-ci-check e2e examples release
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
	CGO_ENABLED=0 $(GO) vet ./...
check: test race vet fmt-check refusal-check schemas-check release-ci-check
ci-check: test vet fmt-check refusal-check schemas-check release-ci-check
fmt:
	$(GO) fmt ./...
# Formatting drifted unnoticed because nothing checked it. A gate that does not
# look is how eleven files ended up unformatted without anyone deciding that.
fmt-check:
	@unformatted=$$($(GOFMT) -l ./cmd ./internal); \
	if [ -n "$$unformatted" ]; then echo "not gofmt-clean:"; echo "$$unformatted"; exit 1; fi
# A refusal carries its code in a typed Fault, not inside the text of an error
# that every reader has to split apart again. Tests still build text-shaped
# errors on purpose, to prove such an error is still read correctly.
refusal-check:
	@sites=$$(rg -n 'errors\.New\("[a-z_]+:|fmt\.Errorf\("[a-z_]+:' internal cmd --glob '!*_test.go' || true); \
	if [ -n "$$sites" ]; then echo "refusal code inside error text:"; echo "$$sites"; exit 1; fi
schemas:
	python3 scripts/check-schema.py --go "$(GO)" --write
schemas-check:
	python3 scripts/check-schema.py --go "$(GO)"
release-ci-check:
	python3 -B test/e2e/verify-release-ci.py
e2e: build
	sh test/e2e/verify-install.sh
	python3 -B test/e2e/test_examples.py
	python3 -B test/e2e/verify-authoring.py --binary bin/prifly
	python3 -B test/e2e/verify-cli.py --binary bin/prifly
	python3 -B test/e2e/verify-core.py --binary bin/prifly
	python3 -B test/e2e/verify-context.py --binary bin/prifly
examples: e2e
release: build
	python3 scripts/release.py --go "$(GO)" --binary bin/prifly
