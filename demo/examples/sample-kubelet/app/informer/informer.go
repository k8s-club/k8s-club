package informer

import (
	"fmt"
	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/cache"
	"k8s.io/klog/v2"
	"time"
)

func InitInformer(clientset kubernetes.Interface, nodeName string) {
	// 创建InformerFactory
	// 使用 field 过滤，只过滤出"调度"到本节点的 pod
	informerFactory := informers.NewSharedInformerFactoryWithOptions(clientset, time.Minute*5, informers.WithTweakListOptions(func(options *v1.ListOptions) {
		options.FieldSelector = fmt.Sprintf("spec.nodeName=%s", nodeName)
	}))

	podInformer := informerFactory.Core().V1().Pods().Informer()
	// 注册事件处理函数到Pod的Informer
	podInformer.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: func(obj interface{}) {
			pod := obj.(*corev1.Pod)
			klog.Infof("Pod Added: %s\n", pod.Name)
			// TODO: 在这里可以处理Pod新增事件
			err := setPodStatus(clientset, pod, corev1.PodRunning)
			if err != nil {
				klog.Errorf("set Pod status running error: %s\n", err)
			}
		},
		UpdateFunc: func(oldObj, newObj interface{}) {
			//oldPod := oldObj.(*corev1.Pod)
			newPod := newObj.(*corev1.Pod)

			// TODO: 在这里可以处理Pod更新事件
			klog.Infof("Pod Updated: %s\n", newPod.Name)
			if newPod.Status.Phase == corev1.PodRunning {
				// 创建一个时间间隔为一分钟定时器，并更新pod状态
				// 只是为了"模拟"，pod的"状态流转"
				ticker := time.NewTicker(time.Minute)
				select {
				case <-ticker.C:
					err := setPodStatus(clientset, newPod, corev1.PodSucceeded)
					if err != nil {
						klog.Errorf("set Pod status running error: %s\n", err)
						break
					}
					klog.Info("Pod status change to Succeeded")
				}
			}
		},

		DeleteFunc: func(obj interface{}) {
			pod := obj.(*corev1.Pod)
			// TODO: 在这里可以处理Pod删除事件
			klog.Infof("Pod Deleted: %s\n", pod.Name)
			realDeletePod(clientset, pod)
		},
	})

	// 启动InformerFactory
	informerFactory.Start(wait.NeverStop)
	for r, ok := range informerFactory.WaitForCacheSync(wait.NeverStop) {
		if !ok {
			klog.Fatal(fmt.Sprintf("Failed to sync cache for resource %v", r))
		}
	}
	klog.Infoln("start kubelet informer...")
}
