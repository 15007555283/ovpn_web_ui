.PHONY: build test linux dev
build:
	cd web && npm ci --no-audit --no-fund && npm run build
	mkdir -p bin
	go build -trimpath -o bin/vpn-admin ./cmd/vpn-admin
linux: build
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -o bin/vpn-admin-linux-amd64 ./cmd/vpn-admin
	CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -trimpath -o bin/vpn-admin-linux-arm64 ./cmd/vpn-admin
test:
	go test -race ./...
	go vet ./...
dev: build
	./bin/vpn-admin -config config.json
