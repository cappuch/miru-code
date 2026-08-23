# Miru Makefile

.PHONY: build test install clean

build:
	go build -o miru ./cmd/miru

test:
	go test ./...

install:
	go install ./cmd/miru

clean:
	rm -f miru
