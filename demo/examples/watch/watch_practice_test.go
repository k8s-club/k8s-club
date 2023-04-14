package watch

import (
	"K8s_demo/demo/examples/client"
	"context"
	"fmt"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"testing"
	"time"
)

func TestWatchResource(t *testing.T) {
	client := client.ClientSet.Client

	configmap1 := createConfigMap(client)

	// 调用api server
	watch, err := client.CoreV1().ConfigMaps(configmap1.Namespace).
		Watch(context.Background(), metav1.ListOptions{
			LabelSelector: "app==configmapTest",
		})
	if err != nil {
		fmt.Println(err)
		return
	}

	go func() {
		// 如果没有，就会阻塞。
		for event := range watch.ResultChan() {
			fmt.Printf("watch Event: %s, kind: %s", event.Type, event.Object.GetObjectKind().GroupVersionKind().Kind)
		}
	}()

	configmap2 := createConfigMap(client)

	deleteConfigMap(client, configmap1)
	deleteConfigMap(client, configmap2)

	time.Sleep(2 * time.Second)
	watch.Stop()
	time.Sleep(1 * time.Second)

}

func createConfigMap(client kubernetes.Interface) *corev1.ConfigMap {

	cm := &corev1.ConfigMap{
		Data: map[string]string{
			"aaa": "bbb",
		},
	}
	cm.GenerateName = "watch-typed-simple-"
	cm.Namespace = "default"
	labels := map[string]string{
		"app": "configmapTest",
	}
	cm.SetLabels(labels)
	cm, err := client.CoreV1().ConfigMaps(cm.Namespace).Create(context.Background(), cm, metav1.CreateOptions{})
	if err != nil {
		fmt.Println("create configmap err: ", err)
		return cm
	}
	fmt.Printf("create configmap: name %s, labels %s \n", cm.GetName(), cm.GetLabels())

	return cm
}

func deleteConfigMap(client kubernetes.Interface, cm *corev1.ConfigMap) {

	err := client.CoreV1().ConfigMaps(cm.Namespace).Delete(context.Background(), cm.GetName(), metav1.DeleteOptions{})
	if err != nil {
		fmt.Println("delete err: ", err)
	}
	fmt.Println("delelte configmap", cm.GetName())

}
