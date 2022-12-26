package fakeclienttest

import (
	"context"
	"fmt"
	v1 "k8s.io/api/core/v1"
	v12 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/json"
	"k8s.io/client-go/kubernetes"
)

type JsonPatch struct {
	Op    string            `json:"op"`
	Path  string            `json:"path"`
	Value map[string]string `json:"value,omitempty"`
}

type JsonPatchList []*JsonPatch

func AddJsonPatch(jps ...*JsonPatch) JsonPatchList {
	list := make([]*JsonPatch, len(jps))

	for i, jsonPatch := range jps {
		list[i] = jsonPatch
	}

	return list
}

func PatchPod(pod *v1.Pod, client kubernetes.Interface) error {
	// 需要确保annotations
	//list := AddJsonPatch(&JsonPatch{
	//	Op: "add",
	//	Path: "/metadata/annotations/version",
	//	Value: "1.0",
	//})
	list := AddJsonPatch(&JsonPatch{
		Op:   "add",
		Path: "/metadata/annotations",
		Value: map[string]string{
			"version": "1.0",
		},
	})

	b, _ := json.Marshal(list)
	pod, err := client.CoreV1().Pods(pod.Namespace).
		Patch(context.Background(), pod.Name, types.JSONPatchType, b, v12.PatchOptions{})
	if err != nil {
		fmt.Println(err)
		return err
	}
	fmt.Println(pod.Annotations)

	return nil
}
