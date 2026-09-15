package main

import (
	"flag"
	"fmt"
	"os"
	"os/exec"

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
	if len(os.Args) == 2 {
		switch os.Args[1] {
		case "start", "stop", "restart", "status":
			cmd := exec.Command("/usr/bin/systemctl", os.Args[1], "vpn-admin.service")
			cmd.Stdout, cmd.Stderr, cmd.Stdin = os.Stdout, os.Stderr, os.Stdin
			if e := cmd.Run(); e != nil {
				fail(e)
			}
			return
		}
	}
	path := flag.String("config", app.DefaultConfigPath(), "配置文件（支持 --config）")
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
