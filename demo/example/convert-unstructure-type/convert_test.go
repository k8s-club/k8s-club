package convert_unstructure_type

import (
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"reflect"
	"testing"
)

func TestConvertType(t *testing.T) {

	unstructuredConfigMap := unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "v1",
			"kind":       "ConfigMap",
			"metadata": map[string]interface{}{
				"creationTimestamp": nil,
				"namespace":         "default",
				"name":              "my-configmap",
			},
			"data": map[string]interface{}{
				"foo": "bar",
			},
		},
	}

	// Unstructured -> Typed
	var typeConfigMap corev1.ConfigMap
	err := runtime.DefaultUnstructuredConverter.FromUnstructured(unstructuredConfigMap.Object, &typeConfigMap)
	if err != nil {
		panic(err.Error())
	}
	if typeConfigMap.GetName() != "my-configmap" {
		panic("Typed config map has unexpected data")
	}

	// Typed -> Unstructured
	object, err := runtime.DefaultUnstructuredConverter.ToUnstructured(&typeConfigMap)
	if err != nil {
		panic(err.Error())
	}
	if !reflect.DeepEqual(unstructured.Unstructured{Object: object}, unstructuredConfigMap ) {
		panic("Unstructured config map has unexpected data")
	}
}