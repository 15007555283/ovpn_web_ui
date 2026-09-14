package version

// Value 由正式构建通过 -ldflags 注入；本地直接 go build 时保持 dev。
var Value = "dev"
