default:
    @just --list

fmt:
	gofmt -w $(git ls-files '*.go')

fmt-check:
	@files="$(gofmt -l $(git ls-files '*.go'))"; status=$?; if [ "$status" -ne 0 ]; then exit "$status"; fi; if [ -n "$files" ]; then printf '%s\n' "$files"; exit 1; fi

lint:
	golangci-lint run ./cmd/... ./internal/...

test:
	go test ./cmd/... ./internal/...
	sh packaging/scripts/verify-aur-package-metadata.sh

build:
	mkdir -p build
	go build -o build/websudo ./cmd/websudo
	go build -o build/websudo-askpass ./cmd/websudo-askpass
	go build -o build/websudo-approverd ./cmd/websudo-approverd

clean:
	rm -rf -- build
