package client

import (
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/klog/v2"
)

func InitClient(path string) kubernetes.Interface {

	// 兼容 inCluster 模式, outCluster 模式
	var config *rest.Config
	config, err := rest.InClusterConfig()
	if err != nil {
		// 使用 KubeConfig 文件创建集群配置 Config 对象
		if config, err = clientcmd.BuildConfigFromFlags("", path); err != nil {
			klog.Fatal(err)
		}
	}

	// 创建Kubernetes客户端
	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		klog.Fatal(err)
	}
	return clientset
}
