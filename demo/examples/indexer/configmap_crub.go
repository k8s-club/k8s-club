package indexer

import (
	"context"
	"fmt"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

const (
	AnnotationsIndex = "annotations-test"
	LabelsIndex = "labels-test"
)

func createConfigMap(client kubernetes.Interface, name string, namespace string, annotationSet string, labelSet string) (*corev1.ConfigMap, error) {


	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
			Namespace: namespace,
			Labels: map[string]string{
				LabelsIndex: labelSet,
			},
			Annotations: map[string]string{
				AnnotationsIndex: annotationSet,
			},
		},
		Data: map[string]string{
			"foo": "boo",
		},
	}

	cm, err := client.CoreV1().ConfigMaps("default").Create(context.Background(), cm, metav1.CreateOptions{})

	if err != nil {
		fmt.Println("create err: ", err)
		return nil, err
	}

	fmt.Printf("create configmap: name %s, labels %s, annotations %s \n", cm.GetName(), cm.GetLabels(), cm.GetAnnotations())
	return cm, nil

}

func deleteConfigMap(client kubernetes.Interface , cm *corev1.ConfigMap) error {
	err := client.CoreV1().ConfigMaps(cm.Namespace).
		Delete(context.Background(), cm.GetName(),metav1.DeleteOptions{})

	if err != nil {
		fmt.Println("delete err: ", err)
		return err
	}

	fmt.Println("delete configmap", cm.GetName())

	return nil
}
