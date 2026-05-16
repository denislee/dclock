BINARY := dclock
GO     ?= go

.PHONY: all build run clean tidy fmt vet

all: build

build:
	$(GO) build -o $(BINARY) ./...

run: build
	./$(BINARY)

tidy:
	$(GO) mod tidy

fmt:
	$(GO) fmt ./...

vet:
	$(GO) vet ./...

clean:
	rm -f $(BINARY)
