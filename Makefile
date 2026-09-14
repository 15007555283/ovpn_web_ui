.PHONY: build test linux dev dev-config
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
dev-config:
	@node -e 'const fs = require("node:fs"); const config = {listen:"127.0.0.1:8080",origin:"http://127.0.0.1:8080",data_dir:require("node:path").join(process.cwd(),"data"),fake:true}; try { fs.writeFileSync("config.json", JSON.stringify(config,null,2)+"\n", {flag:"wx",mode:0o600}); console.log("已生成本地 fake 配置 config.json"); } catch (error) { if (error.code !== "EEXIST") throw error; }'
dev: build dev-config
	./bin/vpn-admin -config config.json
