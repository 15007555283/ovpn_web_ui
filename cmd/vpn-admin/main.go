package main

import (
	"flag"
	"fmt"
	"os"
	"ovpn-web-ui/internal/app"
)

func main() {
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
