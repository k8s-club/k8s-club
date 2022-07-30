TOC
- [1. 概述](#1-概述)
- [2. Docker ENTRYPOINT vs CMD](#2-Docker-ENTRYPOINT-vs-CMD)
    - [2.1 Only ENTRYPOINT](#21-Only-ENTRYPOINT)
    - [2.2 Only CMD](#22-Only-CMD)
    - [2.3 Both ENTRYPOINT + CMD](#23-Both-ENTRYPOINT--CMD)
    - [2.4 Both empty](#24-Both-empty)
- [3. K8s command vs args](#3-K8s-command-vs-args)
    - [3.1 Only command](#31-Only-command)
    - [3.2 Only args](#32-Only-args)
    - [3.3 Both command + args](#33-Both-command--args)
    - [3.4 Both empty](#34-Both-empty)
- [4. K8s with Docker](#4-K8s-with-Docker)
- [5. 小结](#5-小结)


## 1. 概述
Docker 中通过 Dockerfile 对镜像进行分层（Layer）构建，使用 ENTRYPOINT、CMD 来控制容器初始化后的执行入口点。而在 Kubernetes (K8s) pod 中，则使用 command、args 来控制 pod 中 容器的执行入口点。

当我们使用 Docker 为运行时接口（CRI），在 K8s 中两者参数是怎样控制来决定最终的容器行为呢？

本文通过对 Docker 中 ENTRYPOINT、CMD 和 K8s 中 command、args 进行对比分析，并使用对应的 Demo 说明了两者的区别和联系。

## 2. Docker ENTRYPOINT vs CMD

### 2.1 Only ENTRYPOINT
Dockerfile
```
FROM busybox

ENTRYPOINT ["printf", "This is entrypoint from %s.\n"]
```

运行结果：
```
docker build -t busybox-demo . && docker run busybox-demo
```
> This is entrypoint from .

可以看到，ENTRYPOINT 未解析到任何参数，所以占位符 %s 输出为空。

**【动态参数】** 如果在运行时传递了 CLI 参数，则可以解析到：
```
docker build -t busybox-demo . && docker run busybox-demo cli-args
```

运行结果：
> This is entrypoint from cli-args.

**【替换 ENTRYPOINT】** 如果在运行时传递了 CLI 参数 --entrypoint，则会覆盖默认 ENTRYPOINT：
```
docker build -t busybox-demo . && docker run --entrypoint echo busybox-demo new-entrypoint
```

运行结果：
> new-entrypoint

### 2.2 Only CMD
Dockerfile
```
FROM busybox

CMD ["dockerfile cmd"]
```

运行结果：
> docker: Error response from daemon: failed to create shim: OCI runtime create failed: container_linux.go:380: starting container process caused: exec: "dockerfile cmd": executable file not found in $PATH: unknown.

可以看到，当仅仅只有 CMD 而没有 ENTRYPOINT 的时候，CMD 第一个参数为在 $PATH 路径可找到的内置命令或可执行文件。
此时找不到 dockerfile 的可执行文件，所以看到输出为对应的 OCI 报错。

只需要稍微改一下，则可以运行：
Dockerfile
```
FROM busybox

CMD ["echo", "dockerfile cmd"]
```

运行结果：
> dockerfile cmd

### 2.3 Both ENTRYPOINT + CMD
Dockerfile
```
FROM busybox

ENTRYPOINT ["printf", "This is entrypoint from %s.\n"]

CMD ["dockerfile cmd"]
```

运行结果：
> This is entrypoint from dockerfile cmd.

可以看到，当 ENTRYPOINT + CMD 都存在的时候，则 CMD 作为 ENTRYPOINT 的参数。

当 CMD 参数列表有多个，即使第一个参数为可执行文件，也只是作为字符串参数传递给 ENTRYPOINT，如下所示：

Dockerfile
```
FROM busybox

ENTRYPOINT ["printf", "This is entrypoint from %s.\n"]

CMD ["echo", "dockerfile cmd"]
```

运行结果：
> This is entrypoint from echo.
> 
> This is entrypoint from dockerfile cmd.

> Note：当有多个参数时，printf 会换行分别输出格式化字符串。

**【替换 CMD】** 如果在运行时传递了 CLI 参数，则会覆盖 CMD 参数列表：
```
docker build -t busybox-demo . && docker run busybox-demo cli-args
```

运行结果：
> This is entrypoint from cli-args.

### 2.4 Both empty
Dockerfile
```
FROM busybox
```

运行结果为空，表示只是运行启动了 container，但没有执行任何命令就退出了。

## 3. K8s command vs args

在 K8s pod 中，以下面的包含 ENTRYPOINT + CMD 为 container image，进行 command vs args 相关 demo 测试。

> 镜像已推送到 Docker Hub: astraw99/busybox-demo:latest

Dockerfile
```
FROM busybox

ENTRYPOINT ["printf", "This is entrypoint from %s.\n"]

CMD ["dockerfile cmd"]
```

### 3.1 Only command
pod.yaml
```yaml
apiVersion: v1
kind: Pod
metadata:
  name: cmd-demo
spec:
  containers:
  - name: demo-container
    image: astraw99/busybox-demo
    command: ["echo", "cmd"]
```

运行 pod：
```
kubectl apply -f ./pod.yaml
kubectl logs cmd-demo
```

运行结果：
> cmd

可以看到，当 pod 中只有 command 时，会忽略镜像 Dockerfile 中默认的 ENTRYPOINT + CMD，而仅仅执行提供的 command。

### 3.2 Only args
pod.yaml
```yaml
apiVersion: v1
kind: Pod
metadata:
  name: cmd-demo
spec:
  containers:
  - name: demo-container
    image: astraw99/busybox-demo
    args: ["pod arg1", "pod arg2"]
```

运行结果：
> This is entrypoint from pod arg1.
> 
> This is entrypoint from pod arg2.

可以看到，当 pod 中只有 args 时，会使用镜像 Dockerfile 中默认的 ENTRYPOINT，但会忽略其默认的 CMD，然后将 args 传递给 ENTRYPOINT 解析。

### 3.3 Both command + args
pod.yaml
```yaml
apiVersion: v1
kind: Pod
metadata:
  name: cmd-demo
spec:
  containers:
  - name: demo-container
    image: astraw99/busybox-demo
    command: ["echo", "cmd"]
    args: ["pod arg1", "pod arg2"]
```

运行结果：
> cmd pod arg1 pod arg2

可以看到，当 pod 中有 command + args 时，会忽略镜像 Dockerfile 中默认的 ENTRYPOINT + CMD，而仅仅执行提供的 command + args。

### 3.4 Both empty
pod.yaml
```yaml
apiVersion: v1
kind: Pod
metadata:
  name: cmd-demo
spec:
  restartPolicy: OnFailure
  containers:
  - name: demo-container
    image: astraw99/busybox-demo
```

运行结果：
> This is entrypoint from dockerfile cmd.

可以看到，当 pod 中没有 command + args 时，会使用镜像 Dockerfile 中默认的 ENTRYPOINT + CMD，进行执行输出。

## 4. K8s with Docker
根据上文所述，Docker、K8s 分别有两个字段相对应，如下：

|              Description               | Docker field name | Kubernetes field name |
|----------------------------------------|-------------------|-----------------------|
|  The command run by the container      | ENTRYPOINT        |      command          |
|  The arguments passed to the command   | CMD               |      args             |

考虑 Docker、K8s 各 4 种组合，则一共有 4 * 4 = 16 种两者的组合方式。汇总如下：

```
#1# K8s command with Docker ENTRYPOINT
#2# K8s command with Docker CMD
#3# K8s command with Docker ENTRYPOINT + CMD
#4# K8s command with Docker empty

#5# K8s args with Docker ENTRYPOINT
#6# K8s args with Docker CMD
#7# K8s args with Docker ENTRYPOINT + CMD
#8# K8s args with Docker empty

#09# K8s command + args with Docker ENTRYPOINT
#10# K8s command + args with Docker CMD
#11# K8s command + args with Docker ENTRYPOINT + CMD
#12# K8s command + args with Docker empty

#13# K8s empty with Docker ENTRYPOINT
#14# K8s empty with Docker CMD
#15# K8s empty with Docker ENTRYPOINT + CMD
#16# K8s empty with Docker empty
```

篇幅有限，读者感兴趣，请自行验证以上组合方式。

> 操作可能会花费一定时间，但只有通过动手实践过，才能对本文要讲述的核心要点有更深刻的理解，并加深记忆。

## 5. 小结
本文通过对 Docker 中 ENTRYPOINT、CMD 和 K8s 中 command、args 进行对比分析，并使用对应的 Demo 说明了两者的区别和联系。小结如下：

- 当 K8s pod 中没有 command + args，则使用镜像 Dockerfile 中默认的 ENTRYPOINT + CMD；
- 当 K8s pod 中只有 command 时，会忽略镜像 Dockerfile 中默认的 ENTRYPOINT + CMD，而仅仅执行提供的 command；
- 当 K8s pod 中只有 args 时，会使用镜像 Dockerfile 中默认的 ENTRYPOINT，但会忽略其默认的 CMD，然后将 args 传递给 ENTRYPOINT；
- 当 K8s pod 中有 command + args 时，会忽略镜像 Dockerfile 中默认的 ENTRYPOINT + CMD，而执行提供的 command + args；

| Image Entrypoint   |    Image Cmd     | Container command | Container args | Command run      |
|--------------------|------------------|-------------------|----------------|------------------|
|     `[/ep-1]`      |   `[foo bar]`    | `<not set>`       | `<not set>`    | `[ep-1 foo bar]` |
|     `[/ep-1]`      |   `[foo bar]`    | `[/ep-2]`         | `<not set>`    | `[ep-2]`         |
|     `[/ep-1]`      |   `[foo bar]`    | `<not set>`       | `[zoo boo]`    | `[ep-1 zoo boo]` |
|     `[/ep-1]`      |   `[foo bar]`    | `[/ep-2]`         | `[zoo boo]`    | `[ep-2 zoo boo]` |


*PS: 更多内容请关注 [k8s-club](https://github.com/k8s-club/k8s-club)*


### 参考资料
1. [Dockerfile reference 官方文档](https://docs.docker.com/engine/reference/builder/)
2. [Kubernetes command args 文档](https://kubernetes.io/docs/tasks/inject-data-application/define-command-argument-container/)
3. [理解 pod command args](https://sahansera.dev/closer-look-at-kubernetes-pod-commands-args/)
4. [比较 Docker 与 K8s 字段](https://github.com/kubernetes/website/pull/31058)