GO ?= go
export GOTOOLCHAIN := go1.27.3
export GOPATH := $(CURDIR)/.cache/gopath
export GOCACHE := $(CURDIR)/.cache/gobuild
.PHONY: setup build lint test bench demo scan
setup:
	python3 -m venv .venv
	.venv/bin/python -m pip install -r requirements-dev.txt
	$(GO) mod download
	npm ci --prefix apps/web
build:
	mkdir -p bin
	$(GO) build -trimpath -o bin/tidal ./cmd/tidal
	npm run build --prefix apps/web
lint:
	python3 scripts/check_go_format.py
	$(GO) vet ./...
	.venv/bin/ruff check scripts
	.venv/bin/ruff format --check scripts
	npm run lint --prefix apps/web
test:
	$(GO) test -race -count=1 -v ./... > docs/results/tests.log
bench: build
	python3 scripts/trace_provenance.py > docs/results/trace-provenance.log
	./bin/tidal -bench > docs/results/bench-run.log
	$(GO) test ./internal/cache -run '^$$' -bench Benchmark -benchmem -count=3 > docs/results/core-bench.log
demo: setup build
	./bin/tidal -addr $${TIDAL_ADDR:-127.0.0.1:8095}
scan:
	$(GO) run golang.org/x/vuln/cmd/govulncheck@v1.9.0 ./...
	npm audit --prefix apps/web --audit-level=low
	.venv/bin/python -m pip_audit -r requirements-dev.txt -r requirements-browser.txt --progress-spinner off
