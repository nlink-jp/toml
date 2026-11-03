# toml is a library: there is no binary. `build` only compiles the packages,
# which leaves nothing behind, so it is safe to run anywhere.

.PHONY: build test vet clean

build:
	go build ./...

test:
	go test ./...

vet:
	go vet ./...

clean:
	rm -rf dist
