TOC
- [1. 概述](#1-概述)
- [2. Kubelet 上报 Node 状态](#2-Kubelet-上报-Node-状态)
- [3. Kubelet 进程退出](#3-Kubelet-进程退出)
  - [3.1 Kubelet 在 5min 内恢复](#31-Kubelet-在-5min-内恢复)
  - [3.2 Kubelet 超过 5min 未恢复](#32-Kubelet-超过-5min-未恢复)
- [4. Node 重启 (~10s)](#4-Node-重启-10s)
- [5. Node 关机](#5-Node-关机)
  - [5.1 关机时长 <= 5min](#51-关机时长--5min)
  - [5.2 关机时长 > 5min](#52-关机时长--5min)
- [6. 小结](#6-小结)

> 本文 k 为 kubectl 的别名简写：`alias k='kubectl'`。

## 1. 概述
K8s 中的 pod 都运行在对应的 Node(节点) 上，Node 资源对象是 K8s 对底层机器资源进行的抽象，可以是物理机，也可以是虚拟机(VM)。在 Node 异常后，将会发生什么呢？

- kubelet 自身会定期更新状态到 kube-apiserver，通过参数 --node-status-update-frequency 指定上报频率，默认是 10s 上报一次。
- kube-controller-manager 会每隔 --node-monitor-period 时间去检查 kubelet 的状态，默认是 5s。
- 当 Node 失联一段时间后，kubernetes 判定 Node 为 NotReady 状态，这段时长通过 --node-monitor-grace-period 参数配置，默认 40s。
- 当 Node 失联一段时间后，kubernetes 判定 Node 为 UnHealthy 状态，这段时长通过 --node-startup-grace-period 参数配置，默认 1m。
- 当 Node 失联一段时间后，kubernetes 开始驱逐该 Node 上的 pod，这段时长是通过 --pod-eviction-timeout 参数配置，默认 5m。

本文将通过实操 kubelet 进程退出、Node 重启、Node 关机等几种场景，观察对应 Node、pod 的行为，探索 Node 异常后 pod 的处理机制，以期掌握 K8s 中的 Node 异常处理逻辑，为业务高可用实践打下基础。

## 2. Kubelet 上报 Node 状态
> 上报频率由 `nodeStatusUpdateFrequency` 和 `nodeStatusReportFrequency` 字段进行控制。

nodeStatusUpdateFrequency 是 kubelet 计算节点状态的频率。 如果没有启用节点租约功能，这也是 kubelet 将节点状态发布给 master 的频率。 注意：当没有启用节点租约功能时，更改常量时要小心，它必须与 NodeController 中的 nodeMonitorGracePeriod 配合使用。 默认值：“10s”

nodeStatusReportFrequency 是 kubelet 在节点状态没有改变的情况下将节点状态发布给 master 的频率。 如果检测到任何更改，Kubelet 将忽略此频率并立即发布节点状态。 它仅在启用节点租用功能时使用。 nodeStatusReportFrequency 的默认值为 5m。 但是，如果显式设置了 nodeStatusUpdateFrequency，为了向后兼容，nodeStatusReportFrequency 的默认值将设置为 nodeStatusUpdateFrequency。 默认值：“5m”

本文操作环境开启了节点租约功能：
```
k get lease -n kube-node-lease 
NAME           HOLDER         AGE
10.0.240.125   10.0.240.125   453d
10.0.240.132   10.0.240.132   489d
10.0.240.5     10.0.240.5     489d
```

kubelet 每 5min 上报一次状态信息：
```
k get no 10.0.240.5 -oyaml | less

  - lastHeartbeatTime: "2022-12-11T16:19:21Z"
    lastTransitionTime: "2022-12-11T16:07:00Z"
    message: kubelet has sufficient memory available
    reason: KubeletHasSufficientMemory
    status: "False"
    type: MemoryPressure
  - lastHeartbeatTime: "2022-12-11T16:19:21Z"
    lastTransitionTime: "2022-12-11T16:07:00Z"
    message: kubelet has no disk pressure
    reason: KubeletHasNoDiskPressure
    status: "False"
    type: DiskPressure
  - lastHeartbeatTime: "2022-12-11T16:19:21Z"
    lastTransitionTime: "2022-12-11T16:07:00Z"
    message: kubelet has sufficient PID available
    reason: KubeletHasSufficientPID
    status: "False"
    type: PIDPressure
  - lastHeartbeatTime: "2022-12-11T16:19:21Z"
    lastTransitionTime: "2022-12-11T16:07:00Z"
    message: kubelet is posting ready status
    reason: KubeletReady
    status: "True"
    type: Ready
```

5min 后：
```
  - lastHeartbeatTime: "2022-12-11T16:24:22Z"
    lastTransitionTime: "2022-12-11T16:07:00Z"
    message: kubelet has sufficient memory available
    reason: KubeletHasSufficientMemory
    status: "False"
    type: MemoryPressure
  - lastHeartbeatTime: "2022-12-11T16:24:22Z"
    lastTransitionTime: "2022-12-11T16:07:00Z"
    message: kubelet has no disk pressure
    reason: KubeletHasNoDiskPressure
    status: "False"
    type: DiskPressure
  - lastHeartbeatTime: "2022-12-11T16:24:22Z"
    lastTransitionTime: "2022-12-11T16:07:00Z"
    message: kubelet has sufficient PID available
    reason: KubeletHasSufficientPID
    status: "False"
    type: PIDPressure
  - lastHeartbeatTime: "2022-12-11T16:24:22Z"
    lastTransitionTime: "2022-12-11T16:07:00Z"
    message: kubelet is posting ready status
    reason: KubeletReady
    status: "True"
    type: Ready
```

kubelet 进程退出后：
```
  - lastHeartbeatTime: "2022-12-11T16:34:23Z"
    lastTransitionTime: "2022-12-11T16:37:04Z"
    message: Kubelet stopped posting node status.
    reason: NodeStatusUnknown
    status: Unknown
    type: MemoryPressure
  - lastHeartbeatTime: "2022-12-11T16:34:23Z"
    lastTransitionTime: "2022-12-11T16:37:04Z"
    message: Kubelet stopped posting node status.
    reason: NodeStatusUnknown
    status: Unknown
    type: DiskPressure
  - lastHeartbeatTime: "2022-12-11T16:34:23Z"
    lastTransitionTime: "2022-12-11T16:37:04Z"
    message: Kubelet stopped posting node status.
    reason: NodeStatusUnknown
    status: Unknown
    type: PIDPressure
  - lastHeartbeatTime: "2022-12-11T16:34:23Z"
    lastTransitionTime: "2022-12-11T16:37:04Z"
    message: Kubelet stopped posting node status.
    reason: NodeStatusUnknown
    status: Unknown
    type: Ready
```

```
k describe no 10.0.240.5 | less

Conditions:
  Type                         Status    LastHeartbeatTime                 LastTransitionTime                Reason              Message
  ----                         ------    -----------------                 ------------------                ------              -------
  NetworkUnavailable           False     Sun, 03 Oct 2021 15:37:38 +0800   Sun, 03 Oct 2021 15:37:38 +0800   RouteCreated        RouteController created a route
  RouteENINetworkUnavailable   False     Mon, 12 Dec 2022 00:07:10 +0800   Mon, 12 Dec 2022 00:07:10 +0800   HasENIIP            eni-ip resources of node is set
  MemoryPressure               Unknown   Mon, 12 Dec 2022 00:34:23 +0800   Mon, 12 Dec 2022 00:37:04 +0800   NodeStatusUnknown   Kubelet stopped posting node status
.
  DiskPressure                 Unknown   Mon, 12 Dec 2022 00:34:23 +0800   Mon, 12 Dec 2022 00:37:04 +0800   NodeStatusUnknown   Kubelet stopped posting node status
.
  PIDPressure                  Unknown   Mon, 12 Dec 2022 00:34:23 +0800   Mon, 12 Dec 2022 00:37:04 +0800   NodeStatusUnknown   Kubelet stopped posting node status
.
  Ready                        Unknown   Mon, 12 Dec 2022 00:34:23 +0800   Mon, 12 Dec 2022 00:37:04 +0800   NodeStatusUnknown   Kubelet stopped posting node status
.

...

Events:
  Type     Reason                   Age                From        Message
  ----     ------                   ----               ----        -------
  Normal   Starting                 56m                kubelet     Starting kubelet.
  Normal   NodeReady                56m                kubelet     Node 10.0.240.5 status is now: NodeReady
  Normal   NodeAllocatableEnforced  56m                kubelet     Updated Node Allocatable limit across pods
  Normal   NodeHasNoDiskPressure    56m (x2 over 56m)  kubelet     Node 10.0.240.5 status is now: NodeHasNoDiskPressure
  Normal   NodeHasSufficientPID     56m (x2 over 56m)  kubelet     Node 10.0.240.5 status is now: NodeHasSufficientPID
  Warning  Rebooted                 56m                kubelet     Node 10.0.240.5 has been rebooted, boot id: 954314ff-5ebb-413e-9207-1fe7496c3b70
  Normal   NodeHasSufficientMemory  56m (x2 over 56m)  kubelet     Node 10.0.240.5 status is now: NodeHasSufficientMemory
  Normal   Starting                 56m                kube-proxy  Starting kube-proxy.
  Normal   NodeAllocatableEnforced  33m                kubelet     Updated Node Allocatable limit across pods
  Normal   Starting                 33m                kubelet     Starting kubelet.
  Normal   NodeHasSufficientMemory  33m (x2 over 33m)  kubelet     Node 10.0.240.5 status is now: NodeHasSufficientMemory
  Normal   NodeHasNoDiskPressure    33m (x2 over 33m)  kubelet     Node 10.0.240.5 status is now: NodeHasNoDiskPressure
  Normal   NodeHasSufficientPID     33m (x2 over 33m)  kubelet     Node 10.0.240.5 status is now: NodeHasSufficientPID
  Warning  Rebooted                 33m                kubelet     Node 10.0.240.5 has been rebooted, boot id: c89c034d-1cfd-47eb-a7b6-d0c658da00e6
  Normal   NodeReady                33m                kubelet     Node 10.0.240.5 status is now: NodeReady
  Normal   Starting                 33m                kube-proxy  Starting kube-proxy.
```

## 3. Kubelet 进程退出

```
停止 kubelet 进程：
systemctl stop kubelet

查看 kubelet 状态：
systemctl status kubelet

● kubelet.service - kubelet
   Loaded: loaded (/usr/lib/systemd/system/kubelet.service; enabled; vendor preset: disabled)
   Active: inactive (dead) since Sun 2022-12-11 21:04:56 CST; 1h 40min ago
  Process: 4173727 ExecStartPost=/bin/bash /etc/kubernetes/deny-tcp-port-10250.sh (code=exited, status=0/SUCCESS)
  Process: 4173726 ExecStart=/usr/bin/kubelet ${AUTHORIZATION_MODE} ${NON_MASQUERADE_CIDR} ${KUBECONFIG} ${CLUSTER_DNS} ${HOSTNAME_OVERRIDE} ${KUBE_RESERVED} ${POD_INFRA_CONTAINER_IMAGE} ${NETWORK_PLUGIN} ${READ_ONLY_PORT} ${REGISTER_SCHEDULABLE} ${SERIALIZE_IMAGE_PULLS} ${CLOUD_PROVIDER} ${FAIL_SWAP_ON} ${CLOUD_CONFIG} ${IMAGE_PULL_PROGRESS_DEADLINE} ${EVICTION_HARD} ${CLIENT_CA_FILE} ${MAX_PODS} ${ANONYMOUS_AUTH} ${V} ${AUTHENTICATION_TOKEN_WEBHOOK} ${CLUSTER_DOMAIN} (code=exited, status=0/SUCCESS)
 Main PID: 4173726 (code=exited, status=0/SUCCESS)

Dec 11 21:03:51 VM-240-5-centos kubelet[4173726]: I1211 21:03:51.832648 4173726 controlbuf.go:508] transport: loopyWriter.run returning. connection erro... closing"
Dec 11 21:04:56 VM-240-5-centos systemd[1]: Stopping kubelet...
Dec 11 21:04:56 VM-240-5-centos systemd[1]: Stopped kubelet.
```

Node 立即变为 NotReady，pod 在等待 5min 后，变为 Terminating：
```
k get no
                        
NAME           STATUS     ROLES    AGE    VERSION
10.0.240.125   Ready      <none>   388d   v1.18.4-tke.14
10.0.240.132   Ready      <none>   434d   v1.18.4-tke.14
10.0.240.5     NotReady   <none>   434d   v1.18.4-tke.14
```

Deploy/STS pod 状态：
```
k get po -owide

NAME                                        READY   STATUS        RESTARTS   AGE    IP           NODE           NOMINATED NODE   READINESS GATES
job-demo-ghnhj                              0/1     Pending       0          8s     <none>       <none>         <none>           <none>
mysql-0                                     2/2     Terminating   0          75m    10.4.0.144   10.0.240.5     <none>           <none>
mysql-1                                     2/2     Running       0          71m    10.4.0.87    10.0.240.132   <none>           <none>
mysql-2                                     2/2     Running       0          71m    10.4.0.88    10.0.240.132   <none>           <none>
nginx-deployment-5bf87f5f59-6k8z6           1/1     Running       0          55m    10.4.0.93    10.0.240.132   <none>           <none>
nginx-deployment-5bf87f5f59-6sk2m           1/1     Running       0          110m   10.4.0.85    10.0.240.132   <none>           <none>
nginx-deployment-5bf87f5f59-7jznh           1/1     Terminating   0          110m   10.4.0.137   10.0.240.5     <none>           <none>
```

DS pod 状态不变：
```
k get po -n kube-system -owide | grep router

kube-router-4qqsj                      1/1     Running       0          123d   10.0.240.132   10.0.240.132   <none>           <none>
kube-router-lws2j                      1/1     Running       0          123d   10.0.240.5     10.0.240.5     <none>           <none>
kube-router-m7ltf                      1/1     Running       0          123d   10.0.240.125   10.0.240.125   <none>           <none>
```

但是，exec/logs 会报错：
```
k exec -it -n kube-system kube-router-lws2j -- bash

Defaulted container "kube-router" out of: kube-router, install-cni (init)
Error from server: error dialing backend: dial tcp 10.0.240.5:10250: connect: connection refused


k logs -n kube-system kube-router-lws2j                                                 
Error from server: Get "https://10.0.240.5:10250/containerLogs/kube-system/kube-router-lws2j/kube-router": dial tcp 10.0.240.5:10250: connect: connection refused
```

kubectl exec:
```
k exec -it nginx-deployment-5bf87f5f59-7jznh -- bash

Error from server: error dialing backend: dial tcp 10.0.240.5:10250: connect: connection refused
```

kubectl logs:
```
k logs nginx-deployment-5bf87f5f59-7jznh                                                               
Error from server: Get "https://10.0.240.5:10250/containerLogs/demo/nginx-deployment-5bf87f5f59-7jznh/nginx": dial tcp 10.0.240.5:10250: connect: connection refused
```

Node 状态：
kubectl describe no 10.0.240.5 | less
```
Unschedulable:      false
Lease:
  HolderIdentity:  10.0.240.5
  AcquireTime:     <unset>
  RenewTime:       Sun, 11 Dec 2022 21:04:49 +0800
Conditions:
  Type                         Status    LastHeartbeatTime                 LastTransitionTime                Reason              Message
  ----                         ------    -----------------                 ------------------                ------              -------
  NetworkUnavailable           False     Sun, 03 Oct 2021 15:37:38 +0800   Sun, 03 Oct 2021 15:37:38 +0800   RouteCreated        RouteController created a route
  MemoryPressure               Unknown   Sun, 11 Dec 2022 21:01:08 +0800   Sun, 11 Dec 2022 21:05:30 +0800   NodeStatusUnknown   Kubelet stopped posting node status.
  DiskPressure                 Unknown   Sun, 11 Dec 2022 21:01:08 +0800   Sun, 11 Dec 2022 21:05:30 +0800   NodeStatusUnknown   Kubelet stopped posting node status.
  PIDPressure                  Unknown   Sun, 11 Dec 2022 21:01:08 +0800   Sun, 11 Dec 2022 21:05:30 +0800   NodeStatusUnknown   Kubelet stopped posting node status.
  Ready                        Unknown   Sun, 11 Dec 2022 21:01:08 +0800   Sun, 11 Dec 2022 21:05:30 +0800   NodeStatusUnknown   Kubelet stopped posting node status.
```

DS kube-router pod 依然保持 Running:
```
  serviceAccountName: kube-router
  terminationGracePeriodSeconds: 30
  tolerations:
  - key: CriticalAddonsOnly
    operator: Exists
  - effect: NoSchedule
    key: node-role.kubernetes.io/master
    operator: Exists
  - effect: NoExecute
    key: node.kubernetes.io/not-ready
    operator: Exists
  - effect: NoExecute
    key: node.kubernetes.io/unreachable
    operator: Exists
  - effect: NoSchedule
    key: node.kubernetes.io/disk-pressure
    operator: Exists
  - effect: NoSchedule
    key: node.kubernetes.io/memory-pressure
    operator: Exists
  - effect: NoSchedule
    key: node.kubernetes.io/pid-pressure
    operator: Exists
  - effect: NoSchedule
    key: node.kubernetes.io/unschedulable
    operator: Exists
  - effect: NoSchedule
    key: node.kubernetes.io/network-unavailable
    operator: Exists
```

node 增加了 taints:
```
spec:
  podCIDR: 10.4.0.128/27
  ...
  taints:
  - effect: NoSchedule
    key: node.kubernetes.io/unreachable
    timeAdded: "2022-12-11T13:05:30Z"
  - effect: NoSchedule
    key: node.cloudprovider.kubernetes.io/shutdown
  - effect: NoExecute
    key: node.kubernetes.io/unreachable
    timeAdded: "2022-12-11T13:05:35Z"
```

job 手动增加 tolerations:
```
tolerations:
  - key: node.kubernetes.io/unreachable
    operator: Exists
    effect: NoSchedule
  - key: node.cloudprovider.kubernetes.io/shutdown
    operator: Exists
    effect: NoSchedule
```

之后调度成功，但无法正常运行：

```
k get po -owide

job-demo-rltj7                              0/1     Pending       0          82s    <none>       10.0.240.5     <none>           <none>
```

```
Events:
  Type    Reason     Age   From               Message
  ----    ------     ----  ----               -------
  Normal  Scheduled  38s   default-scheduler  Successfully assigned demo/job-demo-rltj7 to 10.0.240.5
```

### 3.1 Kubelet 在 5min 内恢复
若在 5min 内 kubelet 进程重新启动，则 pod 都变为正常，不会有 restart：
```
k get no -w
                      
NAME           STATUS   ROLES    AGE    VERSION
10.0.240.125   Ready    <none>   388d   v1.18.4-tke.14
10.0.240.132   Ready    <none>   435d   v1.18.4-tke.14
10.0.240.5     Ready    <none>   435d   v1.18.4-tke.14
10.0.240.5     NotReady   <none>   435d   v1.18.4-tke.14
10.0.240.5     NotReady   <none>   435d   v1.18.4-tke.14
10.0.240.5     NotReady   <none>   435d   v1.18.4-tke.14
10.0.240.5     NotReady   <none>   435d   v1.18.4-tke.14
10.0.240.5     NotReady   <none>   435d   v1.18.4-tke.14
10.0.240.5     NotReady   <none>   435d   v1.18.4-tke.14
10.0.240.5     NotReady   <none>   435d   v1.18.4-tke.14
10.0.240.5     Ready      <none>   435d   v1.18.4-tke.14
10.0.240.5     Ready      <none>   435d   v1.18.4-tke.14
10.0.240.5     Ready      <none>   435d   v1.18.4-tke.14
```

```
k get po -owide

NAME                                        READY   STATUS    RESTARTS   AGE     IP           NODE           NOMINATED NODE   READINESS GATES
job-demo-fkxl8                              1/1     Running   0          5m26s   10.4.0.131   10.0.240.5     <none>           <none>
mysql-0                                     2/2     Running   0          13m     10.4.0.158   10.0.240.5     <none>           <none>
mysql-1                                     2/2     Running   0          16h     10.4.0.79    10.0.240.132   <none>           <none>
mysql-2                                     2/2     Running   0          12m     10.4.0.130   10.0.240.5     <none>           <none>
nginx-deployment-5bf87f5f59-h27wt           1/1     Running   0          16h     10.4.0.78    10.0.240.132   <none>           <none>
nginx-deployment-5bf87f5f59-sqdfn           1/1     Running   0          13m     10.4.0.154   10.0.240.5     <none>           <none>
```

### 3.2 Kubelet 超过 5min 未恢复
若超过 5min，pod 变为 Terminating，并新创建期望个数的 pod：
```
k get po -owide
                   
NAME                                        READY   STATUS        RESTARTS   AGE     IP           NODE           NOMINATED NODE   READINESS GATES
job-demo-kxt9r                              0/1     Pending       0          78s     <none>       10.0.240.5     <none>           <none>
job-demo-rltj7                              0/1     Terminating   0          6m18s   <none>       10.0.240.5     <none>           <none>
```

5min 后，恢复 Node 上的 kubelet 进程：
```
systemctl start kubelet

查看状态：
systemctl status kubelet
● kubelet.service - kubelet
   Loaded: loaded (/usr/lib/systemd/system/kubelet.service; enabled; vendor preset: disabled)
   Active: active (running) since Sun 2022-12-11 22:55:48 CST; 13s ago
  Process: 3340638 ExecStartPost=/bin/bash /etc/kubernetes/deny-tcp-port-10250.sh (code=exited, status=0/SUCCESS)
 Main PID: 3340637 (kubelet)
    Tasks: 25
   Memory: 43.1M
   CGroup: /system.slice/kubelet.service
           └─3340637 /usr/bin/kubelet --authorization-mode=Webhook --non-masquerade-cidr=0.0.0.0/0 --kubeconfig=/etc/kubernetes/kubelet-kubeconfig --cluster-dns=...
```

Node status.conditions:
```
k get no 10.0.240.5 -oyaml | less

conditions:
  - lastHeartbeatTime: "2021-10-03T07:37:38Z"
    lastTransitionTime: "2021-10-03T07:37:38Z"
    message: RouteController created a route
    reason: RouteCreated
    status: "False"
    type: NetworkUnavailable
  - lastHeartbeatTime: "2022-12-11T14:56:39Z"
    lastTransitionTime: "2022-12-11T14:55:49Z"
    message: kubelet has sufficient memory available
    reason: KubeletHasSufficientMemory
    status: "False"
    type: MemoryPressure
  - lastHeartbeatTime: "2022-12-11T14:56:39Z"
    lastTransitionTime: "2022-12-11T14:55:49Z"
    message: kubelet has no disk pressure
    reason: KubeletHasNoDiskPressure
    status: "False"
    type: DiskPressure
  - lastHeartbeatTime: "2022-12-11T14:56:39Z"
    lastTransitionTime: "2022-12-11T14:55:49Z"
    message: kubelet has sufficient PID available
    reason: KubeletHasSufficientPID
    status: "False"
    type: PIDPressure
  - lastHeartbeatTime: "2022-12-11T14:56:39Z"
    lastTransitionTime: "2022-12-11T14:55:49Z"
    message: kubelet is posting ready status
    reason: KubeletReady
    status: "True"
    type: Ready
```

```
k describe no 10.0.240.5 | less

Conditions:
  Type                         Status  LastHeartbeatTime                 LastTransitionTime                Reason                       Message
  ----                         ------  -----------------                 ------------------                ------                       -------
  NetworkUnavailable           False   Sun, 03 Oct 2021 15:37:38 +0800   Sun, 03 Oct 2021 15:37:38 +0800   RouteCreated                 RouteController created a route
  MemoryPressure               False   Sun, 11 Dec 2022 22:56:39 +0800   Sun, 11 Dec 2022 22:55:49 +0800   KubeletHasSufficientMemory   kubelet has sufficient memory available
  DiskPressure                 False   Sun, 11 Dec 2022 22:56:39 +0800   Sun, 11 Dec 2022 22:55:49 +0800   KubeletHasNoDiskPressure     kubelet has no disk pressure
  PIDPressure                  False   Sun, 11 Dec 2022 22:56:39 +0800   Sun, 11 Dec 2022 22:55:49 +0800   KubeletHasSufficientPID      kubelet has sufficient PID available
  Ready                        True    Sun, 11 Dec 2022 22:56:39 +0800   Sun, 11 Dec 2022 22:55:49 +0800   KubeletReady                 kubelet is posting ready status
```

Terminating 的 pod 被删除，新创建 pod 满足期望的副本数：
```
k get po -owide
                            
NAME                                        READY   STATUS    RESTARTS   AGE     IP           NODE           NOMINATED NODE   READINESS GATES
job-demo-wbtg5                              1/1     Running   0          24m     10.4.0.146   10.0.240.5     <none>           <none>
mysql-0                                     2/2     Running   0          4m23s   10.4.0.150   10.0.240.5     <none>           <none>
mysql-1                                     2/2     Running   0          126m    10.4.0.87    10.0.240.132   <none>           <none>
mysql-2                                     2/2     Running   0          125m    10.4.0.88    10.0.240.132   <none>           <none>
nginx-deployment-5bf87f5f59-6k8z6           1/1     Running   0          110m    10.4.0.93    10.0.240.132   <none>           <none>
nginx-deployment-5bf87f5f59-6sk2m           1/1     Running   0          165m    10.4.0.85    10.0.240.132   <none>           <none>
```

可以通过设置 pod toleration 时间，来控制可以容忍被驱逐的时间：
```
设置超过 60 及被驱逐，变为 Terminating 状态：

tolerations:
  - effect: NoExecute
    key: node.kubernetes.io/not-ready
    operator: Exists
    tolerationSeconds: 60
  - effect: NoExecute
    key: node.kubernetes.io/unreachable
    operator: Exists
    tolerationSeconds: 60
```

pod 将在 60s 后被驱逐为 Terminating：
> 其他没有设置 toleration 的 pods 在 60s 后还是 Running。

```
k get po -owide
NAME                                        READY   STATUS        RESTARTS   AGE     IP           NODE           NOMINATED NODE   READINESS GATES
job-demo-fkxl8                              1/1     Running       0          48m     10.4.0.131   10.0.240.5     <none>           <none>
mysql-0                                     2/2     Running       0          56m     10.4.0.158   10.0.240.5     <none>           <none>
mysql-1                                     2/2     Running       0          16h     10.4.0.79    10.0.240.132   <none>           <none>
mysql-2                                     2/2     Running       0          56m     10.4.0.130   10.0.240.5     <none>           <none>
nginx-deployment-8599548c8f-v95gr           1/1     Terminating   0          2m59s   10.4.0.132   10.0.240.5     <none>           <none>
nginx-deployment-8599548c8f-wnts7           1/1     Running       0          2m55s   10.4.0.80    10.0.240.132   <none>           <none>
```

## 4. Node 重启 (~10s)
```
k get po -owide
                        
NAME                                        READY   STATUS    RESTARTS   AGE     IP           NODE           NOMINATED NODE   READINESS GATES
job-demo-rc9rt                              1/1     Running   0          76s     10.4.0.157   10.0.240.5     <none>           <none>
job-demo-wbtg5                              0/1     Error     0          44m     <none>       10.0.240.5     <none>           <none>
mysql-0                                     2/2     Running   2          24m     10.4.0.130   10.0.240.5     <none>           <none>
mysql-1                                     2/2     Running   0          146m    10.4.0.87    10.0.240.132   <none>           <none>
mysql-2                                     2/2     Running   0          145m    10.4.0.88    10.0.240.132   <none>           <none>
nginx-deployment-5bf87f5f59-7xc8t           1/1     Running   0          5m55s   10.4.0.77    10.0.240.132   <none>           <none>
nginx-deployment-5bf87f5f59-fsp6j           1/1     Running   1          5m55s   10.4.0.153   10.0.240.5     <none>           <none>
```

Watch 查看 pod 变化：
```
k get po -owide -w

NAME                                        READY   STATUS    RESTARTS   AGE    IP           NODE           NOMINATED NODE   READINESS GATES
job-demo-wbtg5                              1/1     Running   0          42m    10.4.0.146   10.0.240.5     <none>           <none>
nginx-deployment-5bf87f5f59-fsp6j           1/1     Running   0          4m7s   10.4.0.152   10.0.240.5     <none>           <none>
mysql-0                                     2/2     Running   0          22m    10.4.0.150   10.0.240.5     <none>           <none>
nginx-deployment-5bf87f5f59-fsp6j           1/1     Running   0          4m37s   10.4.0.152   10.0.240.5     <none>           <none>
job-demo-wbtg5                              0/1     Error     0          42m     <none>       10.0.240.5     <none>           <none>
job-demo-rc9rt                              0/1     Pending   0          0s      <none>       <none>         <none>           <none>
job-demo-rc9rt                              0/1     Pending   0          0s      <none>       10.0.240.5     <none>           <none>
job-demo-rc9rt                              0/1     Pending   0          1s      <none>       10.0.240.5     <none>           <none>
job-demo-rc9rt                              0/1     ContainerCreating   0          8s      <none>       10.0.240.5     <none>           <none>
mysql-0                                     2/2     Running             0          23m     10.4.0.150   10.0.240.5     <none>           <none>
mysql-0                                     0/2     Error               0          23m     <none>       10.0.240.5     <none>           <none>
nginx-deployment-5bf87f5f59-fsp6j           1/1     Running             1          4m52s   10.4.0.153   10.0.240.5     <none>           <none>
job-demo-rc9rt                              1/1     Running             0          18s     10.4.0.157   10.0.240.5     <none>           <none>
mysql-0                                     2/2     Running             2          23m     10.4.0.130   10.0.240.5     <none>           <none>
```

exec/logs 提示超时了，与 kubelet 进程退出后 connection refused 不同，因为此时 kube-proxy 已经挂了，无法进行网络转发：
```
k exec -it nginx-deployment-5bf87f5f59-fsp6j -- bash

Error from server: error dialing backend: dial tcp 10.0.240.5:10250: i/o timeout


k logs nginx-deployment-5bf87f5f59-fsp6j

Error from server: Get "https://10.0.240.5:10250/containerLogs/demo/nginx-deployment-5bf87f5f59-fsp6j/nginx": dial tcp 10.0.240.5:10250: i/o timeout
```

## 5. Node 关机
### 5.1 关机时长 <= 5min
Node 状态立即变化：
```
k get no -w

NAME           STATUS   ROLES    AGE    VERSION
10.0.240.125   Ready    <none>   388d   v1.18.4-tke.14
10.0.240.132   Ready    <none>   434d   v1.18.4-tke.14
10.0.240.5     Ready    <none>   434d   v1.18.4-tke.14

=====

10.0.240.125   Ready    <none>   388d   v1.18.4-tke.14
10.0.240.5     NotReady   <none>   434d   v1.18.4-tke.14
10.0.240.5     NotReady   <none>   434d   v1.18.4-tke.14
10.0.240.5     NotReady   <none>   434d   v1.18.4-tke.14
10.0.240.5     NotReady   <none>   434d   v1.18.4-tke.14
```

pod 表现与上面的 kubelet 进程退出 <= 5min 基本一致。

### 5.2 关机时长 > 5min

```
k get po nginx-deployment-5bf87f5f59-fsp6j -oyaml | less

status:
  conditions:
  - lastProbeTime: null
    lastTransitionTime: "2022-12-11T15:14:45Z"
    status: "True"
    type: Initialized
  - lastProbeTime: null
    lastTransitionTime: "2022-12-11T15:34:02Z"
    status: "False"
    type: Ready
  - lastProbeTime: null
    lastTransitionTime: "2022-12-11T15:19:23Z"
    status: "True"
    type: ContainersReady
  - lastProbeTime: null
    lastTransitionTime: "2022-12-11T15:14:45Z"
    status: "True"
    type: PodScheduled
```

5min 后，pod 出现 Terminating，在其他 Node 新创建 pod 满足期望的副本数：
```
k get po -owide
              
NAME                                        READY   STATUS        RESTARTS   AGE    IP           NODE           NOMINATED NODE   READINESS GATES
job-demo-rc9rt                              1/1     Terminating   0          20m    10.4.0.157   10.0.240.5     <none>           <none>
mysql-0                                     2/2     Terminating   2          43m    10.4.0.130   10.0.240.5     <none>           <none>
mysql-1                                     2/2     Running       0          165m   10.4.0.87    10.0.240.132   <none>           <none>
mysql-2                                     2/2     Running       0          164m   10.4.0.88    10.0.240.132   <none>           <none>
nginx-deployment-5bf87f5f59-7xc8t           1/1     Running       0          24m    10.4.0.77    10.0.240.132   <none>           <none>
nginx-deployment-5bf87f5f59-fsp6j           1/1     Terminating   1          24m    10.4.0.153   10.0.240.5     <none>           <none>
```

exec/logs 出现超时：
```
k exec -it mysql-0 -- bash

Defaulted container "mysql" out of: mysql, xtrabackup, init-mysql (init), clone-mysql (init)
Error from server: error dialing backend: dial tcp 10.0.240.5:10250: i/o timeout


k logs mysql-0 -c mysql

Error from server: Get "https://10.0.240.5:10250/containerLogs/demo/mysql-0/mysql": dial tcp 10.0.240.5:10250: i/o timeout
```

当 Node 超过 5min 后开机，节点上处于 Terminating 的 pod 自动被 GC 删除。DS restart 重启次数会 +1：
```
k get po -n kube-system -owide | grep router

kube-router-4qqsj                      1/1     Running   0          123d   10.0.240.132   10.0.240.132   <none>           <none>
kube-router-lws2j                      1/1     Running   3          123d   10.0.240.5     10.0.240.5     <none>           <none>
kube-router-m7ltf                      1/1     Running   0          123d   10.0.240.125   10.0.240.125   <none>           <none>
```

篇幅有限，读者感兴趣，请自行验证以上各种异常 case、各种 workload(deploy/sts/ds/job 等) 对应的行为。

> 操作可能会花费一定时间，但只有通过动手实践过，才能对本文要讲述的核心要点有更深刻的理解，并加深记忆。

## 6. 小结
本文通过实操 kubelet 进程退出、Node 重启、Node 关机等几种场景，观察对应 Node、pod 的行为，探索 Node 异常后 pod 的处理机制。小结如下：

- 当 Node 上 kubelet 进程退出 <= 5min，Node 立即变为 NotReady，pod 都为正常，不会有 restart；
- 当 Node 上 kubelet 进程退出 > 5min，pod 变为 Terminating，新创建 pod 满足期望的副本数；
- 当 Node 重启 (~10s)、关机时长 <= 5min，表现与上面第一点 kubelet 进程退出 <= 5min 基本一致；
- 当 Node 关机时长 > 5min，表现与上面第二点 kubelet 进程退出 > 5min 基本一致；
- 上述四种情况下，kubectl exec/logs 都会异常提示 connection refused 或 i/o timeout；
- K8s 提供相关参数，让用户根据自身需要灵活设置 Kubelet 状态更新频率；


*PS: 更多内容请关注 [k8s-club](https://github.com/k8s-club/k8s-club)*


### 参考资料
1. [K8s Node 官方文档](https://kubernetes.io/docs/concepts/architecture/nodes/)
2. [K8s NodeLifecycle 控制器源码](https://github.com/kubernetes/kubernetes/blob/master/pkg/controller/nodelifecycle/node_lifecycle_controller.go)
3. [Kubelet 参数 API](https://kubernetes.io/docs/reference/config-api/kubelet-config.v1beta1/)
4. [Kubelet 状态更新机制](https://www.qikqiak.com/post/kubelet-sync-node-status/)