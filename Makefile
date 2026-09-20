.PHONY: check build web clean

check:
	go test -race ./...
	go vet ./...
	cd web && npm ci && npm run build

build: web
	CGO_ENABLED=0 go build -buildvcs=false -trimpath -o bin/gateway ./cmd/gateway

web:
	cd web && npm ci && npm run build

clean:
	rm -rf bin web/dist web/node_modules web/*.tsbuildinfo

