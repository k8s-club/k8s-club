# K8s-club
## Welcome to K8s 👋
Let's learn, share and explore the K8s world together :)
<br>
Contributions are highly appreciated, please feel free to submit your PRs.

### Articles
- [QA to Understand K8s](./articles/QA%20to%20Understand%20K8s.md)
- [K8s 知识图谱](./articles/K8s%20系列(一)%20-%20知识图谱.md)
- [Controller 设计概要](./articles/如何编写%20K8s-Controller.md)
- [Informer 机制 - 概述](./articles/Informer机制%20-%20概述.md)
- [Informer 机制 - DeltaFIFO](./articles/Informer机制%20-%20DeltaFIFO.md)
- [Informer 机制 - Indexer](./articles/Informer机制%20-%20Indexer.md)
- [Informer 机制 - Resync](./articles/Informer机制%20-%20Resync.md)
- [浅谈 Informer](./articles/K8s%20系列(四)%20-%20浅谈%20Informer.md)
- [浅谈 Watch 实现](./articles/浅谈%20K8s%20Watch%20实现.md)
- [浅谈 CSI](./articles/K8s%20系列(五)%20-%20浅谈%20CSI.md)
- [浅谈 CNI](./articles/K8s%20系列(六)%20-%20浅谈%20CNI.md)
- [浅谈 CRI](./articles/浅谈%20K8s%20CRI.md)
- [K8s Pod IP 分配机制](./articles/K8s%20Pod%20IP%20分配机制.md)
- [K8s Service 网络机制](./articles/K8s%20Service%20网络机制.md)
- [K8s Kubelet 启动流程](./articles/K8s%20Kubelet%20启动流程.md)
- [Kubelet - Probe 探针](./articles/Kubelet%20-%20Probe%20探针.md)
- [K8s Scheduler Cache](./articles/K8s%20Scheduler%20Cache.md)
- [K8s Scheduler Queue](./articles/K8s%20Scheduler%20Queue.md)
- [K8s 原地变配 Inplace Vertical Scaling](./articles/K8s%20原地变配%20Inplace%20Vertical%20Scaling.md)
- [关于 Pod 的原地升级](./articles/关于Pod的原地升级.md)
- [Node 异常后 pod 将发生什么？](./articles/Node%20异常后%20pod%20将发生什么？.md)
- [记一次 K8s control-plane 排障经历](./articles/抓虫日记%20-%20kube-apiserver.md)
- [如何区分 K8s cmd args 与 Docker Entrypoint？](./articles/如何区分%20K8s%20cmd%20args%20与%20Docker%20Entrypoint？.md)
- [如何在 client-go 中使用泛型？](./articles/如何在%20client-go%20中使用泛型？.md)
- [从 0 到 1 手动搭建 K8s 集群](./articles/从%200%20到%201%20手动搭建%20K8s%20集群.md)
- [K8s 如何配置 etcd https 证书？](./articles/K8s%20系列(三)%20-%20如何配置%20etcd%20https%20证书？.md)
- [K8s PR 怎样才能被 merge？](./articles/K8s%20系列(二)%20-%20K8s%20PR%20怎样才能被%20merge？.md)
- [Krew - 高效管理 kubectl 插件](./articles/Krew%20-%20高效管理%20kubectl%20插件.md)

### K8s PRs
- [Faster ExtractList. Add ExtractListWithAlloc variant](https://github.com/kubernetes/kubernetes/pull/113362)
- [Fix issue that Audit Server could not correctly encode metav1.DeleteOption](https://github.com/kubernetes/kubernetes/pull/110110)
- [Fix goroutine leak in the DeleteCollection](https://github.com/kubernetes/kubernetes/pull/105606)
- [Explain the reason why metaclient special processing metav1.DeleteOptions encoding](https://github.com/kubernetes/kubernetes/pull/104573)
- [Lock-free broadcaster](https://github.com/kubernetes/kubernetes/pull/91602)
- [Improve DeltaFIFO function 'ListKeys'](https://github.com/kubernetes/kubernetes/pull/104725)
- [Fix delete nil pointer panic](https://github.com/kubernetes/kubernetes/pull/103232)
- [Unify controller worker num param threadiness to workers](https://github.com/kubernetes/kubernetes/pull/104231)
- [Add GC workqueue Forget to stop the rate limiter](https://github.com/kubernetes/kubernetes/pull/106029)
- [Fix kubectl unlabel response msg](https://github.com/kubernetes/kubernetes/pull/104372)
- [Fix klog lock release on panic error](https://github.com/kubernetes/klog/pull/272)
- [Scheduling: fix duplicate checks for number of enabled queue sort plugin](https://github.com/kubernetes/kubernetes/pull/110167)
- [Cleanup useless null pointer check about nodeInfo.Node() from snapshot for in-tree plugins](https://github.com/kubernetes/kubernetes/pull/117834)
- [Add error handling for Write() function](https://github.com/kubernetes/kubernetes/pull/105995)
- [Bugfix about update occupied in podGroup status](https://github.com/kubernetes-sigs/scheduler-plugins/pull/360)
- [Add kubebuilder FAQ section](https://github.com/kubernetes-sigs/kubebuilder/pull/3044)
- [Support krew search plugins by name and description](https://github.com/kubernetes-sigs/krew/pull/799)

### Examples Code
- [Controller](./demo/examples/controller)
- [Client](./demo/examples/client)
- [Informer](./demo/examples/informer)
- [Watch](./demo/examples/watch)
- [Workqueue](./demo/examples/workqueue)
- [Label](./demo/examples/label)
- [Serializer](./demo/examples/serializer)
- [Convert](./demo/examples/convert-type)

### Join us
Slack: https://join.slack.com/t/k8s-club/shared_invite/zt-x8xa3rpx-Vt4krR_ky6xK3XPAeEWlSg
