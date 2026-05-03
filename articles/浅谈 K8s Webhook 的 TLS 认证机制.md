# 浅谈 K8s Webhook 的 TLS 认证机制

> 一文搞懂 K8s Webhook 类型、caBundle 设计、apiserver TLS 认证流程与 cert-manager 证书管理等机制。
>
> 适用 K8s 版本：1.19+

---

TOC

- [1. K8s Webhook 类型与应用场景](#1-k8s-webhook-类型与应用场景)
- [2. 为什么 Webhook 必须用 TLS](#2-为什么-webhook-必须用-tls)
- [3. 核心概念：caBundle 是什么](#3-核心概念cabundle-是什么)
- [4. apiserver 调用 Webhook 的 TLS 认证流程](#4-apiserver-调用-webhook-的-tls-认证流程)
- [5. TLS 证书的三要素](#5-tls-证书的三要素)
- [6. 证书的签发方式](#6-证书的签发方式)
  - [6.1 方案一：手动自签证书](#61-方案一手动自签证书)
  - [6.2 方案二：cert-manager 自动管理](#62-方案二cert-manager-自动管理)
- [7. 证书的存放与加载](#7-证书的存放与加载)
- [8. 实战：Helm Chart 工程化方案](#8-实战helm-chart-工程化方案)
- [9. 证书轮换与续签策略](#9-证书轮换与续签策略)
- [10. 常见问题排查](#10-常见问题排查)
- [11. 安全最佳实践](#11-安全最佳实践)
- [参考资料](#参考资料)

---

## 1. K8s Webhook 类型与应用场景

在深入 TLS 机制前，先厘清 K8s（Kubernetes）里"Webhook"这个词的含义——它并不是单一机制，而是一组**在 apiserver 处理请求链路上的不同扩展点**。理解它们的差异，能帮助你判断自己的业务场景到底要接入哪一类 Webhook。

### 1.1 三大类 Webhook 一览

```
┌─────────────────────────────────────────────────────────────────────┐
│                    kube-apiserver 请求处理链路                       │
│                                                                     │
│  客户端请求                                                          │
│      │                                                              │
│      ▼                                                              │
│  ┌──────── 认证 ────────┐   ← ① Authentication Webhook             │
│  │  （你是谁？）         │      TokenReview                          │
│  └─────────┬────────────┘                                           │
│            ▼                                                        │
│  ┌──────── 鉴权 ────────┐   ← ② Authorization Webhook              │
│  │ （你能做什么？）       │      SubjectAccessReview                  │
│  └─────────┬────────────┘                                           │
│            ▼                                                        │
│  ┌──── 准入控制 ────┐                                                │
│  │                 │── ③a Mutating Admission Webhook （修改对象）  │
│  │                 │                                                │
│  │                 │── 内置 Object Schema 校验                      │
│  │                 │                                                │
│  │                 │── ③b Validating Admission Webhook （校验对象）│
│  └────────┬────────┘                                                │
│           ▼                                                         │
│        写入 etcd                                                     │
└─────────────────────────────────────────────────────────────────────┘
```

| 类型 | 执行阶段 | 是否可改对象 | 典型用途 | 对应配置资源 |
|------|----------|--------------|----------|--------------|
| **① Authentication Webhook** | 认证阶段 | ❌ 只返回用户身份 | 对接企业 SSO、LDAP、自定义 Token 验证 | kube-apiserver 启动参数（非 CRD） |
| **② Authorization Webhook** | 鉴权阶段 | ❌ 只返回 allow/deny | 自定义 RBAC、接入统一权限平台 | kube-apiserver 启动参数（非 CRD） |
| **③ Admission Webhook**（Mutating + Validating） | 准入控制阶段 | ✅ Mutating 可改 / ❌ Validating 只校验 | Sidecar 注入、安全策略、默认值填充、资源配额 | `MutatingWebhookConfiguration` / `ValidatingWebhookConfiguration` |

> 本文聚焦 **③ Admission Webhook** 的 TLS 体系——它是绝大多数运维/平台团队会自研和维护的类型，也是 `MutatingWebhookConfiguration` / `ValidatingWebhookConfiguration` 里 `caBundle` 发挥作用的地方。① / ② 的 Webhook 不走 `caBundle` 字段，而是在 apiserver 启动时通过 `--authentication-token-webhook-config-file` / `--authorization-webhook-config-file` 指定 kubeconfig 文件来配置。

### 1.2 相关概念辨析：Aggregated API Server 与 Conversion Webhook

除了以上三类"狭义 Webhook"，K8s 还有两个常被误认为是 Webhook 的扩展机制：

- **Aggregated API Server（API 聚合）**：**不是 Webhook**，而是一个完整的 API Server 替代实现。kube-apiserver 根据 `APIService` 对象，把特定 API Group 的**整条 HTTPS 请求代理过去**（典型例子：`metrics-server`）。它也用 TLS，但通过 `APIService.spec.caBundle` 字段配置，与本文讨论的机制分属不同路径。
- **CRD Conversion Webhook**：是 Webhook，但用途单一——只负责 **CRD 多版本之间的对象格式互转**。配置在 `CustomResourceDefinition.spec.conversion.webhook` 里，TLS 原理与 Admission Webhook 完全一致。

三者定位对比：

| 机制 | 本质 | 是否叫"Webhook" |
|------|------|-----------------|
| Authn / Authz / Admission | apiserver 主动回调外部 HTTPS 做决策 | ✅ 是 Webhook |
| CRD Conversion | apiserver 主动回调外部 HTTPS 做对象转换 | ✅ 是 Webhook（范围很窄） |
| Aggregated API Server | apiserver 把请求**代理**给另一个 API Server | ❌ 不是 Webhook，是 API 代理 |

### 1.3 Mutating vs Validating：执行顺序和边界

Admission Webhook 在 apiserver 里的执行严格分阶段：**先串行执行所有 Mutating Webhook（按字典序，可以相互感知修改），再做 Schema 校验，最后并行执行所有 Validating Webhook（只读，任何一个拒绝就整体失败），通过后才写入 etcd**。

| 维度 | Mutating | Validating |
|------|----------|------------|
| 是否允许修改对象 | ✅ 返回 JSONPatch 修改 | ❌ 只能返回 allow/deny |
| 执行顺序 | 串行（避免 patch 冲突） | 并行（互不影响） |
| 执行时机 | 早于 schema 校验 | 晚于 schema 校验 |
| 幂等性要求 | 必须幂等（可能重放） | 天然幂等 |

### 1.4 典型应用场景

- **Mutating**（修改对象）：Sidecar 注入（Istio/Linkerd）、默认标签/注解补全、默认 `resources.requests` 填充、临时凭证注入
- **Validating**（校验对象）：拒绝特权容器/hostNetwork、命名规范强制、单对象资源配额、镜像仓库白名单

---

## 2. 为什么 Webhook 必须用 TLS

K8s 的 Admission Webhook（包括 `MutatingAdmissionWebhook` 和 `ValidatingAdmissionWebhook`）是 apiserver 在处理资源请求时主动回调的 HTTPS 服务。**K8s 规范强制要求这个回调必须走 HTTPS**，不允许明文 HTTP。

这个设计有两层目的：

**① 防止中间人篡改**

apiserver 在创建 Pod 时会把完整的资源对象以 JSON 格式发送给 webhook。如果走 HTTP，攻击者可以在传输路径上拦截并修改请求，注入恶意容器镜像、挂载敏感 hostPath 或篡改安全上下文。TLS 加密保证了通信内容不被窥探和篡改。

**② 确认 Webhook 服务身份的真实性**

TLS 不只是加密，更重要的是**身份验证**。apiserver 必须能确认它所调用的 HTTPS 端点确实是集群内可信的 Webhook 服务，而不是一个伪装的恶意服务器。`caBundle` 字段正是为此而存在——它告诉 apiserver 用哪个 CA 来验证 Webhook 服务器的证书。

---

## 3. 核心概念：caBundle 是什么

`caBundle` 是 `MutatingWebhookConfiguration`（或 `ValidatingWebhookConfiguration`）中每个 webhook 条目里的一个字段，其值是**对 Webhook 服务端证书进行签名的 CA 证书的 Base64 编码**。

### 3.1 和普通 HTTPS 的区别：为什么需要这个字段

浏览器访问 HTTPS 网站时，信任由系统内置的公共 CA 决定（DigiCert、Let's Encrypt 等），只要服务端证书来自这些 CA 就被信任。Webhook 的情况不同：集群是私有环境，Webhook Service 用的是内部 DNS 名称（如 `example-webhook.example-system.svc`），公共 CA 根本不会为这种名称签发证书。

K8s 因此采用**显式信任配置**——在 `caBundle` 里直接指定要信任的 CA，不依赖系统证书池。一句话总结：**`caBundle` 是 apiserver 的"信任锚"，告诉它"凡是由这个 CA 签发的服务器证书，都是可信的"**。

### 3.2 在配置中的位置

```yaml
apiVersion: admissionregistration.k8s.io/v1
kind: MutatingWebhookConfiguration
metadata:
  name: example-webhook
webhooks:
  - name: example.com/mutate-pod
    clientConfig:
      service:
        name: example-webhook
        namespace: example-system
        path: /mutate-pod
        port: 443
      caBundle: LS0tLS1CRUdJTiBDRVJUSUZJQ0FURS0tLS0t...   # ← 重点
    rules:
      - apiGroups: [""]
        apiVersions: ["v1"]
        resources: ["pods"]
        operations: ["CREATE"]
    admissionReviewVersions: ["v1"]
    sideEffects: None
```

apiserver 在调用 Webhook 前会解码这个字段得到 CA 证书，然后用它验证 Webhook 服务器在 TLS 握手中出示的 `tls.crt`——具体流程在第 4 章展开。

### 3.3 为什么叫 "Bundle" 而不是 "Cert"？设计精髓

很多人第一次见到这个字段会困惑：既然 Webhook 只需要一个 CA 来验签，为什么叫 "ca **Bundle**" 而不是 "caCert"？

**答案**：`caBundle` 是一个可以**装多张 CA 证书的容器**——它允许按顺序拼接多个 PEM 块，apiserver 会把它们全部作为可信信任锚，**只要 Server 证书能匹配其中任意一张 CA，验签就通过**。

```
单证书容器（假想的 caCert 设计）        Bundle 容器（caBundle 实际设计）
  ┌─────────┐                           ┌─────────┐
  │ CA-v1   │   ← 只能装一张              │ CA-v1   │   ← 旧 CA
  └─────────┘     替换时必然有时间差       ├─────────┤
                                         │ CA-v2   │   ← 新 CA
                                         ├─────────┤
                                         │ Root-CA │   ← 根 CA（中间 CA 场景）
                                         └─────────┘
```

这个看似微小的设计，解决了两个关键问题：

**① 零中断 CA 轮换**

CA 证书总会过期，过期前必须换新。如果 `caBundle` 只能装一张，替换时序必然是"先改信任锚，再换 Server 证书"，两步之间的窗口里 apiserver 会用新 CA 去验旧证书，必然失败。Bundle 设计允许在过渡期**同时信任新旧两张 CA**，从而实现零中断切换：

```
T0: caBundle = [CA-v1]，Server 证书由 CA-v1 签
T1: caBundle = [CA-v1, CA-v2]    ← 两张都信任（过渡期）
T2: Server 证书换成 CA-v2 签的     ← 验签仍通过（匹配到 CA-v2）
T3: caBundle = [CA-v2]            ← 移除旧 CA
全程零中断 ✓
```

cert-manager 的 cainjector 在续签 CA 时就是按这个策略自动维护 `caBundle` 的，用户完全无感知。

**② 支持证书链与多签发者**

企业内网常见 Root CA（离线保管，几十年不换）→ 中间 CA（在线，定期轮换）→ Server 证书的三层结构。此时 `caBundle` 需要同时放 Root CA 和 Intermediate CA，apiserver 才能完整构建信任链。此外，同一个 Webhook 被多集群/federation 调用时，不同集群用不同 CA 签发——Bundle 设计允许把所有可能的 CA 都声明进去，一套配置覆盖多环境。

### 3.4 设计精髓一句话

> `caBundle` 不是"存一张 CA 证书的字段"，而是一个**可演进的信任锚集合**。精髓在于：**把"当前信任谁"和"谁签发了 Server 证书"解耦**，让两者可以各自独立演进。

配合这个理念，这个字段还有几个细节值得一提：

| 设计点 | 背后的考量 |
|--------|------------|
| **PEM 格式拼接** | 无需自定义 schema，openssl/标准库即可拼装多张证书 |
| **Base64 编码** | YAML 安全字符集，避免 PEM 的换行符破坏结构 |
| **嵌入式配置**（而非引用 Secret） | WebhookConfiguration 是 cluster-scoped 的，apiserver 读取时无需跨 namespace 查 Secret，减少启动依赖 |

---

## 4. apiserver 调用 Webhook 的 TLS 认证流程

下图展示了从用户提交 `kubectl apply` 到 Webhook 处理完成的完整链路，重点标出 TLS 认证发生的环节：

```
用户                    kube-apiserver                Webhook Pod
 │                           │                             │
 │  kubectl apply pod.yaml   │                             │
 │ ─────────────────────────>│                             │
 │                           │                             │
 │                           │ 1. 鉴权、准入控制器链        │
 │                           │ 2. 触发 MutatingWebhook     │
 │                           │    规则匹配 (rules: pods/CREATE)
 │                           │                             │
 │                           │ 3. 查找 webhook.clientConfig│
 │                           │    .service / .url          │
 │                           │                             │
 │                           │ 4. 解码 caBundle            │
 │                           │    → 临时信任锚              │
 │                           │                             │
 │                           │ 5. 发起 TLS 握手            │
 │                           │ ─────────────────────────> │
 │                           │    ClientHello              │
 │                           │ <─────────────────────────  │
 │                           │    ServerHello +            │
 │                           │    Certificate (tls.crt)    │
 │                           │                             │
 │                           │ 6. 验证服务器证书            │
 │                           │    ├── 证书未过期？          │
 │                           │    ├── SAN 包含             │
 │                           │    │   example-webhook         │
 │                           │    │   .example-system.svc ？  │
 │                           │    └── 签发者 = caBundle    │
 │                           │        中的 CA ？           │
 │                           │                             │
 │                           │ 7. 验证通过 ✓               │
 │                           │    TLS 握手完成             │
 │                           │                             │
 │                           │ 8. 发送 AdmissionReview     │
 │                           │    (JSON, TLS 加密)         │
 │                           │ ─────────────────────────> │
 │                           │                             │
 │                           │ 9. Webhook 处理业务逻辑     │
 │                           │    (注入 sidecar, 校验标签等)│
 │                           │ <─────────────────────────  │
 │                           │    AdmissionResponse        │
 │                           │    (allowed/denied + patch) │
 │                           │                             │
 │                           │ 10. 继续后续准入链或写 etcd  │
 │ <─────────────────────────│                             │
 │  返回结果给用户             │                             │
```

### 4.1 TLS 握手的三项验证

apiserver 对 Webhook 服务器证书执行标准的 X.509 链式验证，需要同时满足：

| 验证项 | 说明 | 失败后果 |
|--------|------|----------|
| **信任链验证** | `tls.crt` 的签发者必须是 `caBundle` 中的 CA | `tls: failed to verify certificate: x509: certificate signed by unknown authority` |
| **SAN 验证** | 证书的 Subject Alternative Name 必须包含 Webhook 的实际访问域名 | `x509: certificate is valid for X, not Y` |
| **有效期验证** | 证书的 `NotBefore` ≤ 当前时间 ≤ `NotAfter` | `x509: certificate has expired or is not yet valid` |

> **⚠️ 注意**：K8s 1.19+ 已经弃用基于 CommonName 的验证，必须在 SAN（Subject Alternative Names）中显式列出所有域名和 IP。不在 SAN 里的域名即使写在 CN 里也不会被接受。

---

## 5. TLS 证书的三要素

一套完整的 Webhook TLS 方案由三个对象组成，它们分别承担"谁可信 / 出示什么 / 用什么签"的角色：

| | 对象 | 存放位置 | 作用 |
|---|------|----------|------|
| ① | **CA 证书** (`ca.crt`) | 经 Base64 编码写入 `caBundle` 字段 | apiserver 的信任锚——决定信任哪个颁发者 |
| ② | **服务端证书** (`tls.crt`) | K8s Secret | Webhook 在 TLS 握手时出示给 apiserver |
| ③ | **服务端私钥** (`tls.key`) | K8s Secret（与 ②  同 Secret） | 与 `tls.crt` 配对，**绝不可外泄或提交 git** |

### 5.1 SAN 的完整列表

Webhook Service 通常有多个访问路径，SAN 必须覆盖所有可能被 apiserver 使用的域名：

```
# 以 Service: example-webhook, Namespace: example-system 为例

DNS:example-webhook                                  # Service 短名（同 namespace 访问）
DNS:example-webhook.example-system                   # Service.Namespace 形式
DNS:example-webhook.example-system.svc               # 标准 in-cluster 域名（apiserver 默认使用此形式）
DNS:example-webhook.example-system.svc.cluster.local # 完整 FQDN
DNS:localhost                                        # 本地开发调试
IP:127.0.0.1                                         # 本地 IP
```

**apiserver 实际使用哪个域名？** 取决于 `clientConfig.service` 字段的写法，K8s 内部会将其解析为 `<service>.<namespace>.svc` 格式。因此 `DNS:<service>.<namespace>.svc` 这一条 **必须** 出现在 SAN 中。

---

## 6. 证书的签发方式

### 6.1 方案一：手动自签证书

适合小型集群、离线环境、对证书有完全掌控需求的场景。**优点**是无外部依赖、完全掌控 CA；**缺点**是手动续签、CA 私钥需要妥善保管、多集群管理复杂。

**生成流程**：

```bash
# ── 第一步：生成 CA 私钥（4096 位，只保留在本机）
openssl genrsa -out ca.key 4096

# ── 第二步：自签 CA 证书（有效期 10 年）
openssl req -new -x509 -days 3650 -key ca.key \
  -subj "/CN=example-webhook-ca" \
  -out ca.crt

# ── 第三步：生成 Server 私钥（2048 位）
openssl genrsa -out tls.key 2048

# ── 第四步：生成 Server 证书签名请求（CSR）
openssl req -new -key tls.key \
  -subj "/CN=example-webhook.example-system.svc" \
  -out tls.csr

# ── 第五步：写 SAN 扩展配置
cat > san.ext << EOF
subjectAltName = DNS:example-webhook,\
DNS:example-webhook.example-system,\
DNS:example-webhook.example-system.svc,\
DNS:example-webhook.example-system.svc.cluster.local,\
DNS:localhost,IP:127.0.0.1
keyUsage = critical, digitalSignature, keyEncipherment
extendedKeyUsage = serverAuth, clientAuth
EOF

# ── 第六步：用 CA 签发 Server 证书（有效期 1 年）
openssl x509 -req -days 365 -in tls.csr \
  -CA ca.crt -CAkey ca.key -CAcreateserial \
  -extfile san.ext \
  -out tls.crt

# ── 第七步：生成 caBundle（base64 编码，去换行）
# 使用兼容 GNU/Linux 与 macOS 的写法
base64 < ca.crt | tr -d '\n' > caBundle.b64
```

签完后的层级关系是：`ca.key` 自签出 `ca.crt`（有效期 10 年），再用 `ca.crt` + `ca.key` 签出 `tls.crt`（有效期 1 年，配对 `tls.key`）。

**将证书注入集群**：

```bash
# 创建 TLS Secret（Webhook Pod 挂载此 Secret）
kubectl create secret tls example-webhook-tls \
  --cert=tls.crt \
  --key=tls.key \
  -n example-system

# 将 caBundle 注入 MutatingWebhookConfiguration
CA_BUNDLE=$(base64 < ca.crt | tr -d '\n')
kubectl patch mutatingwebhookconfiguration example-webhook \
  --type='json' \
  -p="[{'op':'replace','path':'/webhooks/0/clientConfig/caBundle','value':'${CA_BUNDLE}'}]"
```

---

### 6.2 方案二：cert-manager 自动管理

适合生产环境、多 Webhook 场景、证书生命周期需要自动化管理的场景。**优点**是全自动续签、支持多种 Issuer（ACME、Vault、企业 CA 等）；**缺点**是引入了 cert-manager 组件，增加集群复杂度。

cert-manager 由三个组件协同工作：**cert-manager 控制器**（监听 Certificate CRD，负责签发）、**cert-manager-webhook**（CRD 校验）、**cainjector**（自动将 CA 填入带有 annotation 的资源 `caBundle` 字段）。

**核心对象关系**——理解这三个对象的职责就够了，不用死记 YAML：

```text
Issuer / ClusterIssuer
  └─ 定义“由谁来签发证书”
       │
       ▼
Certificate
  ├─ 定义“要签什么证书、SAN 是什么、写入哪个 Secret”
  └─ 签发后生成 Secret: example-webhook-tls
       │
       ├─ Webhook Pod 挂载 Secret，读取 tls.crt / tls.key
       │
       └─ cainjector 根据 inject-ca-from 注入 caBundle
            ▼
      MutatingWebhookConfiguration.webhooks[].clientConfig.caBundle
```

**第一步：准备 Issuer / ClusterIssuer**

```yaml
apiVersion: cert-manager.io/v1
kind: Issuer
metadata:
  name: example-ca-issuer
  namespace: example-system
spec:
  ca:
    secretName: example-ca-secret   # 保存 CA 证书和私钥的 Secret
```

> 实际生产环境也可以使用 `ClusterIssuer`，或接入 Vault、ACME、企业 CA 等签发源。这里用 `Issuer` 只是为了说明对象关系。

**第二步：申请 Webhook Server 证书**

```yaml
apiVersion: cert-manager.io/v1
kind: Certificate
metadata:
  name: example-webhook-tls
  namespace: example-system
spec:
  secretName: example-webhook-tls   # cert-manager 签发后写入此 Secret
  dnsNames:
    - example-webhook
    - example-webhook.example-system
    - example-webhook.example-system.svc
    - example-webhook.example-system.svc.cluster.local
  issuerRef:
    name: example-ca-issuer
    kind: Issuer
```

**第三步：让 cainjector 注入 caBundle**

```yaml
apiVersion: admissionregistration.k8s.io/v1
kind: MutatingWebhookConfiguration
metadata:
  name: example-webhook
  annotations:
    cert-manager.io/inject-ca-from: "example-system/example-webhook-tls"
webhooks:
  - name: example.com/mutate-pod
    clientConfig:
      caBundle: ""   # 留空，由 cainjector 自动填充
      # ...
```

创建 Certificate 对象后，cert-manager 立即签发并写入 Secret；在 `renewBefore` 时间到达时自动续签、更新 Secret；cainjector 监听 Secret 变化，随即更新所有带 `inject-ca-from` annotation 的 WebhookConfiguration 的 `caBundle` 字段——整个过程无需人工介入。

---

## 7. 证书的存放与加载

### 7.1 存放位置

```
集群内的对象分布
─────────────────────────────────────────────────────────
Secret (example-system/example-webhook-tls)
  ├── tls.crt   ← Webhook 服务端证书
  └── tls.key   ← Webhook 服务端私钥

MutatingWebhookConfiguration (cluster-scoped)
  └── webhooks[].clientConfig.caBundle  ← CA 证书 Base64
─────────────────────────────────────────────────────────

Pod 内的文件系统
─────────────────────────────────────────────────────────
/etc/webhook/tls/
  ├── tls.crt   ← 从 Secret 挂载
  └── tls.key   ← 从 Secret 挂载
─────────────────────────────────────────────────────────
```

### 7.2 Helm Chart 配置：Secret 挂载

```yaml
# deployment.yaml（示例）
volumes:
  - name: tls-certs
    secret:
      secretName: example-webhook-tls

containers:
  - name: example-webhook
    volumeMounts:
      - name: tls-certs
        mountPath: /etc/webhook/tls
        readOnly: true
```

### 7.3 Go 服务端加载证书：静态加载 vs 热重载

Webhook 服务加载证书有两种方式，选择哪种直接影响证书续签时是否需要中断服务：

| | 静态加载 | 热重载 |
|---|---|---|
| **实现方式** | `tls.LoadX509KeyPair` 启动时读一次 | `tls.Config.GetCertificate` 每次握手时回调 |
| **证书续签生效** | 需要重启 Pod | 自动生效，无需重启 |
| **适用场景** | 手动管理证书、低频续签 | cert-manager 自动续签、生产环境 |

**热重载的关键设计**：用 `tls.Config.GetCertificate`（[文档](https://pkg.go.dev/crypto/tls#Config)）替代 `Certificates` 字段，每次 TLS 握手时动态返回最新证书，搭配 `atomic.Pointer` 保证无锁并发安全。

```go
// 热重载核心：GetCertificate 在每次握手时被 tls 标准库回调
srv := &http.Server{
    Addr: ":8443",
    TLSConfig: &tls.Config{
        MinVersion:     tls.VersionTLS12,
        GetCertificate: reloader.GetCertificate, // ← 替代静态 Certificates 字段
    },
}
srv.ListenAndServeTLS("", "") // 空字符串：证书由 GetCertificate 提供
```

### 7.4 证书热重载与 K8s Projected Secret 的配合

K8s 更新 Secret 并同步到挂载 Volume 时，**不是原地覆盖文件，而是原子性地替换整个目录**（通过 symlink 切换到新目录），因此文件监听器必须监听**目录**而非单个文件，否则无法感知这次变更：

```
Secret 内容更新
      │
      ▼
kubelet 在 Pod 内执行原子目录替换：
  /etc/webhook/tls/..data  ──symlink──→  新目录（含新 tls.crt / tls.key）
      │
      ▼
文件监听器（如 fsnotify）检测到目录 Rename/Create 事件
      │                            ↑
      │              监听目录，而非文件本身
      ▼              （github.com/fsnotify/fsnotify）
重新执行 tls.LoadX509KeyPair，原子写入最新证书
      │
      ▼
下一次 TLS 握手自动使用新证书 ✓  （无需重启 Pod）
```

---

## 8. 实战：Helm Chart 工程化方案

在工程实践中，建议把 Webhook TLS 管理抽象成**通用可切换模式**。一个常见的 Helm Chart 可以把 TLS 证书来源设计为三类：

```
values.tls
  ├── selfSigned      → 开发 / 测试 / 离线环境：Helm 直接渲染 TLS Secret + caBundle
  ├── certManager     → 生产 / 多集群环境：cert-manager 签发 Secret，cainjector 注入 caBundle
  └── existingSecret  → GitOps / 安全团队统一托管证书：Chart 只引用已有 Secret
```

### 8.1 推荐的目录结构

```text
project/
  ├── chart/
  │   ├── templates/
  │   │   ├── deployment.yaml
  │   │   ├── mutatingwebhookconfiguration.yaml
  │   │   └── tls-secret.yaml
  │   └── values.yaml
  └── certs/                    # 本地生成目录，必须加入 .gitignore
      ├── ca.crt                # CA 公钥证书，base64 后写入 caBundle
      ├── ca.key                # CA 私钥，只保留在本机或密钥系统
      ├── tls.crt               # Webhook 服务端证书，进入 Secret
      ├── tls.key               # Webhook 服务端私钥，进入 Secret
      └── caBundle.b64          # base64(ca.crt)，无换行
```

> **职责边界**：`caBundle` 只给 kube-apiserver 用来验证 Webhook 服务端证书；Webhook Pod 本身通常只需要挂载 `tls.crt` 和 `tls.key`。`ca.key` 只用于签发证书，既不应进入 K8s Secret，也不应提交到代码仓库。

### 8.2 自签证书模式的完整流程

证书生成细节已经在第 6.1 节说明，工程集成的关键是**让 Helm 直接消费生成好的证书文件**，而不是在 Chart 里硬编码证书内容。核心手段是 `helm --set-file`：它会在渲染时把本地文件内容读出来，写入对应的 values 字段，由模板最终渲染进 Secret 和 `caBundle`。

```bash
# 假设已按第 6.1 节生成 ./certs/tls.crt、./certs/tls.key、./certs/caBundle.b64
helm upgrade --install example-webhook ./chart \
  -n example-system --create-namespace \
  --set-file tls.serverCert=./certs/tls.crt \
  --set-file tls.serverKey=./certs/tls.key \
  --set-file tls.caBundle=./certs/caBundle.b64

# 验证 caBundle 已写入 WebhookConfiguration
kubectl get mutatingwebhookconfiguration example-webhook \
  -o jsonpath='{.webhooks[0].clientConfig.caBundle}' | wc -c
```

### 8.3 Helm values 设计

把三种证书来源统一收敛到 `tls` 配置下即可，不需要为每种模式单独设计一套 values：

```yaml
# values.yaml
tls:
  # 模式一：cert-manager 自动签发与续签
  certManager:
    enabled: false
    issuer:
      existing: false
      name: ""          # 使用已有 Issuer / ClusterIssuer 时填写
      kind: Issuer      # 或 ClusterIssuer
    duration: 8760h
    renewBefore: 720h

  # 模式二：自签证书，由 helm --set-file 注入
  serverCert: ""        # tls.crt 内容
  serverKey: ""         # tls.key 内容
  caBundle: ""          # base64(ca.crt)，单行无换行

  # 模式三：引用已有 Secret；设置后 Chart 不再渲染 Secret
  existingSecret:
    name: ""

  # Pod 内挂载路径，示例值可按项目习惯调整
  mountPath: /etc/webhook/tls
```

渲染逻辑建议保持互斥：

| 模式 | 触发条件 | Chart 行为 |
|------|----------|------------|
| `certManager` | `tls.certManager.enabled=true` | 渲染 `Certificate`，跳过手写 `caBundle`，由 cainjector 注入 |
| `existingSecret` | `tls.existingSecret.name` 非空 | 不渲染 TLS Secret，只挂载已有 Secret |
| `selfSigned` | 以上都未启用 | 用 `serverCert` / `serverKey` 渲染 TLS Secret，并把 `caBundle` 写入 WebhookConfiguration |

---

## 9. 证书轮换与续签策略

### 9.1 Server 证书续签

Server 证书续签的关键原则是：**复用原有 CA，只重新生成 `tls.crt` / `tls.key`，不要修改 `caBundle`**。这样 apiserver 仍然用原来的 CA 信任链验证新 Server 证书。

```bash
# 假设已复用原 ca.crt / ca.key 重新生成 ./certs/tls.crt 和 ./certs/tls.key
kubectl create secret tls example-webhook-tls \
  --cert=./certs/tls.crt \
  --key=./certs/tls.key \
  -n example-system \
  --dry-run=client -o yaml | kubectl apply -f -

# 如果 Webhook 服务支持证书热重载，新证书会在 Secret 投射更新后自动生效；
# 如果不支持热重载，执行滚动重启让进程重新加载证书：
kubectl rollout restart deployment/example-webhook -n example-system
```

> **⚠️ 注意**：如果重新生成了 CA（而非仅续签 Server 证书），必须同时更新 `MutatingWebhookConfiguration` 的 `caBundle` 字段，否则 apiserver 无法验证新证书。

### 9.2 CA 轮换的注意事项

CA 轮换比 Server 证书续签复杂，因为 `caBundle` 和 Server 证书必须同时切换，且有时间窗口风险：

```
不正确的顺序（会导致短暂中断）：
  1. 更新 caBundle（新 CA）
  2. 更新 Secret（新 Server 证书）
  ← 步骤 1-2 之间，apiserver 用新 CA 验证旧证书 → 失败

更稳妥的轮换顺序：
  1. caBundle 中同时放新旧两个 CA（caBundle 支持多证书 PEM 拼接）
  2. 更新 Secret（新 Server 证书，新 CA 签发）
  3. 验证 Webhook 工作正常
  4. caBundle 中移除旧 CA

注意：这套顺序的目标是缩短或消除中断窗口，但实际效果还取决于 Secret 投射延迟、Pod 是否支持证书热重载、WebhookConfiguration 更新传播延迟等因素。
```

cert-manager 可以自动续签证书，并由 cainjector 自动注入 `caBundle`，能显著降低人工操作风险。但涉及 CA 轮换时，仍建议在灰度环境验证 Secret 更新、Pod 热重载和 `caBundle` 注入的时序，不能默认假设所有场景都零中断。

---

## 10. 常见问题排查

### 10.1 `x509: certificate signed by unknown authority`

**原因**：caBundle 与实际签发 Server 证书的 CA 不匹配。

```bash
# 导出 caBundle 中的 CA 证书
kubectl get mutatingwebhookconfiguration example-webhook \
  -o jsonpath='{.webhooks[0].clientConfig.caBundle}' | base64 -d > ca-from-webhook.crt

# 用 caBundle 中的 CA 验证 Server 证书链
openssl verify -CAfile ca-from-webhook.crt tls.crt
# tls.crt: OK  ← 期望输出

# 可选：对比本地 ca.crt 与 caBundle 中 CA 的指纹，确认二者是同一张证书
openssl x509 -in ca.crt -noout -fingerprint -sha256
openssl x509 -in ca-from-webhook.crt -noout -fingerprint -sha256
```

### 10.2 `x509: certificate is valid for X, not Y`

**原因**：Server 证书的 SAN 没有包含 apiserver 实际使用的域名。

```bash
# 查看证书的 SAN
openssl x509 -in tls.crt -noout -ext subjectAltName
# X509v3 Subject Alternative Name:
#   DNS:example-webhook, DNS:example-webhook.example-system, ...

# 检查 Webhook 注册的 Service
kubectl get mutatingwebhookconfiguration example-webhook \
  -o jsonpath='{.webhooks[0].clientConfig.service}'
# {"name":"example-webhook","namespace":"example-system","path":"/mutate-pod","port":443}
# ← apiserver 会访问 example-webhook.example-system.svc
# ← 必须在 SAN 中存在
```

### 10.3 `context deadline exceeded` 或 Webhook 超时

```bash
# 检查 Webhook Pod 是否 Running
kubectl get pods -n example-system -l app=example-webhook

# 检查 Webhook Pod 日志
kubectl logs -n example-system -l app=example-webhook --tail=50

# 检查 Service 是否能在集群内被访问到
# 如果 /healthz 暴露在普通 HTTP 管理端口上，使用：
kubectl run curl-check --rm -it --image=curlimages/curl -- \
  curl -v http://example-webhook.example-system.svc:8080/healthz

# 如果要验证 HTTPS admission 端口，访问 Webhook 注册的 path：
kubectl run curl-check --rm -it --image=curlimages/curl -- \
  curl -vk https://example-webhook.example-system.svc/mutate-pod
```

### 10.4 cert-manager 证书未签发

```bash
# 查看 Certificate 状态
kubectl get certificate -n example-system example-webhook-tls
kubectl describe certificate -n example-system example-webhook-tls

# 查看 CertificateRequest
kubectl get certificaterequest -n example-system

# 查看 cert-manager 日志
kubectl logs -n cert-manager \
  -l app.kubernetes.io/name=cert-manager --tail=100
```

---

## 11. 安全最佳实践

### 11.1 证书安全

| 实践 | 说明 |
|------|------|
| **CA 私钥永不入集群** | `ca.key` 只用于签发，不应存入 K8s Secret 或代码仓库 |
| **`.gitignore` 覆盖证书目录** | 本地证书输出目录和所有 `*.key` 文件必须加入 `.gitignore` |
| **使用短有效期** | Server 证书建议 90~365 天，配合自动续签 |
| **最小 Subject** | 不在证书 Subject 里携带冗余敏感信息 |
| **禁用 TLS < 1.2** | `tls.Config.MinVersion = tls.VersionTLS12` |

### 11.2 RBAC 安全

如果证书通过 K8s Secret Volume 挂载到 Pod，Webhook 进程通常**不需要** `get/watch secrets` 权限；kubelet 会负责把 Secret 内容投射成文件。只有当程序主动调用 K8s API 读取或监听 Secret 时，才需要给 ServiceAccount 授权。

```yaml
# 仅当程序主动通过 K8s API 读取 Secret 时才需要此权限
apiVersion: rbac.authorization.k8s.io/v1
kind: Role
metadata:
  name: example-webhook-secret-reader
  namespace: example-system
rules:
  - apiGroups: [""]
    resources: ["secrets"]
    resourceNames: ["example-webhook-tls"]
    verbs: ["get", "watch"]
```

### 11.3 failurePolicy 的安全权衡

```yaml
webhooks:
  - name: example.com/mutate-pod
    failurePolicy: Ignore   # Webhook 不可用时，请求正常放行
    # failurePolicy: Fail   # Webhook 不可用时，请求被拒绝（更安全但影响可用性）
```

- `Fail`：确保每个 Pod 都经过 Webhook 处理，但 Webhook 宕机会阻塞所有 Pod 创建
- `Ignore`：Webhook 宕机不影响正常业务，但可能有 Pod 绕过 Webhook 直接创建

生产环境建议：**安全敏感场景用 `Fail`，运营类 Webhook 用 `Ignore`**。

---

## 附录：证书相关命令速查

```bash
# 查看证书详情
openssl x509 -in tls.crt -noout -text

# 查看证书有效期
openssl x509 -in tls.crt -noout -dates

# 查看证书 SAN
openssl x509 -in tls.crt -noout -ext subjectAltName

# 验证证书链
openssl verify -CAfile ca.crt tls.crt

# 查看 caBundle 内容
kubectl get mutatingwebhookconfiguration <name> \
  -o jsonpath='{.webhooks[0].clientConfig.caBundle}' | base64 -d | \
  openssl x509 -noout -text

# 模拟 TLS 握手（调试）
openssl s_client -connect example-webhook.example-system.svc:443 \
  -CAfile ca.crt -servername example-webhook.example-system.svc

# 检查证书与私钥是否匹配（modulus 应相同）
openssl x509 -noout -modulus -in tls.crt | md5sum
openssl rsa  -noout -modulus -in tls.key | md5sum
```

---

## 参考资料

- K8s 官方文档：[Dynamic Admission Control](https://kubernetes.io/docs/reference/access-authn-authz/extensible-admission-controllers/)、[Using Admission Controllers](https://kubernetes.io/docs/reference/access-authn-authz/admission-controllers/)、[Webhook Token Authentication](https://kubernetes.io/docs/reference/access-authn-authz/authentication/#webhook-token-authentication)、[Webhook Authorization](https://kubernetes.io/docs/reference/access-authn-authz/webhook/)、[CRD Webhook Conversion](https://kubernetes.io/docs/tasks/extend-kubernetes/custom-resources/custom-resource-definitions/#webhook-conversion)、[API Aggregation Layer](https://kubernetes.io/docs/concepts/extend-kubernetes/api-extension/apiserver-aggregation/)
- cert-manager：[官方文档](https://cert-manager.io/docs/) · [CA Injector](https://cert-manager.io/docs/concepts/ca-injector/)
- Go TLS：[`crypto/tls.Config`](https://pkg.go.dev/crypto/tls#Config) · [`fsnotify`](https://github.com/fsnotify/fsnotify)
- 规范：[RFC 5280（X.509）](https://datatracker.ietf.org/doc/html/rfc5280) · [RFC 6125（SAN 验证）](https://datatracker.ietf.org/doc/html/rfc6125)
