all: build

GIT_VERSION := $(shell git rev-parse --short HEAD)

build: $(wildcard *.go)
	GOOS=linux  GOARCH=amd64 go build -ldflags -w -o build/ipswlite-linux-amd64
	GOOS=darwin GOARCH=amd64 go build -ldflags -w -o build/ipswlite-darwin-amd64
	GOOS=windows GOARCH=386 go build -ldflags -w -o build/ipswlite-windows-x32.exe

archive: build
	cp README.md build
	cd build
	zip ipswlite-win-$(GIT_VERSION).zip build/ipswlite-windows-x32.exe README.md
	zip ipswlite-osx-$(GIT_VERSION).zip build/ipswlite-darwin-amd64 README.md
	zip ipswlite-lin-$(GIT_VERSION).zip build/ipswlite-linux-amd64 README.md
	cd ..

clean:
	rm -rf build
