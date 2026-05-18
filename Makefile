.PHONY: test fmt run smoke-dry-run install

test:
	go test ./...

fmt:
	gofmt -w cmd internal

run:
	go run ./cmd/posters

smoke-dry-run:
	go run ./cmd/posters -config-dir /tmp/posters-smoke -dry-run

install:
	go install ./cmd/posters
