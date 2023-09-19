package informer

import (
	"context"
	"fmt"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

// setPodStatus 修改pod状态
func setPodStatus(client kubernetes.Interface, pod *corev1.Pod, phase corev1.PodPhase) error {
	pod.Status.Phase = phase
	_, err := client.CoreV1().Pods(pod.Namespace).UpdateStatus(context.Background(), pod, metav1.UpdateOptions{})
	if err != nil {
		return err
	}
	return nil
}

// realDeletePod 删除pod操作
func realDeletePod(client kubernetes.Interface, pod *corev1.Pod) {
	var ps int64 = 0
	err := client.CoreV1().Pods(pod.Namespace).Delete(context.Background(), pod.Name, metav1.DeleteOptions{
		// 设置优雅时间为
		GracePeriodSeconds: &ps,
	})
	if err == nil {
		fmt.Println("pod真的删掉了")
	}
}
