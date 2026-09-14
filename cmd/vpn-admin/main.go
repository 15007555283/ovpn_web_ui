package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"ovpn-web-ui/internal/app"
	"ovpn-web-ui/internal/version"
)

func main() {
	if len(os.Args) > 1 && os.Args[1] == "update" {
		if e := app.Update(os.Args[2:]); e != nil {
			fail(e)
		}
		return
	}
	if len(os.Args) == 2 && (os.Args[1] == "version" || os.Args[1] == "--version") {
		fmt.Println(version.Value)
		return
	}
	if len(os.Args) > 1 && os.Args[1] == "helper" {
		if len(os.Args) != 2 {
			fail(fmt.Errorf("helper 不接受命令行参数"))
		}
		if e := app.HelperMain(); e != nil {
			fmt.Fprintln(os.Stderr, "受限管理操作失败")
			os.Exit(1)
		}
		return
	}
	path := flag.String("config", "config.json", "配置文件")
	if filepath.Base(os.Args[0]) == "ovpn-cli" {
		fmt.Println("用法：ovpn-cli version | ovpn-cli update --check | sudo ovpn-cli update [--version vX.Y.Z]")
		return
	}
	flag.Parse()
	c, e := app.ReadConfig(*path)
	if e != nil {
		fail(e)
	}
	if e = app.Serve(c); e != nil {
		fail(e)
	}
}
func fail(e error) { fmt.Fprintln(os.Stderr, e); os.Exit(1) }
