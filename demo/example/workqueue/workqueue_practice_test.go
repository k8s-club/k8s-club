package workqueue

import (
	"K8s_demo/demo/example/initClient"
	"context"
	"fmt"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/dynamic/dynamicinformer"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/util/workqueue"
	"log"
	"testing"
	"time"
)

var (
	namespace         = "default"
	ConfigMapResource = schema.GroupVersionResource{
		Group:    "",
		Version:  "v1",
		Resource: "configmaps",
	}
)

func TestWorkQueuePractice(t *testing.T) {
	// 1. 动态客户端
	client := initClient.ClientSet.DynamicClient

	// 2. 建立workqueue
	queue := workqueue.NewRateLimitingQueue(workqueue.DefaultControllerRateLimiter())
	defer queue.ShutDown()

	// dynamic informer
	factory := dynamicinformer.NewFilteredDynamicSharedInformerFactory(
		client, 5*time.Second, namespace, func(*metav1.ListOptions) {},
	)
	// 动态客户端资源读取
	dynamicInformer := factory.ForResource(ConfigMapResource)
	// 加入回调函数
	dynamicInformer.Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: func(obj interface{}) {
			// key is a string <namespace>/<name>
			key, err := cache.MetaNamespaceKeyFunc(obj)
			if err == nil {
				fmt.Printf("New event: ADD %s\n", key)
				queue.Add(key) // 把key加入queue
			}
		},

		UpdateFunc: func(old, new interface{}) {
			key, err := cache.MetaNamespaceKeyFunc(new)
			if err == nil {
				fmt.Printf("New event: UPDATE %s\n", key)
				queue.Add(key)
			}
		},

		DeleteFunc: func(obj interface{}) {
			// 删除的时候需要注意！！
			// much like cache.MetaNamespaceKeyFunc + some extra check.
			key, err := cache.DeletionHandlingMetaNamespaceKeyFunc(obj)
			if err == nil {
				fmt.Printf("New event: DELETE %s\n", key)
				queue.Add(key)
			}
		},
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// informer 启动
	factory.Start(ctx.Done())


	// 当启动informer时，需要先等待资源同步
	for gvr, ok := range factory.WaitForCacheSync(ctx.Done()) {
		if !ok {
			log.Fatal(fmt.Sprintf("Failed to sync cache for resource %v", gvr))
		}
	}

	// 启动数个worker
	for i := 0; i < 3; i++ {
		// A better way is to use wait.Until() from "k8s.io/apimachinery/pkg/util/wait"
		// for every worker.
		fmt.Printf("Starting worker %d\n", i)

		// worker()
		go func(n int) {
			for {
				// Someone said we're done?
				select {
				case <-ctx.Done():
					fmt.Printf("Controller's done! Worker %d exiting...\n", n)
					return
				default:
				}

				// 从worker中取出资源
				key, quit := queue.Get()
				if quit {
					fmt.Printf("Work queue has been shut down! Worker %d exiting...\n", n)
					return
				}
				fmt.Printf("Worker %d is about to start process new item %s.\n", n, key)

				// processSingleItem() - scoped to utilize defer and premature returns.
				func() {
					// Tell the queue that we are done with processing this key.
					// This unblocks the key for other workers and allows safe parallel
					// processing because two objects with the same key are never processed
					// in parallel.
					defer queue.Done(key)

					// YOUR CONTROLLER'S BUSINESS LOGIC GOES HERE
					obj, err := dynamicInformer.Lister().Get(key.(string))
					if err == nil {
						fmt.Printf("Worker %d found ConfigMap object in informer's cahce %#v.\n", n, obj)
						// RECONCILE THE OBJECT - PUT YOUR BUSINESS LOGIC HERE.
						if n == 1 {
							err = fmt.Errorf("worker %d is a chronic failure", n)
						}
					} else {
						fmt.Printf("Worker %d got error %v while looking up ConfigMap object in informer's cache.\n", n, err)
					}

					// Handle the error if something went wrong during the execution of
					// the business logic.

					if err == nil {
						// The key has been handled successfully - forget about it. In particular, it
						// ensures that future processing of updates for this key won't be rate limited
						// because of errors on previous attempts.
						fmt.Printf("Worker %d reconciled ConfigMap %s successfully. Removing it from te queue.\n", n, key)
						queue.Forget(key)
						return
					}

					// 重试次数如果超过一定次数，直接抛弃不要
					if queue.NumRequeues(key) >= 5 {
						fmt.Printf("Worker %d gave up on processing %s. Removing it from the queue.\n", n, key)
						queue.Forget(key)
						return
					}


					fmt.Printf("Worker %d failed to process %s. Putting it back to the queue to retry later.\n", n, key)
					// 重新入队列
					queue.AddRateLimited(key)
				}()
			}
		}(i)
	}

	// Create some Kubernetes objects to make the above program actually process something.
	cm1 := createConfigMap(client)
	cm2 := createConfigMap(client)
	cm3 := createConfigMap(client)
	cm4 := createConfigMap(client)
	cm5 := createConfigMap(client)

	// Delete config maps created by this test.
	deleteConfigMap(client, cm1)
	deleteConfigMap(client, cm2)
	deleteConfigMap(client, cm3)
	deleteConfigMap(client, cm4)
	deleteConfigMap(client, cm5)

	// Stay for a couple more seconds to let the program finish.
	time.Sleep(10 * time.Second)
	queue.ShutDown() // 关闭队列
	cancel()
	time.Sleep(1 * time.Second)





}




func createConfigMap(client dynamic.Interface) *unstructured.Unstructured {

	cm := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "v1",
			"kind":       "ConfigMap",
			"metadata": map[string]interface{}{
				"namespace":    namespace,
				"generateName": "workqueue-",
			},
			"data": map[string]interface{}{
				"foo": "bar",
			},
		},
	}


	cm, err := client.
		Resource(ConfigMapResource).
		Namespace(namespace).
		Create(context.Background(), cm, metav1.CreateOptions{})

	if err != nil {
		fmt.Println("create err: ",err)
		return cm
	}
	fmt.Printf("create configmap: name %s, labels %s \n", cm.GetName(), cm.GetLabels())


	return cm


}

func deleteConfigMap(client dynamic.Interface, cm *unstructured.Unstructured) {

	err := client.
		Resource(ConfigMapResource).
		Namespace(cm.GetNamespace()).
		Delete(context.Background(), cm.GetName(), metav1.DeleteOptions{})

	if err != nil {
		fmt.Println("delete err: ", err)
	}

	fmt.Println("delete configmap", cm.GetName())


}
