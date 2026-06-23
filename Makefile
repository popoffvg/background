.PHONY: build test vet run clean

BINARY=./bin/bg

build:
	go build -o $(BINARY) .

test:
	go test ./...

vet:
	go vet ./...

run: build
	$(BINARY)

clean:
	rm -f $(BINARY)
