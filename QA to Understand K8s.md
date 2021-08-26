
## 在掌握 K8s 路上，应该理解下面这些 QA：

- `controller` 中的 `retry` 和失败之后重新 `requeue` 的区别是什么?

- 如何理解 `Status` 中的 `Phase` 是一种状态机？

- 如何为什么 `Controller` 不直接从 `informer` 中取出 `Object` 进行处理，而是从 `WorkQueue` 中取一个 `String` 的 `Key`？

- 如何理解 `k8s` 中对象的 `Status` 是一种观察到的情况？ 

- 如何理解 `Controller` 是一种水平触发，边缘触发只是一种优化？

- 如何理解 `k8s` 所谓的有状态和无状态(`headless` 和 `head svc`)？

- 如何理解论文中描述的 `borg` 将面向机器转向到了面向应用？

- 有 `CRD` 为何需要使用 `Extension APIServer`？
  
  CRD必会持久化到`etcd`，而extension APIServer可以选择不持久化

- `K8s` 代码组织方式是怎样的？`staging/vendor` 为何如此设计？
