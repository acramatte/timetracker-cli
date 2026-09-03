BINARY := timetracker
BUILD_DIR := bin

.PHONY: fmt vet lint test acceptance build check clean

fmt:
	gofmt -l -w .

vet:
	go vet ./...

lint: vet
	@command -v staticcheck >/dev/null || { echo "staticcheck not on PATH; go install honnef.co/go/tools/cmd/staticcheck@latest"; exit 1; }
	staticcheck -checks 'all,-ST1000,-ST1003,-ST1020' ./...

test:
	go test ./...

acceptance:
	go test -tags acceptance ./cmd/timetracker -run 'Test(CLIHelp|PomodoroCommandCompletesAtDeadline)' -count=1

build:
	go build -o $(BUILD_DIR)/$(BINARY) ./cmd/$(BINARY)

check: fmt lint test build

clean:
	rm -rf $(BUILD_DIR)
