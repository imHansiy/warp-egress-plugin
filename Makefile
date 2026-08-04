PLUGIN_NAME := warp-egress
OUT_DIR := bin
PKG := ./cmd/warp-egress
IMPORT_PKG := warp-egress-plugin/cmd/warp-egress

# 版本号默认取自最近的 git tag（去掉前导 v），可通过 VERSION=xxx 覆盖
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null | sed 's/^v//')
# 构建时注入插件版本号（单一来源），未注入时插件内显示 dev
VERSION_LDFLAGS := -X $(IMPORT_PKG)/version.Version=$(VERSION)

# 发布目标平台，可通过 GOOS/GOARCH/CC 覆盖
GOOS ?= linux
GOARCH ?= amd64
CC ?= gcc

# 各平台共享库文件名（CGO c-shared 输出扩展名随 GOOS 变化）
LIBEXT := so
ifeq ($(GOOS),windows)
LIBEXT := dll
endif
ifeq ($(GOOS),darwin)
LIBEXT := dylib
endif

TOOLS_PLATFORM := $(GOOS)_$(GOARCH)

.PHONY: test build build-linux-amd64 build-windows-amd64 build-darwin-amd64 build-darwin-arm64 build-linux-arm64 clean install registry-check smoke release release-linux-amd64 release-windows-amd64 release-darwin-amd64 release-darwin-arm64 embedded-tools

# 下载 wgcf/wireproxy 官方二进制供 go:embed 内嵌（产物在 cmd/warp-egress/embedded_tools/，不入库）
embedded-tools:
	GOOS_ARCH=$(TOOLS_PLATFORM) ./scripts/download-tools.sh

test:
	go test ./...

registry-check:
	python3 scripts/validate-registry.py

# 通用构建：GOOS/GOARCH/CC 覆盖生效，输出扩展名随平台
build: embedded-tools test
	mkdir -p $(OUT_DIR)
	CGO_ENABLED=1 go build -trimpath -buildmode=c-shared -ldflags="-s -w $(VERSION_LDFLAGS)" -o $(OUT_DIR)/$(PLUGIN_NAME).$(LIBEXT) $(PKG)

build-linux-amd64: embedded-tools test
	mkdir -p $(OUT_DIR)
	GOOS=linux GOARCH=amd64 CGO_ENABLED=1 go build -trimpath -buildmode=c-shared -ldflags="-s -w $(VERSION_LDFLAGS)" -o $(OUT_DIR)/$(PLUGIN_NAME).so $(PKG)

build-linux-arm64: embedded-tools test
	mkdir -p $(OUT_DIR)
	CC=aarch64-linux-gnu-gcc GOOS=linux GOARCH=arm64 CGO_ENABLED=1 go build -trimpath -buildmode=c-shared -ldflags="-s -w $(VERSION_LDFLAGS)" -o $(OUT_DIR)/$(PLUGIN_NAME).so $(PKG)

build-windows-amd64: embedded-tools test
	mkdir -p $(OUT_DIR)
	CC=x86_64-w64-mingw32-gcc GOOS=windows GOARCH=amd64 CGO_ENABLED=1 go build -trimpath -buildmode=c-shared -ldflags="-s -w $(VERSION_LDFLAGS)" -o $(OUT_DIR)/$(PLUGIN_NAME).dll $(PKG)

build-darwin-amd64: embedded-tools test
	mkdir -p $(OUT_DIR)
	GOOS=darwin GOARCH=amd64 CGO_ENABLED=1 go build -trimpath -buildmode=c-shared -ldflags="-s -w $(VERSION_LDFLAGS)" -o $(OUT_DIR)/$(PLUGIN_NAME).dylib $(PKG)

build-darwin-arm64: embedded-tools test
	mkdir -p $(OUT_DIR)
	GOOS=darwin GOARCH=arm64 CGO_ENABLED=1 go build -trimpath -buildmode=c-shared -ldflags="-s -w $(VERSION_LDFLAGS)" -o $(OUT_DIR)/$(PLUGIN_NAME).dylib $(PKG)

install: build
	@test -n "$(PLUGIN_DIR)" || (echo "Usage: make install PLUGIN_DIR=/path/to/CLIProxyAPI/plugins" && exit 1)
	install -d "$(PLUGIN_DIR)"
	install -m 0755 $(OUT_DIR)/$(PLUGIN_NAME).$(LIBEXT) "$(PLUGIN_DIR)/$(PLUGIN_NAME).$(LIBEXT)"

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
#   make release GOOS=windows GOARCH=amd64 CC=x86_64-w64-mingw32-gcc VERSION=0.2.0
#   make release GOOS=darwin GOARCH=arm64 VERSION=0.2.0   # 需在 macOS 主机上构建
release: embedded-tools test registry-check
	rm -rf dist/release-$(GOOS)-$(GOARCH)
	mkdir -p dist/release-$(GOOS)-$(GOARCH) dist
	@echo "==> 同步 registry 版本号到 $(VERSION)"
	@sed -i 's/"version": "[^"]*"/"version": "$(VERSION)"/' registry.json registry.zh-CN.json registry.en.json
	CC="$(CC)" CGO_ENABLED=1 GOOS=$(GOOS) GOARCH=$(GOARCH) go build -trimpath -buildmode=c-shared -ldflags="-s -w $(VERSION_LDFLAGS)" -o dist/release-$(GOOS)-$(GOARCH)/$(PLUGIN_NAME).$(LIBEXT) $(PKG)
	cd dist/release-$(GOOS)-$(GOARCH) && zip -q ../$(PLUGIN_NAME)_$(VERSION)_$(GOOS)_$(GOARCH).zip $(PLUGIN_NAME).$(LIBEXT)
	cd dist && sha256sum $(PLUGIN_NAME)_$(VERSION)_$(GOOS)_$(GOARCH).zip >> checksums.txt

# 兼容旧目标：Linux AMD64 发布
release-linux-amd64:
	$(MAKE) release GOOS=linux GOARCH=amd64 VERSION=$(VERSION)

# Windows AMD64 发布（需安装 mingw-w64：apt install gcc-mingw-w64-x86-64）
release-windows-amd64:
	$(MAKE) release GOOS=windows GOARCH=amd64 CC=x86_64-w64-mingw32-gcc VERSION=$(VERSION)

# macOS 发布（需在 macOS 主机上构建，使用系统 clang）
release-darwin-amd64:
	$(MAKE) release GOOS=darwin GOARCH=amd64 VERSION=$(VERSION)

release-darwin-arm64:
	$(MAKE) release GOOS=darwin GOARCH=arm64 VERSION=$(VERSION)
