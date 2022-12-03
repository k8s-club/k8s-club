## 标签和选择算符
#### 标签（Labels） 是附加到 Kubernetes 对象（比如 Pod）上的键值对。
#### 标签旨在用于指定对用户有意义且相关的对象的标识属性，但不直接对核心系统有语义含义。 
#### 标签可以用于组织和选择对象的子集。标签可以在创建时附加到对象，随后可以随时添加和修改。 
#### 每个对象都可以定义一组键/值标签。每个键对于给定对象必须是唯一的。
```bigquery
"metadata": {
  "labels": {
    "key1" : "value1",
    "key2" : "value2"
  }
}
```
```bigquery
"release" : "stable", "release" : "canary"
"environment" : "dev", "environment" : "qa", "environment" : "production"
"tier" : "frontend", "tier" : "backend", "tier" : "cache"
"partition" : "customerA", "partition" : "customerB"
"track" : "daily", "track" : "weekly"
```

### 命令行事例
```bigquery
kubectl get pods -l environment=production,tier=frontend
kubectl get pods -l 'environment in (production),tier in (frontend)'
kubectl get pods -l 'environment in (production, qa)'
kubectl get pods -l 'environment,environment notin (frontend)'
```


### client-go调用事例
```bigquery
// 字段选取器 label选取器的用法
labelSet := labels.SelectorFromSet(labels.Set(map[string]string{"app": "nginx"}))
listOptions := metav1.ListOptions{
    LabelSelector: labelSet.String(),
    FieldSelector: "status.phase=Running", // fmt.Sprintf("spec.ports[0].nodePort=%s", port)
    Limit:         500,
}
```

## 注解
#### 使用 Kubernetes 注解为对象附加任意的非标识的元数据。
#### 标签可以用来选择对象和查找满足某些条件的对象集合。 相反，注解不用于标识和选择对象。 
#### 注解中的元数据，可以很小，也可以很大，可以是结构化的，也可以是非结构化的，能够包含标签不允许的字符。
```bigquery
"metadata": {
  "annotations": {
    "key1" : "value1",
    "key2" : "value2"
  }
}
```
