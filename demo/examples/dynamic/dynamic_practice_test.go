package dynamic

import (
	"K8s_demo/demo/examples/init-client"
	"context"
	"fmt"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"log"
	"reflect"
	"testing"
)

/*
	使用动态客户端 crud资源。
*/

func TestDynamicClient(t *testing.T) {

	client := init_client.ClientSet.DynamicClient

	namespace := "default"
	res := schema.GroupVersionResource{
		Group:    "",
		Version:  "v1",
		Resource: "configmaps",
	}

	unstructuredObj := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "v1",
			"kind":       "ConfigMap",
			"metadata": map[string]interface{}{
				"namespace":    namespace,
				"generateName": "crud-dynamic-simple-",
			},
			"data": map[string]interface{}{
				"foo": "bar",
			},
		},
	}

	obj, err := client.Resource(res).Namespace("default").
		Create(context.Background(), unstructuredObj, metav1.CreateOptions{})
	if err != nil {
		fmt.Println("create error:", err)
	}

	fmt.Printf("create configmap:%s\n", obj.GetName())

	data, _, _ := unstructured.NestedStringMap(obj.Object, "data")
	if !reflect.DeepEqual(map[string]string{"foo": "bar"}, data) {
		log.Fatal("Created ConfigMap has unexpected data")
	}

	getObj, err := client.Resource(res).Namespace(namespace).
		Get(context.Background(), obj.GetName(), metav1.GetOptions{})
	if err != nil {
		fmt.Println("get error:")
	}

	fmt.Printf("get configmap:%s\n", getObj.GetName())

	err = unstructured.SetNestedField(getObj.Object, "qux", "data", "foo")
	if err != nil {
		fmt.Println("operator error:", err)
	}
	updateObj, err := client.Resource(res).Namespace(namespace).
		Update(context.Background(), getObj, metav1.UpdateOptions{})
	if err != nil {
		fmt.Println("update error:", err)
	}

	fmt.Printf("update ConfigMap %s\n", updateObj.GetName())

	// Delete
	err = client.Resource(res).Namespace(namespace).
		Delete(context.Background(), updateObj.GetName(), metav1.DeleteOptions{})
	if err != nil {
		fmt.Println("delete error:", err)
	}
	fmt.Printf("Deleted ConfigMap %s\n", updateObj.GetName())

}
