BINARY := git-updater
GO     ?= go
PREFIX ?= $(HOME)/.local/bin

.PHONY: all build test vet install clean

all: build

build:
	$(GO) build -trimpath -o $(BINARY) .

test:
	$(GO) test ./...

vet:
	$(GO) vet ./...

install: build
	install -d $(PREFIX)
	install -m 0755 $(BINARY) $(PREFIX)/$(BINARY)

clean:
	rm -f $(BINARY)
