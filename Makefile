GO ?= $(CURDIR)/.tools/go/bin/go
GOFMT ?= $(dir $(GO))gofmt
TEST_TIMEOUT ?= 20m
RACE_TIMEOUT ?= 30m
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
check: test race vet fmt-check schemas-check release-ci-check
ci-check: test vet fmt-check schemas-check release-ci-check
fmt:
	$(GO) fmt ./...
# Formatting drifted unnoticed because nothing checked it. A gate that does not
# look is how eleven files ended up unformatted without anyone deciding that.
fmt-check:
	@unformatted=$$($(GOFMT) -l ./cmd ./internal); \
	if [ -n "$$unformatted" ]; then echo "not gofmt-clean:"; echo "$$unformatted"; exit 1; fi
schemas:
	python3 scripts/check-schema.py --go "$(GO)" --write
schemas-check:
	python3 scripts/check-schema.py --go "$(GO)"
release-ci-check:
	python3 -B test/e2e/verify-release-ci.py
e2e: build
	python3 -B test/e2e/test_examples.py
	python3 -B test/e2e/verify-authoring.py --binary bin/prifly
	python3 -B test/e2e/verify-cli.py --binary bin/prifly
	python3 -B test/e2e/verify-core.py --binary bin/prifly
	python3 -B test/e2e/verify-context.py --binary bin/prifly
examples: e2e
release: build
	python3 scripts/release.py --go "$(GO)" --binary bin/prifly
