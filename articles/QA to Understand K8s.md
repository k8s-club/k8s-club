
## 在掌握 K8s 路上，应该理解下面这些 QA：

- `controller` 中的 `retry` 和失败之后重新 `requeue` 的区别是什么？
> RetryOnConflict 表示在进行 K8s 对象增删改 的时候，可能存在冲突(被其他组件改动导致 resourceVersion 变化) 从而需要重试，一般通过获取最新 obj 重试即可成功；
>
> WorkQueue 中的对象，表示为了快速响应 ResourceEventHandler 调用，一般采用 queue 先存起来(即完成 response)，再通过 sync logic 异步解耦去 process。若 process 失败可 requeue 重新放回队列(可以设置 backoff 策略)，再次重试。
>
> controller 模式高纬度的视角是：获取当前所有相关资源的状态，其中有一个资源作为描述预期的核心对象，控制器通过对比预期状态，调整对下属资源进行 **CUD**；
>
> 基于这样的设计，在 reconcile 的流程中出现 Conflict, 证明需要 Reconcile 的对象已经发生了变化，个人(@sxllwx) 认为应该进行 requeue，等待下一次的 reconcile，重新获取这些对象然后进行 Reconcile。

- 如何理解 `Status` 中的 `Phase` 是一种状态机？
> Phase 是 Pod lifecycle 中一种抽象的 high-level 状态值，通过枚举值(Enumeration)实现，包括 Pending、Running、Failed 等，状态之间切换存在依赖限制，比如可以从 Pending => Running，但不能从 Running => Pending；
>
> 另外枚举值变更(如新增一种状态值)不便于旧版本兼容。因此社区建议使用 Conditions 代替 Phase [typical-status-properties](https://github.com/kubernetes/community/blob/master/contributors/devel/sig-architecture/api-conventions.md#typical-status-properties)。

- `Status` 中的 `Phase` 与 `Conditions` 区别是？
> Phase 是一种站在更高维度、聚合的总结性状态，通过枚举值(Enumeration)实现，状态之间存在依赖(状态机模式)。
>
> Conditions 则是一种通过对观察到的对象，进行实时计算所得的状态，提供了一种 `open-world` 开放视角，通过列表(Slice list) 实现，包括了 Status, Reason, Message 等字段，不依赖历史状态。

- 为什么 `Controller` 不直接从 `informer` 中取出 `Object` 进行处理，而是从 `WorkQueue` 中取一个 `String` 的 `Key`？
> 因为每个 Object 在 K8s 各组件内经过 Reconcile，obj 随时都在进行变化。Informer 中对象是以 key-accumulator 方式存储，即一个 obj 随着时间的变化存在很多版本，通过取 Key 间接取到最新的 obj，保证了取到的 obj 是实时最新的对象。
>
> 另外，为什么在 Controller 内使用 WorkQueue，还有以下两点考虑：
> 1. 避免 OOM。具体来说，是提升 Controller(Listener) 处理（接收）事件的速率，（直接放入WorkQueue，比完成复杂的 reconcile 流程要快很多很多），这样就能避免 informer 框架内的 processorListener 在向当前这个 Listener/Controller 派发事件时，向 pendingNotifications 中堆积过多事件，从而引发 OOM。
> 2. 减少 reconcile 次数，避免多次无意义的 reconcile。通过 WorkQueue 内部的实现机制，能够保证在处理一个 Object 之前哪怕其被添加了多次（在短时间内大量到来等），也只会被处理一次，极大的减少了 reconcile 的次数。同时每次 reconcile 从 Indexer 中取最新的 Object，而不是直接使用被通知的 Object，能够避免无意义的 reconcile。

- 如何理解 `k8s` 中对象的 `Status` 是一种观察到的情况？
> K8s 中对资源的调谐采用 水平触发(level-based)模式，在控制器 control-loop 中每次都是根据当前观察到的对象进行状态计算，因此 Status 是一种实时观察到的状态。

- 如何理解 `Controller` 是一种水平触发(level-based)，边缘触发(edge-based)只是一种优化？

概念来源：
> 触发事件的概念是从硬件信号产生 `中断` 的机制衍生过来的，其产生一个电平信号时，有水平触发（包括高电平、低电平），也有边缘触发（包括上升沿、下降沿触发等）。
>
> 水平触发 : 系统仅依赖于当前状态。即使系统错过了某个事件（可能因为故障挂掉了），当它恢复时，依然可以通过查看信号的当前状态来做出正确的响应。
>
> 边缘触发 : 系统不仅依赖于当前状态，还依赖于过去的状态。如果系统错过了某个事件（“边缘”），则必须重新查看该事件才能恢复系统。

在 K8s 中：
> 水平触发(level-based) 是一种只关心当前状态、不关心、不依赖历史状态的控制器机制，也是一种 crash-safe 的设计模式；缺点是会牺牲一定的性能，因为需要获取全量数据进行计算，而不是仅计算 delta 增量数据；
>
> 边缘触发(edge-based) 是一种依赖历史、当前状态的控制器机制，crash-safe 恢复需要依赖历史数据，优点是性能高，因为只需要在应用启动时进行一次全量数据计算，之后就可以仅计算 delta 增量数据。
>
> 在其他的组件稳定的情况下，两者等价；
> 在其他的组件不太稳定的情况下，两者对于错误的容忍度，水平触发的包容性更强；

- 如何理解 `k8s` 所谓的有状态和无状态(`stateless` 和 `stateful`)？
> 无状态(stateless) 服务表示应用重启、多副本之间没有依赖关系，容器运行不需要依赖历史数据，也就可以不需要数据的持久化；另外，对外提供的 service dns 也会动态变更(如 pod_xxx.ns.svc.cluster.local)，一般通过 Deployment 实现；
>
> 有状态(stateful) 服务表示应用重启、多副本之前存在依赖关系，容器运行需要依赖历史数据，也依赖多副本之间的关系(如数据库主从)，因此数据需要进行持久化存储，保证应用重启后数据保持一致性、完整性。另外，对外提供的 service dns 会保持不变(如 mysql_0/mysql_1.ns.svc.cluster.local)，一般通过 StatefulSet 实现；

- 如何理解论文中描述的 `borg` 将面向机器转向到了面向应用？
> 容器化(containerization) 将 data center、machine OS 进行了抽象封装，使得应用开发者不必关心底层物理机器资源，而是专注以应用(application) 为单位进行管理、部署和运维，提高资源利用率和 DevOps 效率；

- 有 `CRD` 为何还需要使用 `Extension APIServer`？
> CRD 是 K8s 提供的一种资源扩展方式，相关后端存储(etcd)、controller 都使用 K8s 内部组件实现，对象资源必会持久化到`etcd`；
>
> 而 extension APIServer 是 K8s 提供的另一种更加开放的扩展方式，以 Aggregator API 方式通过 proxy 到自定义 APIService，实现资源、控制器、GC等 都开放给用户，可以配置单独的 etcd 存储。

- `K8s` 代码组织方式是怎样的？`staging/vendor` 为何如此设计？
> K8s 相关 SIGs 核心代码在 /kubernetes/staging/src/k8s.io 目录下，相关 PR 修改基本都是在 staging 目录下；
>
> vendor 目录下是 K8s 自身依赖的一些第三方包，以及通过 symlink 软链接到 staging/src/k8s.io 的内部依赖包(/kubernetes/vendor/k8s.io)，这样组织代码，既可以很方便的以 vendor 方式统一进行包管理，又可以稳定的作为第三方的包引用(staging 代码结构变更，vendor 对外可保持不变)。

- `informer` 内在 `ListWatch` 的时候，是只 `list` 一次，之后靠 `watch` 来同步 `etcd`，还是会定时 `list` 呢？
> ListAndWatch 会在启动时一次性获取全部(可设置分页)对象 items，及对应的 resourceVersion，之后根据 resourceVersion 版本号不断 watch etcd 中 items 的 Add/Update/Delete 变化。

- `etcd cluster` 是否可以切换 `http` 与 `https`？
> 先通过 http 建立了 cluster，然后再用自签证书 https 来建立，这样就会报错：
>
> ```
> tls: first record does not look like a TLS handshake
> ```
> 
> 经过验证：无论是从 http => https，还是从 https => http 的切换都会报这个错，因为一旦建立 cluster 成功，则把连接的协议(http/https) 写入到 etcd 存储里了，不能再更改连接协议。
> 
> 解决：如果真正遇到需要切换协议，可尝试下面方式
> - 允许删除数据：删除后重新建立 cluster
> - 不允许删数据：可以尝试采用 [snapshot & restore](https://etcd.io/docs/v3.5/op-guide/recovery/) 进行快照与恢复操作

- pod 的节点亲和性为什么会存在 `pod.spec.nodeSelector` 和 `pod.spec.affinity.nodeAffinity` 两处设置的地方？
> 确实，可以同时在两处都设置 pod 的节点亲和性，但是两者之间存在一个重要的区别：
> - nodeSelector 内的亲和性条件必须都满足（是 and 的关系）
> - nodeAffinity 内的亲和性条件只要满足其中一个即可（是 Or 的关系）
>
> 这点区别可以从下述代码中直接看出
> ```
> // pkg/scheduler/framework/plugins/nodeaffinity/node_affinity.go
>
> // Match checks whether the pod is schedulable onto nodes according to
> // the requirements in both nodeSelector and nodeAffinity.
> func (s RequiredNodeAffinity) Match(node *v1.Node) (bool, error) {
>     if s.labelSelector != nil {
>        if !s.labelSelector.Matches(labels.Set(node.Labels)) {
>           return false, nil
>        }
>     }
>     if s.nodeSelector != nil {
>        // Match 内部只要 nodeAffinity 中的一项亲和性满足，就返回 true
>        return s.nodeSelector.Match(node)
>     }
>     return true, nil
> }
> ```

## 如何编写 `k8s-Controller`？ 
[Controller 设计概要](https://github.com/k8s-club/k8s-club/tree/master/controller/README.md)

