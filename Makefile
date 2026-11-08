.PHONY: all gate test race fmt lint vuln sec integration integration-down fuzz clean

GO       ?= go
FUZZTIME ?= 30s

# Same commands CI runs. Keep the two in step: a check that only fails in CI
# is a check nobody can debug.
all: fmt lint test race

gate: all vuln sec fuzz

test:
	$(GO) test ./...

race:
	$(GO) test -race -count=1 ./...

fmt:
	gofmt -l -s -w .

lint:
	golangci-lint run ./...

vuln:
	$(GO) run golang.org/x/vuln/cmd/govulncheck@latest ./...

sec:
	$(GO) run github.com/securego/gosec/v2/cmd/gosec@latest ./...

# --wait blocks until the compose healthcheck passes. Without it the test run
# races container startup.
integration:
	docker compose up -d --wait
	$(GO) test -tags=integration ./...

integration-down:
	docker compose down -v

# go test -fuzz takes exactly one target per invocation, so enumerate them.
# Runs every Fuzz* target found in the module.
fuzz:
	@for pkg in $$($(GO) list ./...); do \
		for target in $$($(GO) test -list 'Fuzz.*' $$pkg 2>/dev/null | grep '^Fuzz' || true); do \
			echo "==> fuzzing $$target in $$pkg"; \
			$(GO) test -run '^$$' -fuzz "^$$target$$" -fuzztime=$(FUZZTIME) $$pkg || exit 1; \
		done; \
	done

clean:
	$(GO) clean -testcache
	rm -f coverage.txt coverage.html
