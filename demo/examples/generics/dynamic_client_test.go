package generics

import (
	initclient "K8s_demo/demo/examples/client"
	"context"
	"fmt"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/dynamic"
	"strings"
	"testing"
)

// convertUnstructuredToResource 将 Unstructured 对象转换为 k8s 对象
func convertUnstructuredToResource[T runtime.Object](unstructuredObj *unstructured.Unstructured) (T, error) {
	var t T
	err := runtime.DefaultUnstructuredConverter.FromUnstructured(unstructuredObj.Object, &t)
	if err != nil {
		return t, err
	}
	return t, nil
}

// convertUnstructuredListToResource 将 UnstructuredList 对象转换为 ListRes 对象
// ListRes对象是自定义的struct，类似appsv1.DeploymentList{}，corev1.PodList{}等
func convertUnstructuredListToResource[T runtime.Object](unstructuredObj *unstructured.UnstructuredList) (ListRes[T], error) {
	var t T

	listRes := ListRes[T]{Items: make([]T, 0)}
	err := runtime.DefaultUnstructuredConverter.FromUnstructured(unstructuredObj.Object, &listRes)
	if err != nil {
		return listRes, err
	}
	for _, k := range unstructuredObj.Items {
		err := runtime.DefaultUnstructuredConverter.FromUnstructured(k.Object, &t)
		listRes.Items = append(listRes.Items, t)
		if err != nil {
			return listRes, err
		}
	}

	return listRes, nil
}

// convertResourceToUnstructured 将 k8s 对象转换为 Unstructured 对象
func convertResourceToUnstructured[T runtime.Object](tt T) (*unstructured.Unstructured, error) {
	unstructuredObj, err := runtime.DefaultUnstructuredConverter.ToUnstructured(&tt)
	if err != nil {
		return nil, err
	}
	return &unstructured.Unstructured{Object: unstructuredObj}, nil
}

type GenericClient[T runtime.Object] struct {
	client dynamic.Interface
	gvr    string
}

func NewGenericClient[T runtime.Object](GVR string) *GenericClient[T] {
	if GVR == "" {
		panic("GVR empty error")
	}
	gc := &GenericClient[T]{
		client: initclient.ClientSet.DynamicClient,
		gvr:    GVR,
	}
	return gc
}

type Option func()

// WithNamespace
func WithNamespace(namespace string) Option {
	return func() {
		defaultNamespace = namespace
	}
}

// WithContext
func WithContext(ctx context.Context) Option {
	return func() {
		defaultContext = ctx
	}
}

func WithCreateOptions(opts metav1.CreateOptions) Option {
	return func() {
		defaultCreateOptions = opts
	}
}

func WithDeleteOptions(opts metav1.DeleteOptions) Option {
	return func() {
		defaultDeleteOptions = opts
	}
}

func WithListOptions(opts metav1.ListOptions) Option {
	return func() {
		defaultListOptions = opts
	}
}

func WithGetOptions(opts metav1.GetOptions) Option {
	return func() {
		defaultGetOptions = opts
	}
}

// WithNamespaceScope 支持 Namespace scope 与 非 Namespace scope 资源对象，默认取 Namespace scope
func WithNamespaceScope(namespaceScope bool) Option {
	return func() {
		defaultNamespaceScope = namespaceScope
	}
}

var (
	defaultNamespace      = "default"
	defaultNamespaceScope = true
	defaultContext        = context.Background()
	defaultCreateOptions  = metav1.CreateOptions{}
	defaultListOptions    = metav1.ListOptions{}
	defaultGetOptions     = metav1.GetOptions{}
	defaultDeleteOptions  = metav1.DeleteOptions{}
)

// Create
func (gc *GenericClient[T]) Create(tt T, opts ...Option) (T, error) {
	var t T
	unstructuredObj, err := convertResourceToUnstructured[T](tt)
	if err != nil {
		fmt.Printf("convert resource[%s] error: %s", gc.gvr, err)
		return t, err
	}
	for _, opt := range opts {
		opt()
	}

	var res *unstructured.Unstructured

	switch defaultNamespaceScope {
	case true:
		res, err = gc.client.Resource(parseGVR(gc.gvr)).Namespace(defaultNamespace).
			Create(defaultContext, unstructuredObj, defaultCreateOptions)
		if err != nil {
			fmt.Printf("create resource[%s] error: %s", gc.gvr, err)
			return t, err
		}
	case false:
		res, err = gc.client.Resource(parseGVR(gc.gvr)).Create(defaultContext, unstructuredObj, defaultCreateOptions)
		if err != nil {
			fmt.Printf("create resource[%s] error: %s", gc.gvr, err)
			return t, err
		}
	}

	return convertUnstructuredToResource[T](res)
}

// Delete
func (gc *GenericClient[T]) Delete(name string, opts ...Option) error {

	for _, opt := range opts {
		opt()
	}

	switch defaultNamespaceScope {
	case true:
		err := gc.client.Resource(parseGVR(gc.gvr)).Namespace(defaultNamespace).
			Delete(defaultContext, name, defaultDeleteOptions)
		if err != nil {
			fmt.Printf("delete resource[%s] error: %s", gc.gvr, err)
			return err
		}
	case false:
		err := gc.client.Resource(parseGVR(gc.gvr)).Delete(defaultContext, name, defaultDeleteOptions)
		if err != nil {
			fmt.Printf("delete resource[%s] error: %s", gc.gvr, err)
			return err
		}
	}

	return nil
}

// Get
func (gc *GenericClient[T]) Get(name string, opts ...Option) (T, error) {
	var t T
	for _, opt := range opts {
		opt()
	}

	var res *unstructured.Unstructured
	var err error

	switch defaultNamespaceScope {
	case true:
		res, err = gc.client.Resource(parseGVR(gc.gvr)).Namespace(defaultNamespace).
			Get(defaultContext, name, defaultGetOptions)
		if err != nil {
			fmt.Printf("get resource[%s] error: %s", gc.gvr, err)
			return t, err
		}
	case false:
		res, err = gc.client.Resource(parseGVR(gc.gvr)).Get(defaultContext, name, defaultGetOptions)
		if err != nil {
			fmt.Printf("get resource[%s] error: %s", gc.gvr, err)
			return t, err
		}
	}

	return convertUnstructuredToResource[T](res)
}

type ListRes[T runtime.Object] struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty" protobuf:"bytes,1,opt,name=metadata"`
	Items           []T
}

// List
func (gc *GenericClient[T]) List(opts ...Option) (ListRes[T], error) {
	var tt ListRes[T]
	for _, opt := range opts {
		opt()
	}

	var res *unstructured.UnstructuredList
	var err error

	switch defaultNamespaceScope {
	case true:
		res, err = gc.client.Resource(parseGVR(gc.gvr)).Namespace(defaultNamespace).
			List(defaultContext, defaultListOptions)
		if err != nil {
			fmt.Printf("list resource[%s] error: %s\n", gc.gvr, err)
			return tt, err
		}
	case false:
		res, err = gc.client.Resource(parseGVR(gc.gvr)).List(defaultContext, defaultListOptions)
		if err != nil {
			fmt.Printf("list resource[%s] error: %s\n", gc.gvr, err)
			return tt, err
		}
	}

	return convertUnstructuredListToResource[T](res)
}

// Watch
func (gc *GenericClient[T]) Watch(opts ...Option) watch.Interface {

	for _, opt := range opts {
		opt()
	}

	var res watch.Interface
	var err error

	switch defaultNamespaceScope {
	case true:
		res, err = gc.client.Resource(parseGVR(gc.gvr)).Namespace(defaultNamespace).
			Watch(defaultContext, defaultListOptions)
		if err != nil {
			fmt.Printf("watch resource[%s] error: %s\n", gc.gvr, err)
			return nil
		}
	case false:
		res, err = gc.client.Resource(parseGVR(gc.gvr)).Watch(defaultContext, defaultListOptions)
		if err != nil {
			fmt.Printf("watch resource[%s] error: %s\n", gc.gvr, err)
			return nil
		}
	}

	return res
}

// parseGVR 解析并指定资源对象 "apps/v1/deployments" "core/v1/pods" "batch/v1/jobs"
func parseGVR(gvr string) schema.GroupVersionResource {
	var group, version, resource string
	gvList := strings.Split(gvr, "/")

	// 防止越界
	if len(gvList) < 2 {
		panic("gvr input error, please input like format apps/v1/deployments or core/v1/pods")
	}

	if len(gvList) < 3 {
		group = ""
		version = gvList[0]
		resource = gvList[1]
	} else {
		if gvList[0] == "core" {
			gvList[0] = ""
		}
		group, version, resource = gvList[0], gvList[1], gvList[2]
	}

	return schema.GroupVersionResource{
		Group: group, Version: version, Resource: resource,
	}
}

func int32Ptr(i int32) *int32 {
	return &i
}

func TestGenericClient(t *testing.T) {
	gc := NewGenericClient[*appsv1.Deployment]("apps/v1/deployments")
	// 创建 Deployment 对象
	deployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "my-deployment",
			Namespace: "default",
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: int32Ptr(1),
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"app": "my-app",
				},
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"app": "my-app",
					},
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:  "my-container",
							Image: "nginx",
						},
					},
				},
			},
		},
	}
	// 创建
	_, err := gc.Create(deployment, WithContext(context.Background()),
		WithNamespace("default"), WithCreateOptions(metav1.CreateOptions{}))
	if err != nil {
		fmt.Println(err)
	}
	// 获取
	r, _ := gc.Get("my-deployment")
	fmt.Println("deploy name: ", r.Name)

	// 删除
	_ = gc.Delete("my-deployment")

	// 列表
	depList, _ := gc.List()
	for _, v := range depList.Items {
		fmt.Printf(v.Kind)
	}

	// watch
	rr := gc.Watch()
	go func() {
		aa := <-rr.ResultChan()
		fmt.Println(aa.Object)
	}()

	// 创建 ConfigMap 对象
	configMap := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "my-configmap",
			Namespace: "default",
		},
		Data: map[string]string{
			"key1": "value1",
			"key2": "value2",
		},
	}

	gcc := NewGenericClient[*corev1.ConfigMap]("v1/configmaps")
	_, err = gcc.Create(configMap)
	if err != nil {
		fmt.Println(err)
		return
	}

	kk, _ := gcc.List()

	for _, v := range kk.Items {
		fmt.Println("configmap name: ", v.Name)
	}

	gccr := NewGenericClient[*rbacv1.ClusterRole]("rbac.authorization.k8s.io/v1/clusterroles")

	// 创建 ClusterRole 对象 (非NamespacedScoped资源)
	clusterRole := &rbacv1.ClusterRole{
		ObjectMeta: metav1.ObjectMeta{
			Name: "my-cluster-role",
		},
		Rules: []rbacv1.PolicyRule{
			{
				APIGroups: []string{""},
				Resources: []string{"pods"},
				Verbs:     []string{"get", "list", "watch"},
			},
		},
	}

	cr, err := gccr.Create(clusterRole, WithNamespaceScope(false))
	if err != nil {
		fmt.Println(" err: ", err)
		return
	}

	cr, err = gccr.Get(cr.Name, WithNamespaceScope(false))
	if err != nil {
		fmt.Println(" err: ", err)
		return
	}

	err = gccr.Delete(cr.Name, WithNamespaceScope(false))
	if err != nil {
		fmt.Println(" err: ", err)
		return
	}

}
