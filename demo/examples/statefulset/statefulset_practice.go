package statefulset

import (
	"context"
	"errors"
	"fmt"
	appsv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

func CreateStatefulSet(kubeClient kubernetes.Interface, sfs *appsv1.StatefulSet, namespace string) (*appsv1.StatefulSet, error) {
	statefulSets, err := GetStatefulSet(kubeClient, namespace, sfs.Name)
	if statefulSets != nil {
		fmt.Printf("statefulSets %s is created, exit \n", sfs.Name)
		return statefulSets, nil
	}
	if err != nil {
		fmt.Println("get statefulSet error")
		return statefulSets, nil
	}

	sfs, err = kubeClient.AppsV1().StatefulSets(namespace).Create(context.Background(), sfs, metav1.CreateOptions{})
	if err != nil {
		return nil, errors.New("statefulSets create error")
	}

	return sfs, nil
}

func DeleteStatefulSet(kubeClient kubernetes.Interface, namespace, id string) error {
	sfs, err := GetStatefulSet(kubeClient, namespace, id)
	if sfs == nil {
		errors.New("statefulSet does not exist")
	}

	deletePolicy := metav1.DeletePropagationForeground
	err = kubeClient.AppsV1().StatefulSets(namespace).Delete(context.Background(), id, metav1.DeleteOptions{
		PropagationPolicy: &deletePolicy,
	})

	return err
}

func GetStatefulSet(kubeClient kubernetes.Interface, namespace string, statefulSetName string) (*appsv1.StatefulSet, error) {
	statefulSet, err := kubeClient.AppsV1().StatefulSets(namespace).Get(context.Background(),
		statefulSetName, metav1.GetOptions{})
	if err != nil {
		fmt.Println("get statefulSet error")
		return nil, err
	}

	return statefulSet, nil
}

func ListStatefulSets(kubeClient kubernetes.Interface, namespace string) ([]*appsv1.StatefulSet, error) {
	var objList []*appsv1.StatefulSet
	result, err := kubeClient.AppsV1().StatefulSets(namespace).List(context.Background(), metav1.ListOptions{
		LabelSelector: "node",
	})
	if err != nil {
		return nil, errors.New("get statefulSets list error")
	}
	for _, statefulSet := range result.Items {
		objList = append(objList, &statefulSet)
	}

	return objList, nil
}
