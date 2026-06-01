fmt:
	go fmt ./...

test:
	go test ./...

build:
	mkdir -p build
	go build -o build/websudo ./cmd/websudo
	go build -o build/websudo-askpass ./cmd/websudo-askpass
	go build -o build/websudo-approverd ./cmd/websudo-approverd
	go build -o build/websudo-rootd ./cmd/websudo-rootd

clean:
	rm -rf -- build
