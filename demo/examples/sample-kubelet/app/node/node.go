package node

import (
	"context"
	"fmt"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/json"
	"k8s.io/client-go/kubernetes"
	"k8s.io/klog/v2"
)

// ReportNodeStatus 上报节点状态
func ReportNodeStatus(clientset kubernetes.Interface, node *corev1.Node) error {
	// 构建节点状态的部分更新
	nodePatch := &corev1.Node{
		ObjectMeta: v1.ObjectMeta{
			Name: node.Name,
		},
		Status: corev1.NodeStatus{
			Capacity: corev1.ResourceList{
				corev1.ResourceCPU:              resource.MustParse("4"),
				corev1.ResourceMemory:           resource.MustParse("8Gi"),
				corev1.ResourceEphemeralStorage: resource.MustParse("100Gi"),
			},
			Allocatable: corev1.ResourceList{
				corev1.ResourceCPU:              resource.MustParse("4"),
				corev1.ResourceMemory:           resource.MustParse("8Gi"),
				corev1.ResourceEphemeralStorage: resource.MustParse("100Gi"),
			},
			Conditions: []corev1.NodeCondition{
				{
					Type:               corev1.NodeReady,
					Status:             corev1.ConditionTrue,
					LastTransitionTime: v1.Now(),
				},
			},
			Addresses: []corev1.NodeAddress{
				{
					Type:    corev1.NodeInternalIP,
					Address: "192.168.0.100",
				},
				{
					Type:    corev1.NodeHostName,
					Address: "node-1",
				},
			},
			NodeInfo: corev1.NodeSystemInfo{
				OperatingSystem:         "Linux",
				Architecture:            "x86_64",
				KernelVersion:           "4.19.0-10-amd64",
				ContainerRuntimeVersion: "docker://19.3.13",
			},
			Phase: corev1.NodeRunning,
			Images: []corev1.ContainerImage{
				{
					Names:     []string{"nginx:latest"},
					SizeBytes: 12345678,
				},
				{
					Names:     []string{"busybox:1.32"},
					SizeBytes: 98765432,
				},
			},
		},
	}

	patchBytes, err := json.Marshal(nodePatch)
	if err != nil {
		return fmt.Errorf("failed to marshal patch: %v", err)
	}

	// 执行节点状态的部分更新
	_, err = clientset.CoreV1().Nodes().Patch(context.Background(), node.Name, types.MergePatchType, patchBytes, v1.PatchOptions{})
	if err != nil {
		return fmt.Errorf("failed to patch node status: %v", err)
	}

	return nil
}

// RegisterNode 注册节点
func RegisterNode(clientset kubernetes.Interface, nodeName string) *corev1.Node {

	// 创建 Node 对象
	node := &corev1.Node{
		ObjectMeta: v1.ObjectMeta{
			Name: nodeName,
		},
		Spec: corev1.NodeSpec{
			// 填充 Node 的 Spec 字段
		},
		Status: corev1.NodeStatus{
			Capacity: corev1.ResourceList{
				corev1.ResourceCPU:              resource.MustParse("4"),
				corev1.ResourceMemory:           resource.MustParse("8Gi"),
				corev1.ResourceEphemeralStorage: resource.MustParse("100Gi"),
			},
			Allocatable: corev1.ResourceList{
				corev1.ResourceCPU:              resource.MustParse("4"),
				corev1.ResourceMemory:           resource.MustParse("8Gi"),
				corev1.ResourceEphemeralStorage: resource.MustParse("100Gi"),
			},
			Conditions: []corev1.NodeCondition{
				{
					Type:               corev1.NodeReady,
					Status:             corev1.ConditionTrue,
					LastTransitionTime: v1.Now(),
				},
			},
			Addresses: []corev1.NodeAddress{
				{
					Type:    corev1.NodeInternalIP,
					Address: "192.168.0.100",
				},
				{
					Type:    corev1.NodeHostName,
					Address: "node-1",
				},
			},
			NodeInfo: corev1.NodeSystemInfo{
				OperatingSystem:         "Linux",
				Architecture:            "x86_64",
				KernelVersion:           "4.19.0-10-amd64",
				ContainerRuntimeVersion: "docker://19.3.13",
			},
			Phase: corev1.NodeRunning,
			Images: []corev1.ContainerImage{
				{
					Names:     []string{"nginx:latest"},
					SizeBytes: 12345678,
				},
				{
					Names:     []string{"busybox:1.32"},
					SizeBytes: 98765432,
				},
			},
		},
	}

	// 创建Node
	createdNode, err := clientset.CoreV1().Nodes().Create(context.Background(), node, v1.CreateOptions{})
	if err != nil {
		klog.Fatalf("Failed to create Node: %v", err)
	}

	klog.Infof("Node created: %s\n", createdNode.Name)
	return createdNode
}
