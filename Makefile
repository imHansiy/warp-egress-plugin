PLUGIN_NAME := warp-egress
OUT_DIR := bin
PKG := ./cmd/warp-egress

.PHONY: test build build-linux-amd64 clean install registry-check release-linux-amd64

test:
	go test ./...

registry-check:
	python3 scripts/validate-registry.py

build: test
	mkdir -p $(OUT_DIR)
	CGO_ENABLED=1 go build -trimpath -buildmode=c-shared -ldflags="-s -w" -o $(OUT_DIR)/$(PLUGIN_NAME).so $(PKG)

build-linux-amd64: test
	mkdir -p $(OUT_DIR)
	GOOS=linux GOARCH=amd64 CGO_ENABLED=1 go build -trimpath -buildmode=c-shared -ldflags="-s -w" -o $(OUT_DIR)/$(PLUGIN_NAME).so $(PKG)

install: build
	@test -n "$(PLUGIN_DIR)" || (echo "Usage: make install PLUGIN_DIR=/path/to/CLIProxyAPI/plugins" && exit 1)
	install -d "$(PLUGIN_DIR)"
	install -m 0755 $(OUT_DIR)/$(PLUGIN_NAME).so "$(PLUGIN_DIR)/$(PLUGIN_NAME).so"

clean:
	rm -rf $(OUT_DIR)

.PHONY: smoke
smoke: build
	python3 scripts/abi_smoke_test.py $(OUT_DIR)/$(PLUGIN_NAME).so

release-linux-amd64: build-linux-amd64 registry-check
	rm -rf dist/release-linux-amd64
	mkdir -p dist/release-linux-amd64 dist
	cp $(OUT_DIR)/$(PLUGIN_NAME).so dist/release-linux-amd64/$(PLUGIN_NAME).so
	cd dist/release-linux-amd64 && zip -q ../$(PLUGIN_NAME)_0.2.0_linux_amd64.zip $(PLUGIN_NAME).so
	cd dist && sha256sum $(PLUGIN_NAME)_0.2.0_linux_amd64.zip > checksums.txt
