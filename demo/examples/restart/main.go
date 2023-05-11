package main

import "K8s_demo/demo/examples/restart/pkg"

func main() {
	// 需要更新的副本数
	replicasNum := 2
	// dep name, namespace
	depName := "example-deployment"
	ns := "default"
	pods := pkg.GetPodsByDeployment(depName, ns)

	for i := 0; i < replicasNum; i++ {
		// pod原地升级
		pkg.UpgradePodByImage(&pods[i], "nginx:1.19-alpine")
	}

}
