## 如何在 client-go 中使用泛型？
目录：
- [1. 概述](#t1)
- [2. 动态客户端](#t2)
- [3. 泛型＋动态客户端](#t3)
- [4. Informer](#t4)
- [5. 总结](#t5)

### 1. <a name='t1'></a>概述：
在进行 k8s 相关开发时，会经常使用到 k8s 的 client-go 包或 Informer 来进行开发，免不了对多资源的 CRUD 操作。
通常，我们都使用 ClientSet 进行操作。

一般的 k8s 客户端可分为：四种
- RESTClient：最基础的客户端，主要是对 HTTP 请求进行了封装，支持 Json 和 Protobuf 格式的数据。
- DiscoveryClient：发现客户端，负责发现 APIServer 支持的资源组、资源版本和资源信息的。
- ClientSet：负责操作 Kubernetes 内置的资源对象，例如：Pod、Service 等。
- DynamicClient：动态客户端，可以对任意的 Kubernetes 资源对象进行通用操作，包括 CRD 。

如下示例：对 Configmap 资源对象的 CRUD 操作为例，可以看出 kubernetes 官方很好的封装了给使用方的资源操作接口，使用方可以轻易的调用与选择自己需要的资源对象进行操作。
```go
func createConfigMap(clientset *kubernetes.Clientset, namespace, name string, data map[string]string) error {
   configMap := &corev1.ConfigMap{
      ObjectMeta: metav1.ObjectMeta{
         Namespace: namespace,
         Name:      name,
      },
      Data: data,
   }

   _, err := clientset.CoreV1().ConfigMaps(namespace).Create(context.TODO(), configMap, metav1.CreateOptions{})
   if err != nil {
      return err
   }

   fmt.Printf("ConfigMap %s created\n", name)
   return nil
}

func getConfigMap(clientset *kubernetes.Clientset, namespace, name string) (*corev1.ConfigMap, error) {
   configMap, err := clientset.CoreV1().ConfigMaps(namespace).Get(context.TODO(), name, metav1.GetOptions{})
   if err != nil {
      return nil, err
   }

   return configMap, nil
}
```
然而：有方便自然也会有一些小缺点：所有资源都需要使用各自的操作接口，如果同时对接多种资源，会显得有很多重复性的代码。例如：创建 Pod 资源，必须使用 clientset.CoreV1().Pods ...，创建 Deployment 资源，必须使用 clientset.AppsV1().Deployments ..，其他资源依此类推。

> 是否有不需要频繁切换各资源，也能操作多种资源的方式呢？


### 2. <a name='t2'></a>动态客户端：

基于上述的问题，我们可以使用 k8s 提供的 DynamicClient（动态客户端）。
与 ClientSet 相比，DynamicClient 具有以下特点：
1. 适应性: DynamicClient 可以与任何 Kubernetes API 版本一起使用。它不需要手动切换。
2. 灵活性: DynamicClient 在处理多个不同类型资源的情况非常有用。可以处理自定义资源定义（CRD）以及 Kubernetes 核心资源，无需针对每个资源类型编写特定的代码。

```go
// ConfigMapClient 是一个用于操作 ConfigMap 的客户端
type ConfigMapClient struct {
    dynamicClient dynamic.Interface
}

// NewConfigMapClient 创建一个ConfigMapClient对象
func NewConfigMapClient(dynamicClient dynamic.Interface) *ConfigMapClient {
    return &ConfigMapClient{
            dynamicClient: dynamicClient,
    }
}

// Create 创建一个ConfigMap
func (c *ConfigMapClient) Create(namespace, name string, data map[string]string) error {
    configMap := &unstructured.Unstructured{
        Object: map[string]interface{}{
            "apiVersion": "v1",
            "kind":       "ConfigMap",
            "metadata": map[string]interface{}{
                    "name":      name,
                    "namespace": namespace,
            },
            "data": data,
        },
    }

    _, err := c.dynamicClient.Resource(schema.GroupVersionResource{Group: "", Version: "v1", Resource: "configmaps"}).
            Namespace(namespace).
            Create(context.TODO(), configMap, metav1.CreateOptions{})
    if err != nil {
            return err
    }

    fmt.Printf("ConfigMap %s created\n", name)
    return nil
}

// Get 获取指定的 ConfigMap
func (c *ConfigMapClient) Get(namespace, name string) (*unstructured.Unstructured, error) {
    result, err := c.dynamicClient.Resource(schema.GroupVersionResource{Group: "", Version: "v1", Resource: "configmaps"}).
            Namespace(namespace).
            Get(context.TODO(), name, metav1.GetOptions{})
    if err != nil {
            return nil, err
    }

    configMapObj := &unstructured.Unstructured{}
    err = runtime.DefaultUnstructuredConverter.FromUnstructured(result.Object, configMapObj)
    if err != nil {
            return nil, err
    }

    return configMapObj, nil
}
```

使用 DynamicClient 后，我们只需要指定好需要的 GVR（schema.GroupVersionResource），就能使用同一套代码创建不同资源。

然而：DynamicClient最复杂的一步就是需要频繁使用 unstructured.Unstructured 对象。unstructured.Unstructured 对象的本质是 map[string]interface{}，众所周知，golang 在使用 marshal unmarshal 捞这种嵌套结构时，会容易踩坑，再者，创建时，操作 unstructured.Unstructured 对象，总没有操作原生对象（ex: v1.Pod appsv1.Deployment）来的方便。

> 是否有既可以使用一份代码就操作所有资源，又可以不用操作 unstructured.Unstructured 对象的方法呢？

### 3. <a name='t3'></a>泛型＋动态客户端：
有！使用 golang 泛型 + DynamicClient 在某种程度上可以解决这种两者的问题。

话不多说，直接上代码！
- 创建出全局的客户端 GenericClient，其中 GVR 是指定的资源对象 

例如："apps/v1/deployments", "core/v1/pods", "batch/v1/jobs"等等，其中 core 组的资源对象支持不填 "core" ，亦即 pods configmaps 这种在 core 组下的资源对象可写 "core/v1/pods"  or "v1/pods" "core/v1/configmaps"  or "v1/configmaps"
```go
type GenericClient[T runtime.Object] struct {
   client dynamic.Interface
   gvr    string
}

func NewGenericClient[T runtime.Object](GVR string) *GenericClient[T] {
   if GVR == "" {
      panic("GVR empty error")
   }
   gc := &GenericClient[T]{
          client:          initclient.ClientSet.DynamicClient, 
	  discoveryClient: initclient.ClientSet.DiscoveryClient,
	  gvr:             GVR,
   }
   ...
   return gc
}
```
- 解析 GVR ，动态客户端需要传入 schema.GroupVersionResource 对象，parseGVR 方法是用于解析并指定 GVR 资源对象 
```go
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
```
- 为了在调用过程中不直接操作 *unstructured.Unstructured 对象，可以使用 runtime 包中的 DefaultUnstructuredConverter.FromUnstructured 方法，直接转换为传入的泛型类型对象

```go
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
```
- CRUD操作：

```go
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
	
    // 判断是否为 namespace scope 类型 
    isNamespace := isNamespaceScope(gc.restMapper, parseGVR(gc.gvr))
	
    switch isNamespace {
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

func (gc *GenericClient[T]) Get(name string, opts ...Option) (T, error) {
    var t T
    for _, opt := range opts {
        opt()
    }
    
    var res *unstructured.Unstructured
    var err error

    isNamespace := isNamespaceScope(gc.restMapper, parseGVR(gc.gvr))
	
    switch isNamespace {
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

func (gc *GenericClient[T]) Delete(name string, opts ...Option) error {

    for _, opt := range opts {
        opt()
    }

    isNamespace := isNamespaceScope(gc.restMapper, parseGVR(gc.gvr))
	
    switch isNamespace {
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

```

- 使用方法（以 Deployment 为例）：在初始化 client 时，只需指定泛型与 GVR ，就能对任意的指定资源进行 CRUD 操作，并且调用方在操作或是获取返回值时，仍来是操作原生的资源对象，不需要操作或是断言 unstructured.Unstructured 对象。

```go
func main() {
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
   // 使用选项模式 
   _, err := gc.Create(deployment, WithContext(context.Background()),
      WithNamespace("default"), WithCreateOptions(metav1.CreateOptions{}))
   if err != nil {
      fmt.Println(err)
   }

   r, _ := gc.Get("my-deployment")
   fmt.Println("dep name: ", r.Name)

   _ = gc.Delete("my-deployment")

   depList, _ := gc.List()

   for _, v := range depList.Items {
      fmt.Printf("dep name: ", v.Name)
   }


}
```

### 4. <a name='t4'></a>Informer：
当然，Informer 也可以按照同一个思路，使用泛型，直接上代码。
```go
type ResourceEventHandler[T runtime.Object] struct {
   AddFunc    func(obj T, isInInitialList bool)
   UpdateFunc func(obj T, new T)
   DeleteFunc func(obj T)
}

func (e *ResourceEventHandler[T]) OnAdd(obj interface{}, isInInitialList bool) {
   if o, ok := obj.(*unstructured.Unstructured); ok {
      rr, _ := convertUnstructuredToResource[T](o)
      e.AddFunc(rr, false)
   }
}

func (e *ResourceEventHandler[T]) OnUpdate(oldObj, newObj interface{}) {
   var t, tt *unstructured.Unstructured
   var ok bool
   if t, ok = oldObj.(*unstructured.Unstructured); !ok {
      return
   }
   if tt, ok = newObj.(*unstructured.Unstructured); !ok {
      return
   }
   oldT, err := convertUnstructuredToResource[T](t)
   if err != nil {
      return
   }
   newT, err := convertUnstructuredToResource[T](tt)
   if err != nil {
      return
   }
   e.UpdateFunc(oldT, newT)

}

func (e *ResourceEventHandler[T]) OnDelete(obj interface{}) {
   if o, ok := obj.(*unstructured.Unstructured); ok {
      rr, _ := convertUnstructuredToResource[T](o)
      e.DeleteFunc(rr)
   }
}

func main() {
   // dynamic客户端
   client := initclient.ClientSet.DynamicClient

   factory := dynamicinformer.NewDynamicSharedInformerFactory(client, 5*time.Second)
   deployDynamicInformer := factory.ForResource(parseGVR("apps/v1/deployments"))
   // eventHandler 回调
   deployHandler := &ResourceEventHandler[*appv1.Deployment]{
      AddFunc: func(deploy *appv1.Deployment, isInInitialList bool) {
         fmt.Println("on add deploy:", deploy.Name)
      },
      UpdateFunc: func(old *appv1.Deployment, new *appv1.Deployment) {
         fmt.Println("on update deploy:", new.Name)
      },
      DeleteFunc: func(deploy *appv1.Deployment) {
         fmt.Println("on delete deploy:", deploy.Name)
      },
   }

   deployDynamicInformer.Informer().AddEventHandler(deployHandler)

   podDynamicInformer := factory.ForResource(parseGVR("core/v1/pods"))
   // eventHandler 回调
   podHandler := &ResourceEventHandler[*v1.Pod]{
      AddFunc: func(pod *v1.Pod, isInInitialList bool) {
         fmt.Println("on add pod:", pod.Name)
      },
      UpdateFunc: func(old *v1.Pod, new *v1.Pod) {
         fmt.Println("on update pod:", new.Name)
      },
      DeleteFunc: func(pod *v1.Pod) {
         fmt.Println("on delete pod:", pod.Name)
      },
   }

   podDynamicInformer.Informer().AddEventHandler(podHandler)

   leaseDynamicInformer := factory.ForResource(parseGVR("coordination.k8s.io/v1/leases"))
   // eventHandler 回调

   leaseHandler := &ResourceEventHandler[*v12.Lease]{
      AddFunc: func(pod *v12.Lease, isInInitialList bool) {
         fmt.Println("on add lease:", pod.Name)
      },
      UpdateFunc: func(old *v12.Lease, new *v12.Lease) {
         fmt.Println("on update lease:", new.Name)
      },
      DeleteFunc: func(pod *v12.Lease) {
         fmt.Println("on delete lease:", pod.Name)
      },
   }

   leaseDynamicInformer.Informer().AddEventHandler(leaseHandler)

   ctx, cancel := context.WithTimeout(context.Background(), time.Second*10)
   defer cancel()

   fmt.Println("------开始使用informer监听------------")
   factory.Start(ctx.Done())

   for gvr, ok := range factory.WaitForCacheSync(ctx.Done()) {
      if !ok {
         log.Fatal(fmt.Sprintf("Failed to sync cache for resource %v", gvr))
      }
   }

    select {
    case <-ctx.Done():
        return
    }
}
```

### 5. <a name='t5'></a>总结：
上述代码仅是使用泛型 + dynamic 客户端的一个简易的示例，仅是做一个参考，读者可以自行优化与扩展。完整代码可[参考](../demo/examples/generics)

社区中其实也有[相关 issue ](https://github.com/kubernetes/kubernetes/issues/106846)进行讨论，不过这个议题比较小众，这可能也和 golang 本身泛型特性的功能还不完善有关。