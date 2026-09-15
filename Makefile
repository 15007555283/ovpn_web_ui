.PHONY: build test linux dev dev-config
VERSION ?= dev
LDFLAGS = -X ovpn-web-ui/internal/version.Value=$(VERSION)
build:
	cd web && npm ci --no-audit --no-fund && npm run build
	mkdir -p bin
	go build -trimpath -ldflags "$(LDFLAGS)" -o bin/vpn-admin .
linux: build
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -ldflags "$(LDFLAGS)" -o bin/vpn-admin-linux-amd64 .
	CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -trimpath -ldflags "$(LDFLAGS)" -o bin/vpn-admin-linux-arm64 .
test:
	go test -race ./...
	go vet ./...
dev-config:
	@node -e 'const fs = require("node:fs"); const config = {listen:"127.0.0.1:8080",origin:"http://127.0.0.1:8080",data_dir:require("node:path").join(process.cwd(),"data"),mode:"demo"}; try { fs.writeFileSync("config.json", "// mode: demo 为演示，production 为生产；两种模式请使用不同 data_dir。\n// OpenVPN 全部配置项及中文注释见 deploy/config.example.jsonc。\n"+JSON.stringify(config,null,2)+"\n", {flag:"wx",mode:0o600}); console.log("已生成本地演示配置 config.json"); } catch (error) { if (error.code !== "EEXIST") throw error; }'
dev: build dev-config
	./bin/vpn-admin -config config.json
