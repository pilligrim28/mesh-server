# Сборка сервера под все платформы (Go 1.25+, CGO не нужен)
VERSION ?= 3.0.0
LDFLAGS = -s -w -X main.version=$(VERSION)

.PHONY: build test dist run-sim
build:
	go build -ldflags "$(LDFLAGS)" -o mesh-server ./cmd/mesh-server
test:
	go vet ./... && go test ./...
run-sim:
	TRANSPORT=sim go run ./cmd/mesh-server
dist:
	mkdir -p dist
	GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go build -ldflags "$(LDFLAGS)" -o dist/mesh-server-windows-amd64.exe ./cmd/mesh-server
	GOOS=linux   GOARCH=amd64 CGO_ENABLED=0 go build -ldflags "$(LDFLAGS)" -o dist/mesh-server-linux-amd64 ./cmd/mesh-server
	GOOS=linux   GOARCH=arm64 CGO_ENABLED=0 go build -ldflags "$(LDFLAGS)" -o dist/mesh-server-linux-arm64 ./cmd/mesh-server
	GOOS=linux   GOARCH=arm   GOARM=7 CGO_ENABLED=0 go build -ldflags "$(LDFLAGS)" -o dist/mesh-server-linux-armv7 ./cmd/mesh-server
