APP := ptpatronus
AGENT := ptpatronus-agent

.PHONY: tidy build run dev test fmt vet docker up down clean build-agent build-agent-linux-amd64 build-agent-linux-arm64 build-agent-all

tidy:
	go mod tidy

fmt:
	go fmt ./...

vet:
	go vet ./...

build:
	go build -trimpath -ldflags="-s -w" -o bin/$(APP) ./cmd/ptpatronus

# 监控 agent（独立轻量二进制，跑在被监控机，WS 上报；CGO_ENABLED=0 纯 Go 利于交叉编译）
build-agent:
	CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o bin/$(AGENT) ./cmd/ptpatronus-agent

build-agent-linux-amd64:
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -ldflags="-s -w" -o bin/$(AGENT)-linux-amd64 ./cmd/ptpatronus-agent

build-agent-linux-arm64:
	CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -trimpath -ldflags="-s -w" -o bin/$(AGENT)-linux-arm64 ./cmd/ptpatronus-agent

build-agent-all: build-agent-linux-amd64 build-agent-linux-arm64

run: build
	./bin/$(APP)

dev:
	go run ./cmd/ptpatronus

test:
	go test ./...

docker:
	docker build -t $(APP):latest .

up:
	cd .. && docker compose up -d --build

down:
	cd .. && docker compose down

clean:
	rm -rf bin data
