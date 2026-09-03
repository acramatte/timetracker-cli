BINARY := timetracker
BUILD_DIR := bin

.PHONY: fmt vet test acceptance build check clean

fmt:
	gofmt -l -w .

vet:
	go vet ./...

test:
	go test ./...

acceptance:
	go test -tags acceptance ./cmd/timetracker -run 'Test(CLIHelp|PomodoroCommandCompletesAtDeadline)' -count=1

build:
	go build -o $(BUILD_DIR)/$(BINARY) ./cmd/$(BINARY)

check: fmt vet test build

clean:
	rm -rf $(BUILD_DIR)
