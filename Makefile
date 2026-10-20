PREFIX ?= $(HOME)/.local
BINDIR ?= $(PREFIX)/bin

.PHONY: build install test

build:
	go build -o dch .

install: build
	install -d "$(BINDIR)"
	install -m 755 dch "$(BINDIR)/dch"

test:
	go test ./...
