// Package version 保存插件版本号。
// 构建时经 ldflags -X 注入（c-shared 构建模式下 -X 对 main 包变量不生效，
// 因此版本号放在子包中，由 main 包引用）。
package version

// Version 为插件版本号，默认 dev；构建时由 -X 覆盖，见 Makefile / CI。
var Version = "dev"
