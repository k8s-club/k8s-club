package indexer

import (
	"fmt"
	"log"
	"testing"

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/tools/cache"

	initclient "K8s_demo/demo/examples/client"
)

func TestConfigMapIndexInformer(t *testing.T) {

	client := initclient.ClientSet.Client

	cm1, err := createConfigMap(client, "configmap-test1", "default", "annotation-test", "label-test")
	if err != nil {
		fmt.Println(err)
	}

	cm2, err := createConfigMap(client, "configmap-test2", "default", "annotation-test2", "label-test2")
	if err != nil {
		fmt.Println(err)
	}

	listWatcher := cache.NewListWatchFromClient(client.CoreV1().RESTClient(), "configmaps", "default", fields.Everything()) // list

	// 建立index 本地缓存
	indexer := cache.Indexers{
		cache.NamespaceIndex: cache.MetaNamespaceIndexFunc, // 本来内置的index就是以namespace来当做index
		AnnotationsIndex:     MetaAnnotationsIndexFunc,     // 自定义index增加索引
		LabelsIndex:          MetaLabelsIndexFunc,          // 自定义index增加索引

	}

	// 建立indexInformer
	myIndexer, indexInformer := cache.NewIndexerInformer(listWatcher, &v1.ConfigMap{}, 0, &ConfigMapHandler{}, indexer)

	stopC := make(chan struct{})
	// 需要用goroutine拉资源
	go indexInformer.Run(wait.NeverStop)
	defer close(stopC)

	// 如果没有同步完毕
	if !cache.WaitForCacheSync(stopC, indexInformer.HasSynced) {
		log.Fatal("sync err")
	}

	// 这里可以练习调用indexer接口的各种方法：ex: GetIndexers() IndexKeys() ByIndex() 等等

	fmt.Println("ListKeys:", myIndexer.ListKeys()) // return: [default/kube-root-ca.crt] 切片 <namespace/资源名>

	fmt.Println(myIndexer.IndexKeys(cache.NamespaceIndex, "default"))
	fmt.Println(myIndexer.IndexKeys(AnnotationsIndex, "annotation-test"))
	fmt.Println(myIndexer.IndexKeys(AnnotationsIndex, "annotation-test2"))
	fmt.Println(myIndexer.IndexKeys(LabelsIndex, "label-test2"))
	fmt.Println(myIndexer.ByIndex(AnnotationsIndex, "annotation-test"))

	// 删除
	_ = deleteConfigMap(client, cm1)
	_ = deleteConfigMap(client, cm2)

}

// MetaAnnotationsIndexFunc 自定义indexFunc
func MetaAnnotationsIndexFunc(obj interface{}) ([]string, error) {
	meta, err := meta.Accessor(obj)
	if err != nil {
		return []string{""}, fmt.Errorf("object has no meta: %v", err)
	}

	if res, ok := meta.GetAnnotations()[AnnotationsIndex]; ok {
		return []string{res}, nil
	}

	return []string{}, nil
}

// MetaLabelIndexFunc 自定义indexFunc
func MetaLabelsIndexFunc(obj interface{}) ([]string, error) {
	meta, err := meta.Accessor(obj)
	if err != nil {
		return []string{""}, fmt.Errorf("object has no meta: %v", err)
	}

	if res, ok := meta.GetLabels()[LabelsIndex]; ok {
		return []string{res}, nil
	}

	return []string{}, nil
}

// 事件的回调函数
type ConfigMapHandler struct {
}

func (c *ConfigMapHandler) OnAdd(obj interface{}, isInInitialList bool) {
	fmt.Println("add:", obj.(*v1.ConfigMap).Name)
}

func (c *ConfigMapHandler) OnUpdate(oldObj, newObj interface{}) {

}

func (c *ConfigMapHandler) OnDelete(obj interface{}) {
	fmt.Println("delete:", obj.(*v1.ConfigMap).Name)
}
