## 概述
Kubernetes(K8s) 作为当前最知名的容器编排工具，称得上是云原生(Cloud Native)时代的“操作系统”，熟悉和使用它是研发、运维、产品等的必备技能。本篇文章从发展历史、安装运行、资源、存储、网络、安全、管理、未来展望等方面约 680 个知识点概述了 K8s 的知识图谱，旨在帮助大家更好的了解 K8s 的相关知识，为业务、运维、创新打下坚实基础。

[![K8s_mindmap.png](https://p3-juejin.byteimg.com/tos-cn-i-k3u1fbpfcp/2778182056e34877aa4e88fd5f57d7fa~tplv-k3u1fbpfcp-zoom-1.image)](https://www.processon.com/view/link/60dfeb3e1e085359888fd3e3)

完整版链接：[https://www.processon.com/view/link/60dfeb3e1e085359888fd3e3](https://www.processon.com/view/link/60dfeb3e1e085359888fd3e3)

**名词简写**
> PV: Persistent Volume
PVC: Persistent Volume Claim
SA: Service Account
HA: High Available
HPA: Horizontal Pod Autoscaler
VPA: Vertical Pod Autoscaler
PSP: Pod Security Policy
PDB: Pod Disruption Budget
CRD: Custom Resource Definition
CSI: Container Storage Interface
CNI: Container Network Interface
CRI: Container Runtime Interface
OCI: Open Container Initiative
CNCF: Cloud Native Computing Foundation



## 1. 发展历史 `History`
随着 Docker 在容器技术站稳脚跟，并在多个场合挑战了其它玩家的切身利益，比如 Google、RedHat、CoreOS、Microsoft 等。Google 在 docker 项目刚兴起就祭出一剑：把内部生产验证的容器 lmctfy（Let Me Container That For You）开源。但面对 Docker的强势崛起，毫无招架之力。Docker 在容器界具有绝对的权威和话语权。Google 于是开出高价给 Docker，Docker 的技术 boss 也是联合创始人 Solomon Hykes 估计也是个理想主义者，对这橄榄枝置之不理。

Google 很无奈，于是联合 RedHat、CoreOS 等开源基础设施领域玩家们，共同牵头发起了一个名为 CNCF(Cloud Native Computing Foundation)的基金会。

![CNCF_logo.png](https://p3-juejin.byteimg.com/tos-cn-i-k3u1fbpfcp/5c6fbb47ab5743e781e2f100797e68ed~tplv-k3u1fbpfcp-zoom-1.image)


Borg 是 Google 最核心最底层的技术，托管给 CNCF 基金会，即是 Kubernetes。

2017 年 10 月，Docker 公司出人意料地宣布，将在自己的主打产品 Docker 企业版中内置 Kubernetes 项目，这标志着持续了近两年之久的“编排之争”至此落下帷幕。

Twitter 在 2019 年 5 月之后，不再使用 Apache Mesos，Aliyun 在 2019 年 7 月之后，不再支持 Docker Swarm。

Kubernetes 在整个社区推进“民主化”架构，即：从 API 到容器运行时的每一层，Kubernetes 项目都为开发者暴露出了可以扩展的插件机制，鼓励用户通过代码的方式介入 Kubernetes 项目的每一个阶段。就这样，在这种鼓励二次创新的整体氛围当中，Kubernetes 社区在 2016 年之后得到了空前的发展。更重要的是，不同于之前局限于“打包、发布”这样的 PaaS 化路线，这一次容器社区的繁荣，是一次完全以 Kubernetes 项目为核心的“百家争鸣”。

## 2. 架构 `Architecture`
Kubernetes 遵循非常传统的客户端/服务端的架构模式，客户端可以通过 RESTful 接口或者直接使用 kubectl 与 Kubernetes 集群进行通信，这两者在实际上并没有太多的区别，后者也只是对 Kubernetes 提供的 RESTful API 进行封装并提供出来。每一个 Kubernetes 集群都是由一组 Master 节点和一系列的 Worker 节点组成，其中 Master 节点主要负责存储集群的状态并为 Kubernetes 对象分配和调度资源。

![Architecture.png](https://p3-juejin.byteimg.com/tos-cn-i-k3u1fbpfcp/e2b94ec58dde41a499c03460cc72bea5~tplv-k3u1fbpfcp-zoom-1.image)


### Master (Control Plane)
作为管理集群状态的 Master 节点，它主要负责接收客户端的请求，安排容器的执行并且运行控制循环，将集群的状态向目标状态进行迁移。Master 节点内部由下面四个组件构成：

kube-apiserver: 负责处理来自客户端(client-go, kubectl)的请求，其主要作用就是对外提供 RESTful 的接口，包括用于查看集群状态的读请求以及改变集群状态的写请求，也是唯一一个与 etcd 集群通信的组件。

etcd: 是兼具一致性和高可用性的键值数据库，可以作为保存 Kubernetes 所有集群数据的后台数据库。

kube-scheduler: 主节点上的组件，该组件监视那些新创建的未指定运行节点的 Pod，并选择节点让 Pod 在上面运行。调度决策考虑的因素包括单个 Pod 和 Pod 集合的资源需求、硬件/软件/策略约束、亲和性和反亲和性规范、数据位置、工作负载间的干扰和最后时限。

kube-controller-manager: 在主节点上运行控制器的组件，从逻辑上讲，每个控制器都是一个单独的进程，但是为了降低复杂性，它们都被编译到同一个可执行文件，并在一个进程中运行。通过 list-watch event 事件触发相应控制器的调谐流程，这些控制器包括：Node Controller、Replication Controller、Endpoint Controller、ServiceAccount/Token Controller 等。

### Node (Worker)
kubelet: 是工作节点执行操作的 agent，负责具体的容器生命周期管理，根据从 etcd 中获取的信息来管理容器，并上报 pod 运行状态等。

kube-proxy: 是一个简单的网络访问代理，同时也是一个 Load Balancer。它负责将访问到某个服务的请求具体分配给工作节点上同一类 label 的 Pod。kube-proxy 实质就是通过操作防火墙规则(iptables或者ipvs)来实现 Pod 的映射。

container-runtime: 容器运行环境是负责运行容器的软件，Kubernetes 支持多个容器运行环境: Docker、 containerd、cri-o、 rktlet 以及任何实现 Kubernetes CRI(容器运行时接口)。


## 3. 安装运行 `Install & Run`
K8s 安装可以通过手动下载二进制包([https://github.com/kubernetes/kubernetes/releases](https://github.com/kubernetes/kubernetes/releases))进行，也可以通过第三方工具包安装集群环境，推荐使用后者安装。

目前常用的第三方工具有：Minikube, Kubeadm, Kind, K3S。

Minikube 适用于轻量级、单节点本地集群环境搭建，新手学习可以选用；Kubeadm 适用于完整 K8s master/node 多节点集群环境搭建，Kind 的特点是将 K8s 部署到 Docker 容器中，K3S 适用于轻量级、IoT 等微型设备上搭建集群环境。

## 4. 资源 `Resources`
在 K8s 中，可以把资源对象分为两类：Workloads(工作负载)、Controllers(控制器)。

Workloads 主要包含：Pod, Deployment, StatefulSet, Service, ConfigMap, Secret, DaemonSet, Job/CronJob, HPA, Namespace, PV/PVC, Node 等，主要是将各类型资源按需求和特性分类。

Controllers 主要包含：Node Controller, Replication Controller, Namespace Controller, ServiceAccount Controller, Service Controller, Endpoint Controller 等，主要作用是在资源自动化控制中，将各类型资源真实值(Actual)调谐(Reconcile)到期望值(Expect)。


所有资源通过 REST API 调用 kube-apiserver 进行 GET/POST/PATCH/REPLACE/DELETE 资源控制(增删改查)，需要满足接口的认证、授权、权限控制。kubectl 命令是官方提供的客户端命令行工具，封装了 REST API 调用 kube-apiserver。所有资源通过 kube-apiserver 持久化到 etcd 后端存储，因此生产实践中，需要同时保证 kube-apiserver, etcd 的高可用部署，防止单点故障。

## 5. 存储 `Storage`
Pod 中 Container 产生的数据需要持久化存储，特别是对于有状态(StatefulSet)服务，可以通过 PV/PVC 进行本地或网络存储，以便容器应用在重建之后仍然可以使用之前的数据。如果不需要持久化存储，可以使用 Ephemeral-Volume(emptyDir) 临时存储卷，数据会随着 Pod 的生命周期一起清除。

PV(PersistentVolume) 是对底层共享存储的抽象，将共享存储定义为一种“资源”。PV 由管理员手动创建或动态供给(dynamic provision)，通过插件式的机制完成与 CSI(Container Storage Interface) 具体实现进行对接，如 GlusterFS, iSCSI, GCE, AWS 公有云等。

PVC(PersistentVolumeClaim) 则是对存储资源的一个“申请”，就像 Pod “消费” Node 的资源一样，PVC 会 “消费” PV，两者需要在相同的命名空间，或者满足特定 SC(StorageClass)、Label Selector 的匹配，才可以完成绑定(Bound)。可以设置策略，当 PVC 删除时自动删除其绑定的 PV 资源，及时回收存储资源。

## 6. 网络 `Network`
K8s 网络模型设计的一个基础原则是：每个 Pod 都拥有一个独立的 IP 地址，即 IP-per-Pod 模型，并假定所有 Pod 都在一个可以直接连通的、扁平的网络空间中。所以不管容器是否运行在同一个 Node 中，都要求它们可以直接通过对方的 IP 进行访问。

实际上，K8s 中 IP 是以 Pod 为单位进行分配的，一个 Pod 内部的容器共享一个网络协议栈。而 Docker 原生的通过端口映射访问模式，会引入端口管理的复杂性，而且访问者看到的 IP 地址和端口，与服务提供者实际绑定的不同，因为 NAT 会改变源/目标地址，服务自身很难知道自己对外暴露的真实 IP 和端口。

因此，K8s 对集群网络有如下要求：
- 所有容器都可以在不用 NAT 方式下同别的容器通信；
- 所有节点都可以在不用 NAT 方式下同所有容器通信；
- 容器的 IP 和访问者看到的 IP 是相同的；

K8s 中将一组具有相同功能的容器应用抽象为服务(Service)，并提供统一的访问地址。可以通过 ClusterIP, NodePort, LoadBalancer 实现集群内、集群外的服务通信，并使用 Ingress(入网)、Egress(出网)等网络策略(NetworkPolicy)对出入容器的请求流量进行访问控制。

## 7. 调度 `Scheduler`

调度器(kube-scheduler) 在 K8s 集群中承担了“承上启下”的重要功能，“承上”是指它负责接收 Controller Manager 创建的 Pod，为其选择一个合适的 Node；“启下”是指选好的 Node 上的 kubelet 服务进程接管后续工作，将负责 Pod 生命周期的“下半生”。

K8s 提供的默认调度流程分为下面两步：
- 预选调度过程：通过多种预选策略(xxx Predicates)，筛选出符合要求的候选节点；
- 确定最优节点：在第一步基础上，采用优先级策略(xxx Priority) 计算出每个候选节点的分数(score)，得分最高者将被选中为目标 Node。

另外，Pod 可以通过亲和性(Affinity)、反亲和性(Anti-Affinity) 来设置，以偏好或者硬性要求的方式指示将 Pod 部署到相关的 Node 集合中。而 Taint/Tolerations 与此相反，允许 Node 抵制某些 Pod 的部署。Taint 与 Tolerations 一起工作确保 Pod 不会被调度到不合适的节点上。单个 Node 可以应用多个 Taint，Node 不接受无法容忍 Taint 的 Pod 调度。Tolerations 是 Pod 的属性，允许（非强制）Pod 调度到 Taint 相匹配的 Node 上去。

> Tips：Taint 是 Node 的属性，Affinity 是 Pod 的属性。

## 8. 安全 `Security`

K8s 通过一系列机制来实现集群的安全控制，其中包括 Authentication, Authorization, Admission Control, Secret, Service Account 等机制。K8s 中所有资源的访问、变更都是通过 K8s API Server 的 REST API 实现的，所以在 Authentication 认证中，采用了三种方式：
- HTTPS: CA + SSL
- HTTP Token
- HTTP Base

Authorization 授权模式包括：ABAC(Attribute-Based Access Control), RABC(Role-Based Access Control), Webhook 等，其中 RBAC 在生产实践中使用较多。基于角色的控制主要包括：Role, RoleBinding, ClusterRoleBinding，前面两者控制某个命名空间下的资源，后面两者控制集群级别或指定命名空间的资源。

通过了前面认证、授权两道关卡之后，还需要通过 Admission Control 所控制的一个准入控制链进行层层验证，包括资源、Pod 和其他各类准入控制插件，只有通过已开启的全部控制链，才能获得成功的 API Server 响应。

K8s 组件 kubectl, kubelet 默认通过名叫 default 的 SA(Service Account) 进行与 API Server 的安全通信，具体实现是通过将名叫 Token 的 Secret 挂载到容器目录下，访问时携带对应 Token 进行安全验证。

另外，K8s 提供了 PSP(PodSecurityPolicy) 机制对 Pod/Container 进行了更细粒度的安全策略控制，主要包括 host, user, group, privilege, Linux capabilities 的不同层面的控制，Pod/Container 中 securityContext 需要匹配对应策略才能创建成功。


## 9. 扩展 `Extensions`

随着 K8s 的发展，系统内置的 Pod, RC, Service, Deployment, ConfigMap 等资源对象已经不能满足用户多样化的业务、运维需求，就需要对 API 进行扩展。目前 K8s 提供了两种机制来扩展 API：

- CRD 扩展：复用 K8s API Server，需要提供 CRD Controller；
- API Aggregation layer：需要用户编写额外的 API Server，可以对资源进行更细粒度的控制；

实际上，API Aggregation 方式主要是通过 kube-proxy 对不同路径资源的请求，转发到用户自定义的 API Service handler 中处理。

另外，K8s 采用 out-of-tree 模式的插件机制，在不影响主干分支代码基础上，支持基础设施组件的扩展，包括：CSI, CNI, CRI, Device Plugins 等，将底层接口以插件方式抽象出来，用户只需要实现具体的接口即可将自己的基础设施能力接入 K8s。

## 10. 管理 `Management`
K8s 提供了不同维度的集群管理机制，包括 Node 封锁(cordon)、解除封锁(uncordon)、驱逐(drain)、PDB 主动驱逐保护、资源使用量控制(requests/limits)、Metrics Server 监控采集、Log、Audit 审计等方面。集群管理员还可以使用前面提到的 Affinity, Taints/Tolerations, Label, Annotation 等机制，满足集群多样化管理需求。

另外，集群运行需要考虑高可用(HA) 部署，可以从 etcd、master 组件、biz 容器等方面采用多副本高可用部署，保证集群的稳定运行。


## 11. 工具 `Tools`
K8s 提供了客户端工具 kubectl 供用户使用，该工具几乎集成了 API Server 可以处理的所有 API，具体使用请参考图中列出的常用命令，全部命令请参考官方文档说明。

针对复杂的业务类型，官方建议使用 Kustomize 工具包来灵活处理 yaml 文件，在高版本的 K8s 中 kubectl 命令已默认安装 Kustomize，无需单独安装。

另外，K8s 官方推荐使用包管理工具 Helm，通过将各种不同版本、不同环境、不同依赖的应用打包为 Chart，实现快速 CI/CD 部署、回滚、灰度发布等。

## 12. 未来展望 `Ongoing & Future`
本文中所涉及的 API、资源、字段、工具等可能因 K8s 项目发展太快，有些已经 Deprecated，请以官方最新为准。

目前，Kubernetes 已经充分涉足互联网、AI、区块链、金融等行业，可以预想的是将来会有越来越多的行业开始使用 Kubernetes，并且各行业实践会更深入。

当前 K8s 社区在支持 Windows container、GPU、VPA 垂直扩容、Cluster Federation 联合多集群、边缘计算、机器学习、物联网等方面进行开发工作，推动更多云原生产品落地实践，最大化利用云的能力、发挥云的价值。


### 参考资料
1. [Kubernetes 官方文档](https://kubernetes.io/)
2. [Kubernetes 权威指南：从 Docker 到 Kubernetes 实践全接触(第4版)](https://book.douban.com/subject/33444476/)
3. [https://www.padok.fr/en/blog/minikube-kubeadm-kind-k3s](https://www.padok.fr/en/blog/minikube-kubeadm-kind-k3s)
4. [https://segmentfault.com/a/1190000039790009](https://segmentfault.com/a/1190000039790009)
5. [https://www.cnblogs.com/lioa/p/12624601.html](https://www.cnblogs.com/lioa/p/12624601.html)
6. [https://blog.csdn.net/dkfajsldfsdfsd/article/details/80990736](https://blog.csdn.net/dkfajsldfsdfsd/article/details/80990736)

