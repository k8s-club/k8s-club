package main

import (
	"context"
	"flag"
	"fmt"
	"path/filepath"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/util/homedir"
)

func main() {
	var kubeconfig *string
	if home := homedir.HomeDir(); home != "" {
		kubeconfig = flag.String("kubeconfig", filepath.Join(home, ".kube", "config"), "(optional) absolute path to the kubeconfig file")
	} else {
		kubeconfig = flag.String("kubeconfig", "", "absolute path to the kubeconfig file")
	}
	flag.Parse()

	config, err := clientcmd.BuildConfigFromFlags("", *kubeconfig)
	if err != nil {
		panic(err)
	}
	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		panic(err)
	}

	patchData := fmt.Sprintf(`{
  "spec": {
    "template": {
      "spec": {
        "containers": [
          {
			"name": "nginx",
            "image": "nginx:1.8"
          }
        ],
		"affinity":null
      }
    }
  }
}`)
	_, err = clientset.AppsV1().Deployments("default").Patch(context.TODO(), "nginx-deployment", types.MergePatchType, []byte(patchData), metav1.PatchOptions{})

	if err != nil {
		println(err.Error())
	} else {
		fmt.Println("patch ok")
	}
}
