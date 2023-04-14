package serializer

import (
	"fmt"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	jsonserializer "k8s.io/apimachinery/pkg/runtime/serializer/json"
	"k8s.io/apimachinery/pkg/util/json"
	"k8s.io/apimachinery/pkg/util/yaml"
	"k8s.io/client-go/kubernetes/scheme"
	"reflect"
	"testing"
)

var (
	obj = corev1.ConfigMap{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "v1",
			Kind:       "ConfigMap",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-my-configmap",
			Namespace: "default",
		},
		Data: map[string]string{
			"player_initial_lives": "3",
			"aaa":                  "bbbb",
			"vvv":                  "zzzz",
		},
	}

	// 用Unstructured 实现。
	uConfigMap = unstructured.Unstructured{
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
	// yaml 字符串
	yConfigMap = `---
apiVersion: v1
data:
  foo: bar
kind: ConfigMap
metadata:
  creationTimestamp:
  name: my-configmap
  namespace: default
`
)

func TestSerializeToJsonPractice(t *testing.T) {

	encoder := jsonserializer.NewSerializerWithOptions(
		// 解码(json -> runtime.Object)才需要传入参数，因为要找到k8s中对应的资源
		nil, // jsonserializer.MetaFactory
		nil, // runtime.ObjectCreater
		nil, // runtime.ObjectTyper
		jsonserializer.SerializerOptions{
			Yaml:   false,
			Pretty: false,
			Strict: false,
		},
	)

	// 类型化 -> JSON（选项 I）
	// - 序列化器 = 解码器 + 编码器。 因为我们只需要编码器功能
	// 在这个例子中，我们可以传递 nil 而不是 MetaFactory、Creater 和
	// Typer 参数，因为它们仅由解码器使用。
	encoded, err := runtime.Encode(encoder, &obj)
	if err != nil {
		panic(err.Error())
	}
	fmt.Println("Serialized (option I)", string(encoded))

	// 类型 -> JSON（选项 II）
	// 其实就是Encoder.Encode()在JSON情况下的实现
	// 归结为调用 stdlib encoding/json.Marshal() 可选
	// 漂亮打印并将 JSON 转换为 YAML。
	encoded2, err := json.Marshal(obj)
	if err != nil {
		panic(err.Error())
	}
	fmt.Println("Serialized (option II)", string(encoded2))

	// JSON -> 类型
	// - 序列化器 = 解码器 + 编码器。
	// - jsonserializer.MetaFactory 是一个简单的部分 JSON 解组器，它
	// 在提供的文件中查找 APIGroup/Version 和 Kind 属性
	// 一段 JSON 并将它们解析为 schema.GroupVersionKind{} 对象。
	// - runtime.ObjectCreater 用于创建空类型的 runtime.Object
	//（例如，Deployment、Pod、ConfigMap 等）用于提供的 APIGroup/Version 和 Kind。
	// - runtime.ObjectTyper 是可选的 - 解码器接受一个可选的
	// `into runtime.Object` 参数，ObjectTyper 用于确保
	// MetaFactory 的 GroupVersionKind 与 `into` 参数中的匹配。
	decoder := jsonserializer.NewSerializerWithOptions(
		jsonserializer.DefaultMetaFactory, // jsonserializer.MetaFactory
		scheme.Scheme,                     // runtime.Scheme implements runtime.ObjectCreater
		scheme.Scheme,                     // runtime.Scheme implements runtime.ObjectTyper
		jsonserializer.SerializerOptions{
			Yaml:   false,
			Pretty: false,
			Strict: false,
		},
	)
	// 转换
	decoded, err := runtime.Decode(decoder, encoded)
	if err != nil {
		panic(err.Error())
	}
	fmt.Printf("Deserialized %#v\n", decoded)
}

func TestSerializeToYamlPractice(t *testing.T) {
	serializer := jsonserializer.NewSerializerWithOptions(
		// 解码(json -> runtime.Object)才需要传入参数，因为要找到k8s中对应的资源
		jsonserializer.DefaultMetaFactory,
		scheme.Scheme,
		scheme.Scheme,
		jsonserializer.SerializerOptions{
			Yaml:   true, // 转换yaml时使用
			Pretty: false,
			Strict: false,
		},
	)

	// Typed -> YAML
	// Runtime.Encode() is just a helper function to invoke Encoder.Encode()
	yamlEncode, err := runtime.Encode(serializer, &obj)
	if err != nil {
		panic(err.Error())
	}
	fmt.Println("Serialized (option I)", string(yamlEncode))

	// YAML -> Typed (through JSON, actually)
	decoded, err := runtime.Decode(serializer, yamlEncode)
	if err != nil {
		panic(err.Error())
	}
	fmt.Printf("Deserialized %#v\n", decoded)
}

func TestUnstructuredToJson(t *testing.T) {

	// 非结构化 -> JSON (选项一)
	// `UnstructuredJSONScheme`并不是一个方案，而是一个编解码器
	// runtime.Encode()只是一个辅助函数，用于调用UnstructuredJSONScheme.Encode()
	// 需要UnstructuredJSONScheme.Encode()是因为非结构化的实例可以是
	// 是一个单一的对象、一个列表或一个未知的运行时对象，所以需要对其进行一定的
	// 在将数据传递给json.Marshal()之前，需要进行一些预处理。
	bytes, err := runtime.Encode(unstructured.UnstructuredJSONScheme, &uConfigMap)
	fmt.Println("Serialized (option I)", string(bytes))

	// 非结构化 -> JSON (选项二)
	bytes, err = uConfigMap.MarshalJSON()
	if err != nil {
		panic(err.Error())
	}
	fmt.Println("Serialized (option II)", string(bytes))

	// JSON -> 非结构化（选项一）
	obj1, err := runtime.Decode(unstructured.UnstructuredJSONScheme, bytes)
	if err != nil {
		panic(err.Error())
	}

	// JSON -> 非结构化 (选项二)
	obj2 := &unstructured.Unstructured{}
	err = obj2.UnmarshalJSON(bytes)
	if err != nil {
		panic(err.Error())
	}
	if !reflect.DeepEqual(obj1, obj2) {
		panic("Unexpected configmap data")
	}

}

func TestUnstructuredToYaml(t *testing.T) {

	// YAML -> Unstructured (through JSON)
	jConfigMap, err := yaml.ToJSON([]byte(yConfigMap))
	if err != nil {
		panic(err.Error())
	}

	object, err := runtime.Decode(unstructured.UnstructuredJSONScheme, jConfigMap)
	if err != nil {
		panic(err.Error())
	}

	uConfigMap, ok := object.(*unstructured.Unstructured)
	if !ok {
		panic("unstructured.Unstructured expected")
	}

	if uConfigMap.GetName() != "my-configmap" {
		panic("Unexpected configmap data")
	}

}
