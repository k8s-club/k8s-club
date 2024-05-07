## 关于 Pod 的原地升级
目录：
- [1. 概述](#t1)
- [2. 一般操作](#t2)
- [3. 原地升级操作](#t3)
- [4. 核心代码](#t4)  
- [5. 参考资料](#t5)


### 1. <a name='t1'></a>概述：
背景解释：把应用的旧版本替换成新版本，但是不再次执行调度，而在"原来节点上"直接操作的过程。此作法能够使应用在升级过程中避免将整个 Pod 删除、新建，而是基于原有的 Pod 升级其中某一个或多个容器的镜像版本。

原地升级的优势：
1. 可以节省调度的消耗，不需要再次调度pod。
2. pod Ip 不会再次分配；
3. 节省了分配、挂载远程盘的耗时，Pod 还使用原有的 PV（且都是已经在 Node 上挂载好的）；



### 2. <a name='t2'></a>一般操作：
#### 一般升级
(重点比较一下 pod 与 deployment 的区别)：
- Pod

使用kubectl如下操作，可以看到pod被调度到vm-0-17-centos的节点上去。
```bash
[root@vm-0-12-centos try_yaml]# kubectl apply -f example-pod.yaml
pod/example-pod created
[root@vm-0-12-centos try_yaml]# kubectl get pods example-pod -owide
NAME          READY   STATUS    RESTARTS   AGE    IP               NODE             NOMINATED NODE   READINESS GATES
example-pod   1/1     Running   0          2m3s   10.244.167.235   vm-0-17-centos   <none>           <none>
```
当修改pod的image镜像时(如：镜像修改成nginx:1.19-alpine)，会发现kube-scheduler不会重新调度，内部IP也不会重新分配，并且RESTARTS字段会增加1，代表pod在apply时发生原地重启。
```bash
[root@vm-0-12-centos try_yaml]# vim example-pod.yaml
[root@vm-0-12-centos try_yaml]# kubectl apply -f example-pod.yaml
pod/example-pod configured
[root@vm-0-12-centos try_yaml]# kubectl get pods example-pod -owide
NAME          READY   STATUS    RESTARTS   AGE     IP               NODE             NOMINATED NODE   READINESS GATES
example-pod   1/1     Running   1          5m48s   10.244.167.235   vm-0-17-centos   <none>           <none>
```
- Deployment

使用kubectl如下操作，可以看到在vm-0-17-centos与vm-0-13-centos上k8s分别创建出pod。
```bash
[root@vm-0-12-centos try_yaml]# kubectl apply -f example-deployment.yaml
deployment.apps/example-deployment created
[root@vm-0-12-centos try_yaml]# kubectl get pods -owide | grep example-deployment
example-deployment-5b77458df9-ft784   1/1     Running   0          4s     10.244.167.234   vm-0-17-centos   <none>           <none>
example-deployment-5b77458df9-g8h5p   1/1     Running   0          4s     10.244.182.178   vm-0-13-centos   <none>           <none>
```
同样修改deployment的镜像时，会发现与修改pod时的不同，首先RESTARTS字段没有增加，其次IP地址也经过cni重新分配，代表deployment更新时不是原地重启，而是经过k8s-scheduler重新调度过的结果。
```bash
[root@vm-0-12-centos try_yaml]# vim example-deployment.yaml
[root@vm-0-12-centos try_yaml]# kubectl apply -f example-deployment.yaml
deployment.apps/example-deployment configured
[root@vm-0-12-centos try_yaml]# kubectl get pods -owide | grep example-deployment
example-deployment-658789c5cd-pk99c   1/1     Running   0          12s    10.244.182.164   vm-0-13-centos   <none>           <none>
example-deployment-658789c5cd-qkn8d   1/1     Running   0          10s    10.244.167.236   vm-0-17-centos   <none>           <none>
```

### 3. <a name='t3'></a>原地升级操作：

#### 原地升级

- Pod

使用kubectl提供的patch操作，也能达到与apply一样的效果。

```bash
[root@vm-0-12-centos ~]# kubectl patch pod example-pod --type='json' --patch='[{"op": "replace", "path": "/spec/containers/0/image", "value": "nginx:1.19-alpine"}]'
pod/example-pod patched
[root@vm-0-12-centos ~]# kubectl get pods -owide | grep example-pod
example-pod                           1/1     Running   3          39m    10.244.167.235   vm-0-17-centos   <none>           <none>
```

- Deployment

同上，对deployment管理的pod进行操作，也能达到相同效果。

```bash
[root@vm-0-12-centos ~]# kubectl patch pod example-deployment-658789c5cd-pk99c --type='json' --patch='[{"op": "replace", "path": "/spec/containers/0/image", "value": "nginx:1.19-alpine"}]'
pod/example-deployment-658789c5cd-pk99c patched
[root@vm-0-12-centos ~]# kubectl get pods -owide | grep example-deployment
example-deployment-658789c5cd-pk99c   1/1     Running   1          19m    10.244.182.164   vm-0-13-centos   <none>           <none>
example-deployment-658789c5cd-qkn8d   1/1     Running   0          19m    10.244.167.236   vm-0-17-centos   <none>           <none>

[root@vm-0-12-centos ~]# kubectl patch pod example-deployment-658789c5cd-qkn8d --type='json' --patch='[{"op": "replace", "path": "/spec/containers/0/image", "value": "nginx:1.19-alpine"}]'
pod/example-deployment-658789c5cd-qkn8d patched
[root@vm-0-12-centos ~]# kubectl get pods -owide | grep example-deployment
example-deployment-658789c5cd-pk99c   1/1     Running   1          20m    10.244.182.164   vm-0-13-centos   <none>           <none>
example-deployment-658789c5cd-qkn8d   1/1     Running   1          20m    10.244.167.236   vm-0-17-centos   <none>           <none>
```

附注：使用kubectl命令如果不搭配bash脚本，需要手敲非常多次，降低工作效率，以下准备了使用clientgo调用的方式进行原地升级的简易代码。


[原地升级代码示例](../demo/examples/restart)

### 4.  <a name='t4'></a>核心代码：
核心思路：由deployment中找到对应的pod，再对pod进行patch操作更新镜像。
```go

func main() {
    // 需要更新的副本数
    replicasNum := 2
    // dep name, namespace
    depName := "example-deployment"
    ns := "default"
    pods := pkg.GetPodsByDeployment(depName, ns)
    
    for i := 0; i < replicasNum; i++ {
        // pod原地升级
        pkg.UpgradePodByImage(&pods[i], "nginx:1.19-alpine")
    }

}

// GetPodsByDeployment 根据传入Deployment获取当前"正在"使用的pod
func GetPodsByDeployment(depName, ns string) []v1.Pod {

    clientSet := client.ClientSet.Client
    deployment, err := clientSet.AppsV1().Deployments(ns).Get(context.TODO(),
    depName, metav1.GetOptions{})
    if err != nil {
        klog.Error("create clientSet error: ", err)
    return nil
    }
    rsIdList := getRsIDsByDeployment(deployment, clientSet)
    podsList := make([]v1.Pod, 0)
    for _, rs := range rsIdList {
        pods := getPodsByReplicaSet(rs, clientSet, ns)
        podsList = append(podsList, pods...)
    }
    
    return podsList
}

// UpgradePodImage 原地升级pod镜像
func UpgradePodByImage(pod *v1.Pod, image string) {

    clientSet := client.ClientSet.Client
    patch := fmt.Sprintf(`[{"op": "replace", "path": "/spec/containers/0/image", "value": "%s"}]`, image)
    patchBytes := []byte(patch)
    
    jsonPatch, err := jsonpatch.DecodePatch(patchBytes)
    if err != nil {
        klog.Error("DecodePatch error: ", err)
        return
    }
    jsonPatchBytes, err := json.Marshal(jsonPatch)
    if err != nil {
        klog.Error("json Marshal error: ", err)
        return
    }
    _, err = clientSet.CoreV1().Pods(pod.Namespace).
    Patch(context.TODO(), pod.Name, types.JSONPatchType,
    jsonPatchBytes, metav1.PatchOptions{})
    if err != nil {
        log.Fatalln(err)
    }
}

```


另外，也有一个简易版的原地升级控制器的demo可以参考[代码仓库](https://github.com/googs1025/podReStarter-operator)

### 5. <a name='t5'></a>参考资料：

[OpenKruise原地升级](https://developer.aliyun.com/article/765421)

[原地升级](https://jimmysong.io/kubernetes-handbook/practice/in-place-update.html)

