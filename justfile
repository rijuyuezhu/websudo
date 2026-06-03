default:
    @just --list

fmt:
	gofmt -w $(git ls-files '*.go')
	npm --prefix web run format

fmt-check:
	@files="$(gofmt -l $(git ls-files '*.go'))"; status=$?; if [ "$status" -ne 0 ]; then exit "$status"; fi; if [ -n "$files" ]; then printf '%s\n' "$files"; exit 1; fi
	npm --prefix web run format:check

lint:
	golangci-lint run ./cmd/... ./internal/... ./tests/...
	npm --prefix web run lint
	npm --prefix web run typecheck

test:
	go test ./cmd/... ./internal/... ./tests/...
	npm --prefix web run typecheck

web-build:
	rm -rf internal/approverd/static/app/assets internal/approverd/static/app/index.html
	npm --prefix web run build

build: web-build
	mkdir -p build
	go build -o build/websudo ./cmd/websudo
	go build -o build/websudo-askpass ./cmd/websudo-askpass
	go build -o build/websudo-approverd ./cmd/websudo-approverd
	go build -o build/websudo-rootd ./cmd/websudo-rootd

clean:
	rm -rf -- build
	rm -rf -- internal/approverd/static/app/assets internal/approverd/static/app/index.html
