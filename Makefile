SHELL := /bin/sh

.PHONY: dev frontend-install frontend-dev frontend-build test build clean

frontend-install:
	cd web && npm install

frontend-dev:
	cd web && npm run dev

dev:
	@mkdir -p internal/httpapi/assets
	@trap 'kill 0' INT TERM EXIT; \
		go run ./cmd/blackhand & \
		(cd web && npm run dev) & \
		wait

build: frontend-install frontend-build
	rm -rf internal/httpapi/assets
	mkdir -p internal/httpapi/assets bin
	cp -R web/dist/. internal/httpapi/assets/
	go build -trimpath -ldflags="-s -w" -o bin/blackhand ./cmd/blackhand

frontend-build:
	cd web && npm run build

test:
	go test ./...
	cd web && npm run check

clean:
	rm -rf bin web/dist internal/httpapi/assets
