package informer

import (
	"fmt"
	"testing"

	v1 "k8s.io/api/core/v1"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/tools/cache"

	"K8s_demo/demo/examples/client"
)

func TestInformerHasSynced(t *testing.T) {
	InformerHasSynced()
}

func TestListenerHasSynced(t *testing.T) {
	ListenerHasSynced()
}

func InformerHasSynced() {
	kubeClient := client.ClientSet.Client
	factory := informers.NewSharedInformerFactoryWithOptions(kubeClient, 0, informers.WithNamespace("default"))
	podInformer := factory.Core().V1().Pods().Informer()

	_, err := podInformer.AddEventHandler(cache.ResourceEventHandlerFuncs{
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
	if err != nil {
		return
	}

	stopC := make(chan struct{})
	defer close(stopC)

	fmt.Println("------start informer------------")
	factory.Start(stopC)

	// 等待 informer 完成 hasSynced
	if !cache.WaitForNamedCacheSync("InformerHasSynced", stopC, podInformer.HasSynced) {
		return
	}
	<-stopC
}

func ListenerHasSynced() {
	kubeClient := client.ClientSet.Client
	factory := informers.NewSharedInformerFactoryWithOptions(kubeClient, 0, informers.WithNamespace("default"))
	podInformer := factory.Core().V1().Pods().Informer()

	registration, err := podInformer.AddEventHandler(cache.ResourceEventHandlerFuncs{
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
	if err != nil {
		return
	}

	stopC := make(chan struct{})
	defer close(stopC)

	fmt.Println("------start informer------------")
	factory.Start(stopC)

	// 等待 listener 完成 hasSynced
	if !cache.WaitForNamedCacheSync("ListenerHasSynced", stopC, registration.HasSynced) {
		return
	}
	<-stopC
}
