package informer

import (
	"K8s_demo/demo/examples/client"
	"context"
	"fmt"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/rand"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/dynamic/dynamicinformer"
	"k8s.io/client-go/tools/cache"
	"log"
	"testing"
	"time"
)

var (
	namespace         = "default"
	label             = "informer-dynamic-simple-" + rand.String(5)
	configMapResource = schema.GroupVersionResource{
		Group:    "",
		Version:  "v1",
		Resource: "configmaps",
	}
)

// createConfigMap 使用unstructured.Unstructured创建configMap
func createConfigMap(client dynamic.Interface) *unstructured.Unstructured {
	// Unstructured对象
	cm := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "v1",
			"kind":       "ConfigMap",
			"metadata": map[string]interface{}{
				"namespace":    namespace,
				"generateName": "informer-dynamic-simple-",
				"labels": map[string]interface{}{
					"examples": label,
				},
			},
			"data": map[string]interface{}{
				"test": "test",
			},
		},
	}

	cm, err := client.Resource(configMapResource).Namespace(namespace).Create(context.Background(), cm, metav1.CreateOptions{})
	if err != nil {
		log.Fatal(err)
	}

	fmt.Printf("Created ConfigMap %s/%s\n", cm.GetNamespace(), cm.GetName())
	return cm
}

func deleteConfigMap(client dynamic.Interface, cm *unstructured.Unstructured) {
	err := client.Resource(configMapResource).Namespace(cm.GetNamespace()).Delete(context.Background(), cm.GetName(), metav1.DeleteOptions{})
	if err != nil {
		log.Fatal(err)
	}

	fmt.Printf("Deleted ConfigMap %s/%s\n", cm.GetNamespace(), cm.GetName())
}

func TestDynamicInformer(t *testing.T) {

	// dynamic客户端
	client := client.ClientSet.DynamicClient

	// 先创建一个config
	first := createConfigMap(client)

	factory := dynamicinformer.NewDynamicSharedInformerFactory(client, 5*time.Second)
	dynamicInformer := factory.ForResource(configMapResource)
	// eventHandler 回调
	dynamicInformer.Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: func(obj interface{}) {
			cm := obj.(*unstructured.Unstructured)
			fmt.Printf("Informer event: ConfigMap add %s/%s\n", cm.GetNamespace(), cm.GetName())
		},
		UpdateFunc: func(old, new interface{}) {
			cm := old.(*unstructured.Unstructured)
			fmt.Printf("Informer event: ConfigMap update %s/%s\n", cm.GetNamespace(), cm.GetName())
		},
		DeleteFunc: func(obj interface{}) {
			cm := obj.(*unstructured.Unstructured)
			fmt.Printf("Informer event: ConfigMap delete %s/%s\n", cm.GetNamespace(), cm.GetName())
		},
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	fmt.Println("------开始使用informer监听------------")
	factory.Start(ctx.Done())

	for gvr, ok := range factory.WaitForCacheSync(ctx.Done()) {
		if !ok {
			log.Fatal(fmt.Sprintf("Failed to sync cache for resource %v", gvr))
		}
	}

	// label查找的方法
	selector, err := labels.Parse("examples==" + label)
	if err != nil {
		log.Fatal(err)
	}
	list, err := dynamicInformer.Lister().List(selector) // 使用informer list，不从api server
	if err != nil {
		log.Fatal(err)
	}
	if len(list) != 1 {
		log.Println("expected ConfigMap not found")
	}

	// 创建cm
	second := createConfigMap(client)

	// Delete config maps created by this test.
	deleteConfigMap(client, first)
	deleteConfigMap(client, second)

	// 阻塞，可以发现 定时会有update事件，这是同步更新的状态
	select {}
}
