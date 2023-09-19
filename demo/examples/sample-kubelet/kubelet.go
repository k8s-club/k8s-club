package main

import (
	"K8s_demo/demo/examples/sample-kubelet/app"
	"k8s.io/component-base/cli"
	"os"
)

func main() {
	command := app.NewKubeletCommand()
	code := cli.Run(command)
	os.Exit(code)
}
