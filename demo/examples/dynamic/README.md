### 常见的资源使用dynamic client客户端操作

#### 动态客户端常用的 GVR 对应的是k8s中资源的标示。
```bigquery
// k8s.io/apimachinery/pkg/runtime/schema/group_version.go
// 对应一个 http 路径
type GroupVersionResource struct {
	Group    string
	Version  string
	Resource string
}
// 对应一个golang struct
type GroupVersionKind struct {
	Group   string
	Version string
	Kind    string
}
```
