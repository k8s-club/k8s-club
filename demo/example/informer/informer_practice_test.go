package informer

import (
	"K8s_demo/demo/example/initClient"
	"fmt"
	v1 "k8s.io/api/core/v1"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/tools/cache"
	"testing"
)

func InformerPractice() {
	client := initClient.ClientSet.Client

	factory := informers.NewSharedInformerFactoryWithOptions(client, 0, informers.WithNamespace("default"))
	// 不同informer可直接在 factory中同时监听。注意：需要创建eventHandler。
	podInformer := factory.Core().V1().Pods().Informer()
	//jobInformer := factory.Batch().V1().Jobs().Informer()
	//serviceInformer := factory.Core().V1().Services().Informer()
	//deploymentInformer := factory.Apps().V1().Deployments().Informer()
	//statefulSetInformer := factory.Apps().V1().StatefulSets().Informer()

	podInformer.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: func(obj interface{}) {
			pod := obj.(*v1.Pod)
			fmt.Println("add new pod:", pod.GetName())
		},
		UpdateFunc: func(oldObj, newObj interface{}) {
			oldPod := oldObj.(*v1.Pod)
			newPod := newObj.(*v1.Pod)
			fmt.Printf("update old pod name:%s to new pod name:%s \n", oldPod.GetName(), newPod.GetName())
		},
		DeleteFunc: func(obj interface{}) {
			pod := obj.(*v1.Pod)
			fmt.Println("delete pod:", pod.GetName())
		},
	})

	stopC := make(chan struct{})
	defer close(stopC)

	fmt.Println("------开始使用informer监听------------")
	factory.Start(stopC)
	<-stopC
}

func TestInformer(t *testing.T) {
	InformerPractice()
}
