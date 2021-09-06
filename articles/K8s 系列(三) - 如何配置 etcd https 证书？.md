
在 K8s 中，`kube-apiserver` 使用 etcd 对 `REST object` 资源进行持久化存储，本文介绍如何配置生成自签 https 证书，搭建 etcd 集群给 apiserver 使用，并附相关坑点记录。

## 1. 安装 cfssl 工具

```
cd /data/work

wget https://github.com/cloudflare/cfssl/releases/download/v1.6.0/cfssl_1.6.0_linux_amd64 -O cfssl
wget https://github.com/cloudflare/cfssl/releases/download/v1.6.0/cfssljson_1.6.0_linux_amd64 -O cfssljson
wget https://github.com/cloudflare/cfssl/releases/download/v1.6.0/cfssl-certinfo_1.6.0_linux_amd64 -O cfssl-certinfo

chmod +x cfssl*
mv cfssl* /usr/local/bin/


chmod +x cfssl*
mv cfssl_linux-amd64 /usr/local/bin/cfssl
mv cfssljson_linux-amd64 /usr/local/bin/cfssljson
mv cfssl-certinfo_linux-amd64 /usr/local/bin/cfssl-certinfo
```

## 2. 创建 ca 证书

```
cat > ca-csr.json <<EOF
{
  "CN": "etcd-ca",
  "key": {
      "algo": "rsa",
      "size": 2048
  },
  "names": [
    {
      "C": "CN",
      "ST": "Beijing",
      "L": "Beijing",
      "O": "etcd-ca",
      "OU": "etcd-ca"
    }
  ],
  "ca": {
          "expiry": "87600h"
  }
}
EOF


cfssl gencert -initca ca-csr.json | cfssljson -bare ca

=> 会生成：ca-key.pem, ca.csr, ca.pem
```

## 3. 配置 ca 证书策略

```
cat > ca-config.json <<EOF
{
  "signing": {
      "default": {
          "expiry": "87600h"
        },
      "profiles": {
          "etcd-ca": {
              "usages": [
                  "signing",
                  "key encipherment",
                  "server auth",
                  "client auth"
              ],
              "expiry": "87600h"
          }
      }
  }
}
EOF
```

## 4. 配置 etcd 请求 csr

```
cat > etcd-csr.json <<EOF
{
  "CN": "etcd",
  "hosts": [
    "127.0.0.1",
    "etcd0-0.etcd",
    "etcd1-0.etcd",
    "etcd2-0.etcd"
  ],
  "key": {
    "algo": "rsa",
    "size": 2048
  },
  "names": [{
    "C": "CN",
    "ST": "Beijing",
    "L": "Beijing",
    "O": "etcd",
    "OU": "etcd"
  }]
}
EOF
```

## 5. 生成 etcd 证书

```
cfssl gencert -ca=ca.pem -ca-key=ca-key.pem -config=ca-config.json -profile=etcd-ca etcd-csr.json | cfssljson  -bare etcd

=> 会生成：etcd-key.pem, etcd.csr, etcd.pem


mv etcd.pem etcd-server.pem
mv etcd-key.pem etcd-server-key.pem
```

## 6. 创建 etcd cluster

yaml 文件：[https://github.com/k8s-club/etcd-operator](https://github.com/k8s-club/etcd-operator)


```
kubectl apply -f etcd-cluster.yaml
```

## 7. 查看 DNS 解析

dnsutils 安装：[https://kubernetes.io/docs/tasks/administer-cluster/dns-debugging-resolution/](https://kubernetes.io/docs/tasks/administer-cluster/dns-debugging-resolution/)

```
kubectl exec -it -n etcd dnsutils -- nslookup etcd

Server:         9.165.x.x
Address:        9.165.x.x#53

Name:   etcd.etcd.svc.cluster.local
Address: 9.165.x.x
Name:   etcd.etcd.svc.cluster.local
Address: 9.165.x.x
Name:   etcd.etcd.svc.cluster.local
Address: 9.165.x.x
```

## 8. 查看 etcd 集群状态
```
kubectl exec -it -n etcd etcd0-0 -- sh

/usr/local/bin/etcdctl --write-out=table --cacert=/etc/etcd/ssl/ca.pem --cert=/etc/etcd/ssl/etcd-server.pem --key=/etc/etcd/ssl/etcd-server-key.pem --endpoints=https://etcd0-0.etcd:2379,https://etcd1-0.etcd:2379,https://etcd2-0.etcd:2379 endpoint health

+---------------------------+--------+-------------+-------+
|         ENDPOINT          | HEALTH |    TOOK     | ERROR |
+---------------------------+--------+-------------+-------+
| https://etcd0-0.etcd:2379 |   true | 13.551982ms |       |
| https://etcd1-0.etcd:2379 |   true | 13.540498ms |       |
| https://etcd2-0.etcd:2379 |   true | 23.119639ms |       |
+---------------------------+--------+-------------+-------+

/usr/local/bin/etcdctl --write-out=table --cacert=/etc/etcd/ssl/ca.pem --cert=/etc/etcd/ssl/etcd-server.pem --key=/etc/etcd/ssl/etcd-server-key.pem --endpoints=https://etcd0-0.etcd:2379,https://etcd1-0.etcd:2379,https://etcd2-0.etcd:2379 endpoint status

+--------------------------+------------------+---------+---------+-----------+------------+-----------+------------+--------------------+--------+
|         ENDPOINT         |        ID        | VERSION | DB SIZE | IS LEADER | IS LEARNER | RAFT TERM | RAFT INDEX | RAFT APPLIED INDEX | ERRORS |
+--------------------------+------------------+---------+---------+-----------+------------+-----------+------------+--------------------+--------+
| http://etcd0-0.etcd:2379 | 4dde210279eea33a |  3.4.13 |   20 kB |      true |      false |         2 |          9 |                  9 |        |
| http://etcd1-0.etcd:2379 | 20669865d12a473b |  3.4.13 |   20 kB |     false |      false |         2 |          9 |                  9 |        |
| http://etcd2-0.etcd:2379 | 3f17922d1ed63113 |  3.4.13 |   20 kB |     false |      false |         2 |          9 |                  9 |        |
+--------------------------+------------------+---------+---------+-----------+------------+-----------+------------+--------------------+--------+

```

## 9. 验证 etcd 读写
```
kubectl exec -it -n etcd etcd0-0 -- sh

/usr/local/bin/etcdctl --cacert=/etc/etcd/ssl/ca.pem --cert=/etc/etcd/ssl/etcd-server.pem --key=/etc/etcd/ssl/etcd-server-key.pem put hello world
OK

/usr/local/bin/etcdctl --cacert=/etc/etcd/ssl/ca.pem --cert=/etc/etcd/ssl/etcd-server.pem --key=/etc/etcd/ssl/etcd-server-key.pem get hello
hello
world

查看所有 keys:
/usr/local/bin/etcdctl --cacert=/etc/etcd/ssl/ca.pem --cert=/etc/etcd/ssl/etcd-server.pem --key=/etc/etcd/ssl/etcd-server-key.pem get "" --keys-only --prefix
hello

查看所有 key-val:
/usr/local/bin/etcdctl --cacert=/etc/etcd/ssl/ca.pem --cert=/etc/etcd/ssl/etcd-server.pem --key=/etc/etcd/ssl/etcd-server-key.pem get "" --prefix
hello
world

```

## 10. 配置 apiserver 请求 csr

```
cat > apiserver-csr.json <<EOF
{
  "CN": "apiserver",
  "hosts": [
    "*.etcd"
  ],
  "key": {
    "algo": "rsa",
    "size": 2048
  },
  "names": [{
    "C": "CN",
    "ST": "Beijing",
    "L": "Beijing",
    "O": "apiserver",
    "OU": "apiserver"
  }]
}
EOF
```

## 11. 生成 apiserver 证书

```
cfssl gencert -ca=ca.pem -ca-key=ca-key.pem -config=ca-config.json -profile=etcd-ca apiserver-csr.json | cfssljson  -bare apiserver

=> 会生成：apiserver-key.pem, apiserver.csr, apiserver.pem


mv apiserver.pem etcd-client-apiserver.pem
mv apiserver-key.pem etcd-client-apiserver-key.pem
```

## 12. 创建 extension-apiserver

apiserver.yaml：通过 `ConfigMap` 将生成的 *.pem 证书挂载给 apiserver 使用


```
...
containers:
  - image: apiserver.xxxxx:latest
    args:
    - --etcd-servers=https://etcd0-0.etcd:2379
    - --etcd-cafile=/etc/kubernetes/certs/ca.pem
    - --etcd-certfile=/etc/kubernetes/certs/etcd-client-apiserver.pem
    - --etcd-keyfile=/etc/kubernetes/certs/etcd-client-apiserver-key.pem
...
```

```
kubectl apply -f apiserver.yaml
```

## 13. 坑点记录

### 13.1 证书 hosts 不对

```
log:
etcd0-0:
{"level":"warn","ts":"2021-08-19T11:55:07.755Z","caller":"embed/config_logging.go:279","msg":"rejected connection","remote-addr":"127.0.0.1:41226","server-name":"","error":"tls: first record does not look like a TLS handshake"}


etcd1-0:
{"level":"info","ts":"2021-08-19T11:54:16.830Z","caller":"embed/serve.go:191","msg":"serving client traffic securely","address":"[::]:2379"}
{"level":"info","ts":"2021-08-19T11:54:16.838Z","caller":"etcdserver/server.go:716","msg":"initialized peer connections; fast-forwarding election ticks","local-member-id":"30dd90df9a304e97","forward-ticks":18,"forward-duration":"4.5s","election-ticks":20,"election-timeout":"5s","active-remote-members":2}
{"level":"info","ts":"2021-08-19T11:54:16.867Z","caller":"membership/cluster.go:558","msg":"set initial cluster version","cluster-id":"80c7f1f6c2848777","local-member-id":"30dd90df9a304e97","cluster-version":"3.4"}
{"level":"info","ts":"2021-08-19T11:54:16.867Z","caller":"api/capability.go:76","msg":"enabled capabilities for version","cluster-version":"3.4"}


etcd2-0:
{"level":"warn","ts":"2021-08-19T11:54:17.782Z","caller":"embed/config_logging.go:270","msg":"rejected connection","remote-addr":"9.165.x.x:40180","server-name":"etcd2-0.etcd","ip-addresses":["0.0.0.0","127.0.0.1"],"dns-names":["etcd0-0.etcd","etcd1-0.etcd","etcd2-0.etcd"],"error":"tls: \"9.165.x.x\" does not match any of DNSNames [\"etcd0-0.etcd\" \"etcd1-0.etcd\" \"etcd2-0.etcd\"] (lookup etcd1-0.etcd on 9.165.x.x:53: no such host)"}

```

解决：重新配置正确的 hosts 域名


### 13.2 证书 hosts 配置坑点

```
"hosts": [
    "127.0.0.1",
    "etcd0-0.etcd",
    "*.etcd" // 允许 * 泛域名，但不能为空 "" 或 *
  ],
```

### 13.3 dns 设置参考

推荐设置 `*.xxx.ns.svc`，这样扩容后也不需要重签证书
> 参考：[https://kubernetes.io/docs/concepts/services-networking/dns-pod-service/](https://kubernetes.io/docs/concepts/services-networking/dns-pod-service/)

Go 代码参考如下：

```go
func genEtcdWildcardDnsName(namespace, serviceName string) []string {
	return []string{
		fmt.Sprintf("%s.%s.%s", serviceName, namespace, "svc"),
		fmt.Sprintf("*.%s.%s.%s", serviceName, namespace, "svc"),
		fmt.Sprintf("%s.%s.%s", serviceName, namespace, DnsBase),
		fmt.Sprintf("*.%s.%s.%s", serviceName, namespace, DnsBase),
	}
}
```

### 13.4 leader/follower 已经建立成功了，但访问报错

```
# /usr/local/bin/etcdctl put hello world
{"level":"warn","ts":"2021-08-19T12:32:11.200Z","caller":"clientv3/retry_interceptor.go:62","msg":"retrying of unary invoker failed","target":"endpoint://client-05ed1825-e70f-492a-af94-03c633d0affc/127.0.0.1:2379","attempt":0,"error":"rpc error: code = DeadlineExceeded desc = latest balancer error: all SubConns are in TransientFailure, latest connection error: connection closed"}
Error: context deadline exceeded
```

解决：etcdctl 需要带证书访问

```
/usr/local/bin/etcdctl --cacert=/etc/etcd/ssl/ca.pem --cert=/etc/etcd/ssl/etcd-server.pem --key=/etc/etcd/ssl/etcd-server-key.pem put hello world
```

### 13.5 http 与 https 之间不能切换

先通过 http 建立了 cluster，然后再用自签证书 https 来建立，这样就会报错：

```
tls: first record does not look like a TLS handshake
```

经过验证：无论是从 http => https，还是从 https => http 的切换都会报这个错，因为一旦建立 cluster 成功，则把连接的协议(http/https) 写入到 etcd 存储里了，不能再更改连接协议。

解决：如果真正遇到需要切换协议，可尝试下面方式
- 允许删除数据：删除后重新建立 cluster
- 不允许删数据：可以尝试采用 [snapshot & restore](https://etcd.io/docs/v3.5/op-guide/recovery/) 进行快照与恢复操作


### 13.6 apiserver 可直接使用第 5 步生成的 etcd 证书吗？

经过验证，是可以直接使用 etcd 证书的，但生产上不建议这样使用。

生产上建议对 apiserver(或其他应用) 单独生成证书，可使用泛域名(*.xx.xx)、不同过期时间等方式灵活配置，也更有利于集群管控。

