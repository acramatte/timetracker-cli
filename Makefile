BINARY := timetracker
BUILD_DIR := bin

.PHONY: fmt vet test build check clean

fmt:
	gofmt -l -w .

vet:
	go vet ./...

test:
	go test ./...

build:
	go build -o $(BUILD_DIR)/$(BINARY) ./cmd/$(BINARY)

check: fmt vet test build

clean:
	rm -rf $(BUILD_DIR)
