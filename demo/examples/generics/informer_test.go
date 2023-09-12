package generics

import (
	initclient "K8s_demo/demo/examples/client"
	"context"
	"fmt"
	appv1 "k8s.io/api/apps/v1"
	v12 "k8s.io/api/coordination/v1"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/dynamic/dynamicinformer"
	"log"
	"testing"
	"time"
)

type ResourceEventHandler[T runtime.Object] struct {
	AddFunc    func(obj T)
	UpdateFunc func(oldObj T, newObj T)
	DeleteFunc func(obj T)
}

func (e *ResourceEventHandler[T]) OnAdd(obj interface{}) {
	if o, ok := obj.(*unstructured.Unstructured); ok {
		rr, _ := convertUnstructuredToResource[T](o)
		e.AddFunc(rr)
	}
}

func (e *ResourceEventHandler[T]) OnUpdate(oldObj, newObj interface{}) {
	var t, tt *unstructured.Unstructured
	var ok bool
	if t, ok = oldObj.(*unstructured.Unstructured); !ok {
		return
	}
	if tt, ok = newObj.(*unstructured.Unstructured); !ok {
		return
	}
	oldT, err := convertUnstructuredToResource[T](t)
	if err != nil {
		return
	}
	newT, err := convertUnstructuredToResource[T](tt)
	if err != nil {
		return
	}
	e.UpdateFunc(oldT, newT)

}

func (e *ResourceEventHandler[T]) OnDelete(obj interface{}) {
	if o, ok := obj.(*unstructured.Unstructured); ok {
		rr, _ := convertUnstructuredToResource[T](o)
		e.DeleteFunc(rr)
	}
}

func TestDynamicInformer(t *testing.T) {
	// dynamic客户端
	client := initclient.ClientSet.DynamicClient
	factory := dynamicinformer.NewDynamicSharedInformerFactory(client, 5*time.Second)

	// deployment对象
	deployDynamicInformer := factory.ForResource(parseGVR("apps/v1/deployments"))
	// eventHandler 回调
	deployHandler := &ResourceEventHandler[*appv1.Deployment]{
		AddFunc: func(deploy *appv1.Deployment) {
			fmt.Println("on add deploy:", deploy.Name)
		},
		UpdateFunc: func(old *appv1.Deployment, new *appv1.Deployment) {
			fmt.Println("on update deploy:", new.Name)
		},
		DeleteFunc: func(dep *appv1.Deployment) {
			fmt.Println("on delete deploy:", dep.Name)
		},
	}

	deployDynamicInformer.Informer().AddEventHandler(deployHandler)

	// pod对象
	podDynamicInformer := factory.ForResource(parseGVR("core/v1/pods"))
	podHandler := &ResourceEventHandler[*v1.Pod]{
		AddFunc: func(pod *v1.Pod) {
			fmt.Println("on add pod:", pod.Name)
		},
		UpdateFunc: func(old *v1.Pod, new *v1.Pod) {
			fmt.Println("on update pod:", new.Name)
		},
		DeleteFunc: func(pod *v1.Pod) {
			fmt.Println("on delete pod:", pod.Name)
		},
	}
	podDynamicInformer.Informer().AddEventHandler(podHandler)

	// lease对象
	leaseDynamicInformer := factory.ForResource(parseGVR("coordination.k8s.io/v1/leases"))
	leaseHandler := &ResourceEventHandler[*v12.Lease]{
		AddFunc: func(pod *v12.Lease) {
			fmt.Println("on add lease:", pod.Name)
		},
		UpdateFunc: func(old *v12.Lease, new *v12.Lease) {
			fmt.Println("on update lease:", new.Name)
		},
		DeleteFunc: func(pod *v12.Lease) {
			fmt.Println("on delete lease:", pod.Name)
		},
	}
	leaseDynamicInformer.Informer().AddEventHandler(leaseHandler)

	ctx, cancel := context.WithTimeout(context.Background(), time.Second*10)
	defer cancel()

	fmt.Println("------开始使用informer监听------------")
	// 启动informer
	factory.Start(ctx.Done())

	for gvr, ok := range factory.WaitForCacheSync(ctx.Done()) {
		if !ok {
			log.Fatal(fmt.Sprintf("Failed to sync cache for resource %v", gvr))
		}
	}

	select {
	case <-ctx.Done():
		return
	}

}
