//go:build cli

package main

import (
	"log"
	"os"

	"aiproxy/internal/cli"
)

// main CLI 版入口。
// 编译方式：go build -tags cli
func main() {
	log.SetFlags(log.LstdFlags | log.Lshortfile)
	os.Exit(cli.Run(os.Args[1:]))
}
