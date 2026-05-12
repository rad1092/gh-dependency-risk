.PHONY: build build-release test workflow-example

VERSION ?= dev
COMMIT ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo none)
DATE ?= $(shell git show -s --format=%cI HEAD 2>/dev/null || echo unknown)
LDFLAGS = -s -w -X github.com/rad1092/gh-dependency-risk/cmd.version=$(VERSION) -X github.com/rad1092/gh-dependency-risk/cmd.commit=$(COMMIT) -X github.com/rad1092/gh-dependency-risk/cmd.date=$(DATE)

build:
	go build -ldflags "$(LDFLAGS)" -o gh-dep-risk .

build-release:
	go build -trimpath -ldflags "$(LDFLAGS)" -o gh-dep-risk .

test:
	go test ./...

workflow-example:
	@printf '%s\n' \
		'gh workflow run .github/workflows/dep-risk-manual.yml -f pr=123' \
		'gh workflow run .github/workflows/dep-risk-manual.yml -f pr=https://github.com/OWNER/REPO/pull/123 -f comment=true' \
		'gh run watch'
