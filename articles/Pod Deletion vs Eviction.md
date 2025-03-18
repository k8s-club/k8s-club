
# 前沿
本文基于 k8s 1.32.3

写在最前面：因为「驱逐」这个词被广义的使用为非自主发起的删除 Pod 的场景，因此即使是部分只有删除的场景，也会被理解为驱逐。因此限定本文中所说的驱逐为调用 Eviction API。

对于 Pod 的删除，K8s 提供了如下所示的两个 API 接口，一个是 PodDeletion，另一个是 PodEviction，这两个最终的目标都是将 Pod 删除，那么为什么需要区别设计两个 API？
```go
// pod Deletion
kuebClient.CoreV1().Pods(pod.Namespace).Delete(ctx, pod.Name, *deleteOptions)

// pod Eviction
kubeClient.CoreV1().Pods(pod.Namespace).EvictV1(ctx, eviction) // 通过 coreV1
kubeClient.PolicyV1().Evictions(eviction.Namespace).Evict(ctx, eviction) // 或者通过 policyV1
```

一句话结论: `PodEviction 是「受策略控制的」删除 Pod，而 PodDeletion 是「直接」删除 Pod`。

# 探究 PodEvction
接下来让我们走近内部看下，上述的两种通过 coreV1 或者 policyV1 调用 PodEvction 的方式，最终都是调用 Pod 下的 subresource eviction，如下述所示：

```go
// The EvictionExpansion interface allows manually adding extra methods to the ScaleInterface.
type EvictionExpansion interface {
	Evict(ctx context.Context, eviction *policy.Eviction) error
}

func (c *evictions) Evict(ctx context.Context, eviction *policy.Eviction) error {
	return c.GetClient().Post().
		AbsPath("/api/v1").
		Namespace(eviction.Namespace).
		Resource("pods").
		Name(eviction.Name).
		SubResource("eviction").
		Body(eviction).
		Do(ctx).
		Error()
}
```

那么，也就是说 Evict 所有的逻辑都是在 `pkg/registry/core/pod/storage/eviction.go` 内，简单来说流程如下：
1. 处于 `Pending`，`Succeeded`，`Failed` 的 Pod，执行 4
2. `Runing` 的 Pod:
- 若处于 Terminating (DeletionTimestamp != nil)，执行 4
- 若 UnReady (Pod.Status.Conditions.Ready == false) ：
  - 那么 pdb.Spec.UnhealthyPodEvictionPolicy == policyv1.AlwaysAllow, 执行 4
  - 若 pdb.Spec.UnhealthyPodEvictionPolicy == policyv1.IfHealthyBudget, 执行 3
- 若 Ready，执行 3
3. 查看删除该 Pod 是否违背 pdb，如果违背，那么返回拒绝驱逐。如果不违背，执行 4
4. 为 Pod 添加 DisruptionTarget(reason=EvictionByEvictionAPI) 的 Condition 后直接执行删除

> ps： 4 中所说的 DisruptionTarget Condition 是在 1.25 中引入，主要面向四种场景：\
> a. PreemptionByScheduler (Pod preempted by kube-scheduler) \
> b. DeletionByTaintManager (Pod deleted by taint manager due to  NoExecute taint)\
> c. EvictionByEvictionAPI (Pod evicted by Eviction API)\
> d. DeletionByPodGC (an orphaned Pod deleted by PodGC) \
> [MR](https://github.com/kubernetes/kubernetes/pull/110959)

Eviction 相较于 Deletion 来说，会考虑 pdb 的影响。违背 pdb 的驱逐会得到如下的返回：
```
WARN[0000] pod xxx in namespace test evicting failed
Error: Cannot evict pod as it would violate the pod's disruption budget.
```

ps: 一个 Pod 只能被一个 pdb 对象约束，如果存在多个 pdb，调用 Eviction API 会报错。

```
WARN[0000] pod xxx in namespace test evicting failed
Error: This pod has more than one PodDisruptionBudget, which the eviction subresource does not support.
```


# 一些 QA

* Q: 对于不同的场景，怎么选择何时使用 Eviction？
> A: 大体来说，Eviction 是在有预期的受控删除时，一些典型的场景：
> - drain node，我们做运维性质的清空 node 操作时，肯定不期望我们的服务受到影响。比如我们有一个 2 副本的实例，并且我们设置对应 pdb.Spec.MaxUnavailable 为 1，表明最多同时只能接受有一个副本不可用。那么如果这两个副本都在这个 node 上，在驱逐过程中一定会保证在其中一个副本被驱逐完成，并在新的节点上起来 Ready 之后，再开始驱逐另一个副本。
> - de-scheduler，我们在做 Rebalance 装箱优化过程中，会需要迁移一些运行中的 Pod 到新的节点上，这个动作的前提也是不影响服务。

对于在执行 drain 过程中违反 pdb 的 Pod，会得到以下输出，并在 5s 后重试。
```
evicting pod test/pause-deployment-6cb695d7d6-x7bt5
evicting pod test/pause-deployment-6cb695d7d6-fvgf8
evicting pod kube-system/tke-kube-state-metrics-0
...
error when evicting pods/"pause-deployment-6cb695d7d6-fvgf8" -n "test" (will retry after 5s): Cannot evict pod as it would violate the pod's disruption budget.
error when evicting pods/"pause-deployment-6cb695d7d6-x7bt5" -n "test" (will retry after 5s): Cannot evict pod as it would violate the pod's disruption budget.
...
```

* Q: 为什么能够看到一个 Eviction 的资源定义，但是通过 kubectl 看不到一个叫 Eviction 的资源对象？
> A：因为 Eviction 只是 Pod 对象下的一个子资源，相似的还有 Binding, PodLogOptions, PodPortForwardOptions, PodExecOptions, PodAttachOptions 对象等。

* Q: 有一个控制器来负责 eviction 对象吗？
> A: 没有。Eviction 仅限于 API，具体 API 内的执行流程如上述介绍所示。

* Q: 如果已经在 deploy,sts 上设置了优雅删除时间后，在调用 pod eviction API 时显示设置优雅删除时间，会听谁的？
> A: 对于 pod 的 deletionGracePeriodSeconds 而言，最终生效的优先级为：
> - Deletion Api 的 DeletionOption (也就是 Eviction API)
> - pod.Spec.TerminationGracePeriodSeconds (源自 workload（delpoy，sts）上 podTemplate 的 TerminationGracePeriodSeconds)

* Q: k8s 内部都有哪些场景会调用 Evict API？
> A: 看下来就 drain node 的时候调用了 Eviction。

* Q: Node NotReady 之后，发生的该 Node 上所有的 Pod 都被删除的场景，是通过驱逐实现的吗？
> A: 不是，是通过 Deletion API 实现的。为什么要这么做？我理解是因为此种情况下，kubelet 已经不工作或者断连了，所以在该 Node 上的 Pod 的状态也是不可靠的（Unknown），因此执行 Eviction 来尽可能的保护这部分 Pod 已经没有意义，因此直接执行了 Deletion。\
> ps: 对于现网真实运行的业务而言，删除 Pod 是个高危动作，尤其是对于有状态服务而言，比如 DB，中间件，因此需要避免因为管控面故障引起数据面故障。

* Q: 为什么在 Eviction 执行真正的 Delete 请求之前，还需要调用 Update 更新 Pod Condition？
> A: 首先这在 1.25 版本引入了新的 Condition `DisruptionTarget`，具体见 [MR](https://github.com/kubernetes/kubernetes/pull/110959)。个人理解这么做的意义有三个：
> 1. 之前在 Pod status 内缺少由于 disruption 造成 Pod 被删除的真实原因
> 2. 因为 Delete Pod 之后，Pod 可能长时间处于 Terminating（e.g. 有 finalizer, pv umount failed ...）。此时通过调用 Evict API 我们能够看到触发 Pod 删除的原因是 EvictionByEvictionAPI; 
> 3. 配合集群审计/监控。我们可以关注 Pod 对象的变化，查看到哪些 Pod 是被驱逐的


# 参考
- https://kubernetes.io/docs/concepts/scheduling-eviction/api-eviction/