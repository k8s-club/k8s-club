package informer

import (
	"fmt"
	"testing"

	v1 "k8s.io/api/core/v1"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/tools/cache"

	"K8s_demo/demo/examples/client"
)

/*
 * 测试 informer 不同维度的 hasSynced 功能
 * 功能解释详见：Informer机制 - 概述 (https://github.com/k8s-club/k8s-club/blob/main/articles/Informer%E6%9C%BA%E5%88%B6%20-%20%E6%A6%82%E8%BF%B0.md)
 */

// 测试 SharedInformer 维度的 hasSynced
func TestInformerHasSynced(t *testing.T) {
	factory, podInformer, _, err := InitInformer()
	if err != nil {
		return
	}

	stopC := make(chan struct{})
	defer close(stopC)

	fmt.Println("------start informer------------")
	factory.Start(stopC)

	// 等待 informer 完成 hasSynced
	// 注意：这里第三个参数中传入的是 podInformer.HasSynced
	if !cache.WaitForNamedCacheSync("InformerHasSynced", stopC, podInformer.HasSynced) {
		return
	}
	<-stopC
}

// 测试 processorListener 维度的 hasSynced
func TestListenerHasSynced(t *testing.T) {
	factory, _, registration, err := InitInformer()
	if err != nil {
		return
	}

	stopC := make(chan struct{})
	defer close(stopC)

	fmt.Println("------start informer------------")
	factory.Start(stopC)

	// 等待 listener 完成 hasSynced
	// 注意：这里第三个参数中传入的是 registration.HasSynced
	if !cache.WaitForNamedCacheSync("ListenerHasSynced", stopC, registration.HasSynced) {
		return
	}
	<-stopC
}

func InitInformer() (informers.SharedInformerFactory, cache.SharedIndexInformer, cache.ResourceEventHandlerRegistration, error) {
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
		return nil, nil, nil, err
	}
	return factory, podInformer, registration, nil
}
