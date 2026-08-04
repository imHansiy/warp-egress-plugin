PLUGIN_NAME := warp-egress
OUT_DIR := bin
PKG := ./cmd/warp-egress

# 版本号默认取自最近的 git tag（去掉前导 v），可通过 VERSION=xxx 覆盖
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null | sed 's/^v//')

# 发布目标平台，可通过 GOOS/GOARCH/CC 覆盖
GOOS ?= linux
GOARCH ?= amd64
CC ?= gcc

.PHONY: test build build-linux-amd64 clean install registry-check smoke release release-linux-amd64 embedded-tools

# 下载 wgcf/wireproxy 官方二进制供 go:embed 内嵌（产物在 cmd/warp-egress/embedded_tools/，不入库）
embedded-tools:
	./scripts/download-tools.sh

test:
	go test ./...

registry-check:
	python3 scripts/validate-registry.py

build: embedded-tools test
	mkdir -p $(OUT_DIR)
	CGO_ENABLED=1 go build -trimpath -buildmode=c-shared -ldflags="-s -w" -o $(OUT_DIR)/$(PLUGIN_NAME).so $(PKG)

build-linux-amd64: embedded-tools test
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

# 参数化发布打包，产物符合 CLIProxyAPI 插件商店格式：
#   <pluginID>_<version>_<goos>_<goarch>.zip + checksums.txt
# 用法：
#   make release VERSION=0.2.0
#   make release GOOS=linux GOARCH=arm64 CC=aarch64-linux-gnu-gcc VERSION=0.2.0
release: embedded-tools test registry-check
	rm -rf dist/release-$(GOOS)-$(GOARCH)
	mkdir -p dist/release-$(GOOS)-$(GOARCH) dist
	CC="$(CC)" CGO_ENABLED=1 GOOS=$(GOOS) GOARCH=$(GOARCH) go build -trimpath -buildmode=c-shared -ldflags="-s -w" -o dist/release-$(GOOS)-$(GOARCH)/$(PLUGIN_NAME).so $(PKG)
	cd dist/release-$(GOOS)-$(GOARCH) && zip -q ../$(PLUGIN_NAME)_$(VERSION)_$(GOOS)_$(GOARCH).zip $(PLUGIN_NAME).so
	cd dist && sha256sum $(PLUGIN_NAME)_$(VERSION)_$(GOOS)_$(GOARCH).zip >> checksums.txt

# 兼容旧目标：Linux AMD64 发布
release-linux-amd64:
	$(MAKE) release GOOS=linux GOARCH=amd64 VERSION=$(VERSION)
