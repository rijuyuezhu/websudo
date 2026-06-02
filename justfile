default:
    @just --list

fmt:
	go fmt ./...
	npm --prefix web run typecheck

test:
	go test ./...
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
