package lease

import (
	"context"
	coordinationv1 "k8s.io/api/coordination/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	clientset "k8s.io/client-go/kubernetes"
	"k8s.io/component-helpers/apimachinery/lease"
	"k8s.io/klog/v2"
	"k8s.io/utils/clock"
	"os"
	"time"
)

// SetNodeOwnerFunc 设置 lease OwnerReferences
func SetNodeOwnerFunc(c clientset.Interface, nodeName string) func(lease *coordinationv1.Lease) error {
	return func(lease *coordinationv1.Lease) error {
		if len(lease.OwnerReferences) == 0 {
			if node, err := c.CoreV1().Nodes().Get(context.TODO(), nodeName, metav1.GetOptions{}); err == nil {
				lease.OwnerReferences = []metav1.OwnerReference{
					{
						APIVersion: corev1.SchemeGroupVersion.WithKind("Node").Version,
						Kind:       corev1.SchemeGroupVersion.WithKind("Node").Kind,
						Name:       nodeName,
						UID:        node.UID,
					},
				}
			} else {
				klog.ErrorS(err, "Failed to get node when trying to set owner ref to the node lease", "node", klog.KRef("", nodeName))
				return err
			}
		}
		return nil
	}
}

const (
	LeaseDurationSeconds = 40
	LeaseNameSpace       = "kube-node-lease"
)

// StartLeaseController 启动租约控制器
func StartLeaseController(clientset clientset.Interface, nodeName string) {
	myClock := clock.RealClock{}

	renewInterval := time.Duration(LeaseDurationSeconds * 0.25)
	heartbeatFailure := func() {
		klog.Infoln("lease controller heartbeat...")
		os.Exit(1)
	}
	klog.Infoln("starting lease controller")
	ctl := lease.NewController(myClock,
		clientset, nodeName, LeaseDurationSeconds,
		heartbeatFailure, renewInterval,
		nodeName, LeaseNameSpace,
		SetNodeOwnerFunc(clientset, nodeName))

	// 此方法会阻塞
	go ctl.Run(context.Background())
}
