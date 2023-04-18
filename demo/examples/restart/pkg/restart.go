package pkg

import (
	"K8s_demo/demo/examples/client"
	"context"
	"fmt"
	jsonpatch "github.com/evanphx/json-patch"
	appv1 "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/json"
	"k8s.io/client-go/kubernetes"
	"k8s.io/klog/v2"
	"log"

)



// GetPodsByDeployment 根据传入Deployment获取当前"正在"使用的pod
func GetPodsByDeployment(depName, ns string) []v1.Pod {

	clientSet := client.ClientSet.Client
	deployment, err := clientSet.AppsV1().Deployments(ns).Get(context.TODO(),
		depName, metav1.GetOptions{})
	if err != nil {
		klog.Error("create clientSet error: ", err)
		return nil
	}
	rsIdList := getRsIdsByDeployment(deployment, clientSet)
	podsList := make([]v1.Pod, 0)
	for _, rs := range rsIdList {
		pods := getPodsByReplicaSet(rs, clientSet, ns)
		podsList = append(podsList, pods...)
	}

	return podsList
}

// getPodsByReplicaSet 根据传入的ReplicaSet查询到需要的pod
func getPodsByReplicaSet(rs appv1.ReplicaSet, clientSet kubernetes.Interface, ns string) []v1.Pod {
	pods, err := clientSet.CoreV1().Pods(ns).List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		klog.Error("list pod error: ", err)
		return nil
	}

	ret := make([]v1.Pod, 0)
	for _, p := range pods.Items {
		// 找到 pod OwnerReferences uid相同的
		if p.OwnerReferences != nil && len(p.OwnerReferences) == 1 {
			if p.OwnerReferences[0].UID == rs.UID {
				ret = append(ret, p)
			}
		}
	}
	return ret

}

// getRsIdsByDeployment 根据传入的dep，获取到相关连的rs列表(滚更后的ReplicaSet就没用了)
func getRsIdsByDeployment(dep *appv1.Deployment, clientSet kubernetes.Interface) []appv1.ReplicaSet {
	// 需要使用match labels过滤
	rsList, err := clientSet.AppsV1().ReplicaSets(dep.Namespace).
		List(context.TODO(), metav1.ListOptions{
			LabelSelector: labels.Set(dep.Spec.Selector.MatchLabels).String(),
		})
	if err != nil {
		klog.Error("list ReplicaSets error: ", err)
		return nil
	}

	ret := make([]appv1.ReplicaSet, 0)
	for _, rs := range rsList.Items {
		ret = append(ret, rs)
	}
	return ret
}


// UpgradePodImage 原地升级pod镜像
func UpgradePodByImage(pod *v1.Pod, image string) {

	clientSet := client.ClientSet.Client
	patch := fmt.Sprintf(`[{"op": "replace", "path": "/spec/containers/0/image", "value": "%s"}]`, image)
	patchBytes := []byte(patch)

	jsonPatch, err := jsonpatch.DecodePatch(patchBytes)
	if err != nil {
		klog.Error("DecodePatch error: ", err)
		return
	}
	jsonPatchBytes, err := json.Marshal(jsonPatch)
	if err != nil {
		klog.Error("json Marshal error: ", err)
		return
	}
	_, err = clientSet.CoreV1().Pods(pod.Namespace).
		Patch(context.TODO(), pod.Name, types.JSONPatchType,
			jsonPatchBytes, metav1.PatchOptions{})
	if err != nil {
		log.Fatalln(err)
	}
}
