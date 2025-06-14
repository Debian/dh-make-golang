.PHONY: build test lint fmt vet clean install man help

BINARY_NAME=dh-make-golang
GO=go
GOFMT=gofmt
PANDOC=pandoc

help:
	@echo "Available targets:"
	@echo "  build      - Build the binary"
	@echo "  test       - Run tests"
	@echo "  lint       - Run linting checks"
	@echo "  fmt        - Format code"
	@echo "  vet        - Run go vet"
	@echo "  clean      - Remove build artifacts"
	@echo "  install    - Install binary to /usr/local/bin"
	@echo "  man        - Generate man page"

build:
	$(GO) build -o $(BINARY_NAME)

test:
	$(GO) test -v ./...

lint: fmt vet

fmt:
	$(GOFMT) -w -s .

vet:
	$(GO) vet ./...

clean:
	rm -f $(BINARY_NAME)
	rm -f $(BINARY_NAME).1

install: build
	install -m755 $(BINARY_NAME) /usr/local/bin/

man:
	$(PANDOC) -f markdown -t man -s dh-make-golang.md -o $(BINARY_NAME).1
