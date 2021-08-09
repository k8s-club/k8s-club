# 我是如何编写k8s-Controller(@sxllwx)

官方Doc, 我认为可以当成圣经去读，**慢慢读，反复写，反复重构**:

- 对象的定义的一些参考和约束:  https://github.com/kubernetes/community/blob/master/contributors/devel/sig-architecture/api-conventions.md
- Controller的编写的一些参考和约束: https://github.com/kubernetes/community/blob/master/contributors/design-proposals/architecture/principles.md


做好一套(Controller + runtime.Object)我认为重要的几个点:

- 避免简单的封装
- 定义好需要被调协的对象(runtime.Object),并且选择合适的方式CRD+(ValidatingWebHook+MutatingWebHook, 这两个的WebHook和CRD的组合几乎可以完成第二种方式的所有的功能集)或者通过一个`extension-APIServer`扩展的对象
- 将对象拆分为`spec`和`status`两块，各自负责自己的语义，spec负责描述需要将对象调协到样子([在最早的代码中，使用desiredStateOfWorld描述](https://github.com/kubernetes/kubernetes/blob/b766960348daba51ea47a428175148b16c7a0bb3/pkg/controller/replication_controller.go#L55)), status描述的是**最近一次观察到**的状态
- 了解`ListAndWatch`, `Informer`, `WorkQueue`， 尤其是WorkQueue，是Reconcile逻辑的核心点

> 当前来讲除了少数基础对象(Pod, PVC, PV)中还保留了Phase字段，其他的对象大多都不拥有该字段，`Phase`字段的本质是状态机，而k8s的设计需要尽量避免状态机的存在。设计我们自己的对象尽量避免使用Phase【别杠，杠就是你赢】

## 纵览

> k8s中大量的组件使用Reconcile(所谓Reoncile指的就是驱动`actual`往`desired`递进)的逻辑完成k8s现役功能

比如: 

- ReplicasSet-Controller通过调协 `ReplicasSet` 对象和 `Pod` 对象 进而来保证集群中有指定数量的Pod正在运行.
- Deployment-Controller通过调协 `Deployment` 对象和 `ReplicasSet` 对象 进而保证集群中有特定数量版本的Pod正在运行.
- DaemonSet-Controller通过调协 `DaemonSet` 对象和 `Node` `Pod` 对象 进而保证集群中每个节点都拥有一个特定的Pod正在运行.

设计Controller之初需要考虑的问题:

- 假设ReplicasSet下的两个Pod都同时发生了变化，那么Replicas-Controller的`sync`逻辑会执行两次，还是一次，还是随机?
- 如果Controller需要更新对应的对象应该进行retry.RetryOnConflict还是回Queue?
- Controller是否能够更新对应的对象的spec字段?
- Informer中UpdateFunc前后对象为同一个`ResourceVersion`是否还要enqueue?

## 我的一些实践经验【仅供参考，欢迎讨论】

> 往往我们的Controller关注我们自定义的一个对象，为了让这个对象处于某种正常的状态那么需要使底层的其他Object处于正常，而做Reconcile的目标是**驱动`actual`往`desired`递进**。

我一般会在我的代码中定义一个名为`WorldView`的对象用来记录整个调协过程中观察到的状态，调协例程执行完成之后**不管成功与否**将统计到的所有状态一并刷新。

- 根据当前对象的**spec**计算出其他Object应该处于什么样状态
- 读取每个相关的Object，记录当前该Object的状态，
- 获取每个相关的Object，并且对其进行对比，是否满足预期，如果不符合预期，向对应的对象发送修改预期, 并对其**spec**进行修改
- 根据当前的`WorldView`修改**status**(这个地方我认为不应该使用retry)


## 不使用WorkQueue处理Event会出现以下一些奇奇怪怪的Bug

- 内存暴涨:  
   - https://github.com/kubernetes/kubernetes/issues/102718
   - https://github.com/kubernetes/kubernetes/issues/103789
