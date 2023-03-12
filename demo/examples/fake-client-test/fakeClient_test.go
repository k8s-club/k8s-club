package client

import (
	. "K8s_demo/demo/examples/init-client"
	"context"
	"fmt"
	v1 "k8s.io/api/core/v1"
	v12 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
	"testing"
)

func TestFakeClient(t *testing.T) {
	// 方法一：直接调用
	client := fake.NewSimpleClientset(
		NewPod("abc", "pod1"), // 在fake client插入数据
	)
	ctx := context.Background()
	pod, err := client.CoreV1().Pods("abc").Get(ctx, "pod1", v12.GetOptions{})
	fmt.Println(pod, err)

	// 方法二：组合成结构体方式。
	clientset := fake.NewSimpleClientset(
		NewPod("abc", "pod2"),
	)
	ClientSet.Client = clientset
	pod2, err := ClientSet.Client.CoreV1().Pods("abc").Get(ctx, "pod2", v12.GetOptions{})
	fmt.Println(pod2, err)

}

func TestPodPatch(t *testing.T) {
	pod := NewPod("abc", "p1")

	// 加入进去。
	//pod.Annotations = map[string]string{
	//	"try": "aaa",
	//}
	clientSet := fake.NewSimpleClientset(pod)
	_ = PatchPod(pod, clientSet)

}

func NewPod(namespace string, name string) *v1.Pod {
	pod := &v1.Pod{}
	pod.Name = name
	pod.Namespace = namespace
	return pod
}
