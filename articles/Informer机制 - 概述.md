# K8s Informer 机制概述
本文写于2021年9月3日，kubernetes 版本 v1.22.1。天气：多云 ☁️ ～\
ps：如理解有偏差，欢迎随时指正。
## 前言
K8s 中所有的对象都可以理解为是一种资源，包括：Pod、Node、PV、PVC、ns、configmap、service 等等。对于每一种内建资源，K8s 都已经实现了对应的 Informer 机制。以 PodInformer 为例，它包含一个 Informer 和 Lister。
```go
type PodInformer interface {
	Informer() cache.SharedIndexInformer
	Lister() v1.PodLister
}
```
Informer 是一种 SharedIndexInformer 类型，也是我们后续说明的重点。Lister 提供 List 和 Get 方法，能够按照 selector 和 namespace/name 按需获取对应的资源
```go
type PodLister interface {
    List(selector labels.Selector) (ret []*v1.Pod, err error)
    Pods(namespace string) PodNamespaceLister
    PodListerExpansion
}
```
此时会有以下疑问
* 为什么需要 informer 机制呢？
>引用《Kubernetes 源码剖析》一书中的介绍：在 Kubernetes 系统中，组件之间的通过 HTTP 协议进行通信，在不依赖任何中间件的情况下需要保证消息的实时性、可靠性、顺序性等。那么 Kubernetes 是如何做到的呢？答案就是 Informer 机制。

* informer 机制是怎么实现的呢？
>这就是本文的主要内容，首先通过[大的框架](#大的框架)从整体框架的角度了解 informer 机制的流程，之后在[各组件简介](#各组件简介)了解学习其中不同组件的角色和功能，并通过[关键方法之源码解析](#关键方法之源码解析)从源码的角度解析一些重要的函数，帮助进一步理解思想。同时[Informer 机制中的数据同步流向](#Informer-机制中的数据同步流向)在整个 informer 机制中是非常核心的，需要理解清楚。最后例举[一些思考](#一些思考)。end：一些[TODO](#TODO)等待完善。

* informer 机制对我们开发者具体有什么用呢？
>最直接的就是：可以非常方便的动态获取各种资源的实时变化，开发者只需要在对应的 informer 上调用`AddEventHandler`，添加相应的逻辑处理`AddFunc`、`DeleteFunc`、`UpdateFunc`，就可以处理资源的`Added`、`Deleted`、`Updated`动态变化。这样，整个开发流程就变得非常简单，开发者只需要注重回调的逻辑处理，而不用关心具体事件的生成和派发。

### 本文的组织结构如下：
* [大的框架](#大的框架)
* [各组件简介](#各组件简介)
* [Informer 机制中的数据同步流向](#Informer-机制中的数据同步流向)
* [关键方法之源码解析](#关键方法之源码解析)
* [一些思考](#一些思考)
* [TODO](#TODO)

## 大的框架
![framework.png](https://github.com/NoicFank/picture/raw/main/deltaFIFO/framework.png)

kubernetes Informer 机制的整体框架如上图所示，我们从使用者的角度出发，可以发现只有绿色的部分需要我们关心/实现。也就是：
1. 调用`AddEventHandler`，添加相应的逻辑处理`AddFunc`、`DeleteFunc`、`UpdateFunc`
2. 实现 worker 逻辑从 workqueue 中消费 obj-key 即可。

可以发现用户需要实现的只是自身业务的逻辑，所有的数据存储、同步、分发都由 kubernetes 内建的 client-go 完成了，
也就是图中剩余的蓝色的部分，其中包含：
1. SharedIndexInformer：内部包含 controller 和 Indexer，手握控制器和存储，并实现了[sharedIndexInformer 共享机制](#sharedIndexInformer-共享机制)
2. [Reflector](#Reflector)：这是远端（API Server）和本地（DeltaFIFO、Indexer、Listener）之间数据同步逻辑的核心，通过[ListAndWatch 方法](#ListAndWatch-方法)来实现
3. [DeltaFIFO](#DeltaFIFO)：Reflector 中存储待处理 obj(确切说是 Delta)的地方，存储本地**最新的**数据，提供数据 Add、Delete、Update 方法，以及执行 relist 的[Replace 方法](#Replace-方法)
4. [Indexer(Local Store)](#indexerlocal-store)：本地**最全的**数据存储，提供数据存储和数据索引功能。
5. [HandleDeltas](#HandleDeltas-方法)：消费 DeltaFIFO 中排队的 Delta，同时更新给 Indexer，并通过[distribute 方法](#distribute-方法)派发给对应的 Listener 集合
6. [workqueue](#workqueue)：回调函数处理得到的 obj-key 需要放入其中，待 worker 来消费，支持延迟、限速、去重、并发、标记、通知、有序。

### sharedIndexInformer 共享机制

对于同一个资源，会存在多个 Listener 去关心它的变化，如果每一个 Listener 都来实例化一个对应的 Informer 实例，那么会存在非常多冗余的 List、watch 操作，导致 API Server 的压力山大。因此一个良好的设计思路为：`Singleton 模式`，一个资源只实例化一个 Informer，后续所有的 Listener 都共享这一个 Informer 实例即可。这就是 K8s 中 Informer 的共享机制。

下面我们通过源码看看 K8s 内部是如何实现 Informer 的共享机制的：\
所有的 Informer 都通过同一个工厂`SharedInformerFactory`来生成：
* 其内部存在一个 map，名为`informers`来存储所有当前已经实例化的所有 informer。
* 通过`InformerFor`这个方法来实现共享机制，也就是 Singleton 模式，具体见下述代码和注解。
```go
type sharedInformerFactory struct {
	...
	// 工厂级别(所有 informer)默认的 resync 时间
	defaultResync    time.Duration
	// 每个 informer 具体的 resync 时间
	customResync     map[reflect.Type]time.Duration
	// informer 实例的 map
	informers map[reflect.Type]cache.SharedIndexInformer
    ...
}

// 共享机制 通过 InformerFor 来完成
func (f *sharedInformerFactory) InformerFor(
	obj runtime.Object, 
	newFunc internalinterfaces.NewInformerFunc,
) cache.SharedIndexInformer {
	...
	informerType := reflect.TypeOf(obj)
	// 如果已经有 informer 实例 就直接返回该实例
	informer, exists := f.informers[informerType]
	if exists {
		return informer
	}
	// 如果不存在该类型的 informer
	// 1. 设置 informer 的 resync 时间
	resyncPeriod, exists := f.customResync[informerType]
	if !exists {
		resyncPeriod = f.defaultResync
	}
	// 2. 实例化该 informer
	informer = newFunc(f.client, resyncPeriod)
	// 3. 在 map 中记录该 informer
	f.informers[informerType] = informer

	return informer
}
```
>**多个同步时间说明**：\
sharedInformerFactory 中存在一个默认同步时间 defaultResync，这是所有从这个工厂生产出来的 Informer 的默认同步时间，当然每个 informer 可以自定义同步时间，就存储在 customResync 中。此时关联这个 informer 的多个 listeners 的默认同步时间就是对应 informer 的同步时间，同样的 Listener 也可以设置自己的同步时间，就产生了`syncingListeners`。

**共享机制 example：podInformer** \
结合文章开头所说，每一个内建资源都有对应的 Informer 机制，同时内部包含一个 Informer 和 Lister，我们以 pod 为例子说明整个共享 informer 的流程。
见下述代码及注解。
```go
// 1. 通过 Pods 实例化 podInformer，可以看到传入了 factory
func (v *version) Pods() PodInformer {
	return &podInformer{factory: v.factory, namespace: v.namespace, tweakListOptions: v.tweakListOptions}
}
// 2. podInformer 内部就两个属性：Informer 和 Lister
type PodInformer interface {
	Informer() cache.SharedIndexInformer
	Lister() v1.PodLister
}
// 2.1 通过 InformerFor 获得 podInformer 实例
func (f *podInformer) Informer() cache.SharedIndexInformer {
	return f.factory.InformerFor(&corev1.Pod{}, f.defaultInformer)
}
// 2.2 获取 podInformer 实例的 Indexer，之后实例化 PodListener，提供 List 和 Pods 功能
func (f *podInformer) Lister() v1.PodLister {
	return v1.NewPodLister(f.Informer().GetIndexer())
}
```

## 各组件简介
### Reflector
首先了解其包含流程 sharedIndexInformer->controller->reflector，因此一个 informer 就有一个 Reflector。
Reflector 负责监控对应的资源，其中包含 ListerWatcher、store(DeltaFIFO)、lastSyncResourceVersion、resyncPeriod 等信息，
当资源发生变化时，会触发相应 obj 的变更事件，并将该 obj 的 delta 放入 DeltaFIFO 中。
它提供一个非常重要的[ListAndWatch 方法](#ListAndWatch-方法)

### DeltaFIFO
DeltaFIFO 队列在 Reflector 内部，它作为了远端（API Server）和本地（Indexer、Listener）之间的传输桥梁。简单来说，它是一个生产者消费者队列，拥有 FIFO 的特性，操作的资源对象为 Delta。每一个 Delta 包含一个操作类型和操作对象。更多内容见[Informer 机制-DeltaFIFO](./Informer 机制%20-%20DeltaFIFO.md)

### Indexer(Local Store)
Indexer(local store)是 Informer 机制中本地最全的数据存储，其通过 DeltaFIFO 中最新的 Delta 不停的更新自身信息，同时需要在本地(DeltaFIFO、Indexer、Listener)之间执行同步，以上两个更新和同步的步骤都由 Reflector 的 ListAndWatch 来触发。同时在本地 crash，需要进行 replace 时，也需要查看到 Indexer 中当前存储的所有 key。更多内容见[Informer 机制-Indexer 初探](./Informer 机制%20-%20Indexer.md)

>**注意**：Reflector 包含 DeltaFIFO，也即是 Reflector 中的 store 使用的为 DeltaFIFO。在 DeltaFIFO 中有 Indexer，也即是其中的 KnownObjects 属性为 Indexer。因此除了 controller 拥有 Indexer、reflector 之外，DeltaFIFO 也拥有 Indexer。

### Listener
通过 informer 的`AddEventHandler`或`AddEventHandlerWithResyncPeriod`就可以向 informer 注册新的 Listener，这些 Listener 共享同一个 informer，
也就是说一个 informer 可以拥有多个 Listener，是一对多的关系。
当 HandleDeltas 处理 DeltaFIFO 中的 Delta 时，会将这些更新事件派发给注册的 Listener。当然这里具体派发给哪些 Listener 有一定的规则，具体如下：
* 派发给`listeners`：DeltaType 为`Added`、`Updated`、`Deleted`、新旧资源版本号不一致的`Replaced`
* 派发给`syncingListeners`：DeltaType 为`Sync`、新旧资源版本号一致的`Replaced`

> **syncingListeners 与 listeners** \
> deltaFIFO 设计之初就分为了两条线，一个是正常 CUD 的 listeners，一个是 sync 的 listener(syncingListeners)。当我们通过 AddEventHandler 方法添加 handler 时，listeners 和 syncingListeners 始终一致，因为它们的同步倒计时一致。通过 AddEventHandlerWithResyncPeriod 方法添加 handler，因为个性化倒计时，所以 listeners 和 syncingListeners 会不一致。

### workqueue
Listener 通过回调函数接收到对应的 event 之后，需要将对应的 obj-key 放入 workqueue 中，从而方便多个 worker 去消费。workqueue 内部主要有 queue、dirty、processing 三个结构，其中 queue 为 slice 类型保证了有序性，dirty 与 processing 为 hashmap，提供去重属性。使用 workqueue 的优势：
* 并发：支持多生产者、多消费者
* 去重：由 dirty 保证一段时间内的一个元素只会被处理一次
* 有序：FIFO 特性，保证处理顺序，由 queue 来提供
* 标记：标示以恶搞元素是否正在被处理，由 processing 来提供
* 延迟：支持延迟队列，延迟将元素放入队列
* 限速：支持限速队列，对放入的元素进行速率限制
* 通知：ShutDown 告知该 workqueue 不再接收新的元素

## Informer 机制中的数据同步流向
这部分为 Informer 机制中数据同步的核心思路。需要知道有四类数据存储需要同步：API Server、DeltaFIFO、Listener、Indexer。对于这四部分，可以简单理解：**API Server 侧为最权威的数据、DeltaFIFO 为本地最新的数据、Indexer 为本地最全的数据、Listener 为用户侧做逻辑用的数据。**。在这其中，存在两条同步通路，一条为远端与本地之间的通路，另一条为本地内部的通路，接下来，让我们对这两条通路进行详细的理解。

### 远端通路：远端(API Server) ⇔ 本地(DeltaFIFO、Indexer、Listener)
远端通路可以理解为两类，第一类为通过`List`行为产生的同步行为，这类 event 的 DeltaType 为`Replaced`，同时只有在 Reflector 初始启动时才会产生。另一类为通过`Watch`行为产生的同步行为，对于 watch 到的`Added、Modified、Deleted`类型的 event，对应的 DeltaType 为`Added、Updated、Deleted`。以上步骤为 Reflector 的`ListAndWatch`方法将 API Server 侧的 obj 同步到本地 DeltaFIFO 中。当对应 event 的 Delta 放入 DeltaFIFO 之后，就通过 Controller 的`HandleDeltas`	方法，将对应的 Delta 更新到 Indexer 和 Listener 上。具体更新步骤见：[HandleDeltas 实现逻辑](#HandleDeltas-方法)

### 本地通路：本地(DeltaFIFO、Indexer、SyncingListener）之间同步
本地通路是通过 Reflector 的`ListAndWatch`方法中运行一个 goroutine 来执行定期的`Resync`操做。首先通过 ShouldResync 计算出`syncingListener`,之后其中的 store.Resync 从 Indxer 拉一遍所有 objs 到 DeltaFIFO 中(list)，其中的 Delta 为`Sync`状态。如果 DeltaFIFO 的 items 中存在该 obj，就不会添加该 obj 的 sync delta。之后 handleDeltas 就会同步 DeltaFIFO 中的 Sync Delta 给 syncingListeners 和 Indexer。当然这个过程中，别的状态的 delta 会被通知给所有的 listener 和 Indexer。站在 Indexer 的角度，这也是一种更新到最新状态的过程。站在本地的视角，DeltaFIFO、Indexer、Listener 都是从 DelataFIFO 中接收 API Server 发来最新数据。

## 关键方法之源码解析
### ListAndWatch 方法
```go
func (r *Reflector) ListAndWatch(stopCh <-chan struct{}) error { 
	// list
	...
	// resync
	...
	// watch
	...
}

func (r *Reflector) Run(stopCh <-chan struct{}) {
	...
	wait.BackoffUntil(func() {
		if err := r.ListAndWatch(stopCh); err != nil {
			r.watchErrorHandler(r, err)
		}
	}, r.backoffManager, true, stopCh)
	...
}
```
该方法就是 Informer 机制的**核心**，在看源码的过程中就能感受到它把大的逻辑框架穿起来了。其内部做了三件事：**list->resync->watch**，每一件都特别重要。
同时在 Reflector 的 Run 方法中，由`BackoffUntil`函数保护`ListAndWatch`运行。如果遇到 watch event 出错(IO 失败)，ListAndWatch 会退出，此时由 BackoffUntil 函数负责重启，可以理解 BackoffUntil 为 ListAndWatch 的'监工'。除了 stopChan 发来停止消息以外，如果 ListAndWatch'罢工'(遇到错误退出)，都会负责再重启，来恢复 ListAndWatch 的'工作'。

下面，我们对这三部分 **list->resync->watch** 进行说明。

**list** 
```go
func (r *Reflector) ListAndWatch(stopCh <-chan struct{}) error { 
	// list
	if err := func() error {
		...
		// 1. 开启 goroutine 执行 list
		go func() {
			defer func() {
				if r := recover(); r != nil {
					// list 失败，向 panicCh 发送信号
					panicCh <- r
				}
			}()
			// 执行 list 操作，从 API Server 侧获取所有 obj 集合
			...
			// 成功完成 list
			close(listCh)
		}()
		// 2. 等待执行 list 操作的 goroutine 结束，或者 stopCh、panicCh 终止
		select {
		case <-stopCh:
			return nil
		case r := <-panicCh:
			panic(r)
		case <-listCh:
		}
		...
		// 待研究 watch cache 是什么？
		if options.ResourceVersion == "0" && paginatedResult {
			r.paginatedResult = true
		}

		r.setIsLastSyncResourceVersionUnavailable(false) // list was successful
		...
		listMetaInterface, err := meta.ListAccessor(list)
		...
		resourceVersion = listMetaInterface.GetResourceVersion()
		// 从 list 中整理所有 obj 为一个数组
		items, err := meta.ExtractList(list)

		// 3. 将 API Server 侧的最新 Obj 集合同步到 DeltaFIFO 中 最终调用 DeltaFIFO 的 Replace 方法
		if err := r.syncWith(items, resourceVersion); err != nil {
			return fmt.Errorf("unable to sync list result: %v", err)
		}
		r.setLastSyncResourceVersion(resourceVersion)
		...
		return nil
	}(); err != nil {
		return err
	}
	// resync
	...
	// watch
	...
}
```
list 操作在 ListAndWatch 中只会运行一次，简单来说，也可看作三个步骤：
1. 派发 goroutine 去 API Server 拉取最新的 Obj 集合
2. 等待 goroutine 结束，`listCh`接收到信号，表示 list 完成。或者`stopCh`、`panicCh`发来信号。其中 stopCh 表示调用者需要停止，panicCh 表示 goroutine 的 list 过程出错了
3. 整理 API Server 侧拉取到的最新 obj 集合，同时`syncWith`到 DeltaFIFO 中（最终调用 DeltaFIFO 的 Replace 方法）。

> 注意：对于 relist 操作，目前理解：是由于 watch 阶段遇到错误导致 ListAndWatch 退出，但是退出的 err=nil，此时通过外层的 Backoffuntil 来负责重启 ListAndWatch，
这样又回执行一遍新的 List，开启新的 Resyn goroutine，再持续 watch。这也就是 DeltaFIFO 中 DeltaType 为`Replace`的 Delta 产生的源头

**resync**
```go
func (r *Reflector) ListAndWatch(stopCh <-chan struct{}) error { 
	// list
	...
	// resync
	go func() {
		resyncCh, cleanup := r.resyncChan()
		defer func() {
			cleanup() // Call the last one written into cleanup
		}()
		for {
			select {
			case <-resyncCh:
			case <-stopCh:
				return
			case <-cancelCh:
				return
			}
			if r.ShouldResync == nil || r.ShouldResync() {
				klog.V(4).Infof("%s: forcing resync", r.name)
				if err := r.store.Resync(); err != nil {
					resyncerrc <- err
					return
				}
			}
			cleanup()
			resyncCh, cleanup = r.resyncChan()
		}
	}()
	// watch
	...
}
```

这部分是通过派发 goroutine 来完成的，内部通过 for 死循环来定期执行`Resync`操作，`resyncChan()`会定期向`resyncCh`发来信号，定期的时间由 resyncPeriod 属性来设置。
整个过程直到`cancelCh`或者`stopCh`发来停止信号，其中
cancelCh 表示本次 ListAndWatch 结束了，stopCh 表示上层(调用者)发来停止信号。
在每次的`Resync`操作操作中：
1. 首先调用`ShouldResync`函数，其具体实现在 sharedProcessor 中，其会根据每一个 Listener 的同步时间来选出当前期待/需要进行 Resync 的 Listener 放入`syncingListeners`中。
```go
func (p *sharedProcessor) shouldResync() bool {
	p.listenersLock.Lock()
	defer p.listenersLock.Unlock()

	p.syncingListeners = []*processorListener{}

	resyncNeeded := false
	now := p.clock.Now()
	// 遍历所有的 Listener，将同步时间已经到了的
	// Listener 加入 syncingListeners
	for _, listener := range p.listeners {
		if listener.shouldResync(now) {
			resyncNeeded = true
			p.syncingListeners = append(p.syncingListeners, listener)
			listener.determineNextResync(now)
		}
	}
	return resyncNeeded
}
```
2. 调用 store.Resync()，具体由 DeltaFIFO 中的 Resync 来实现，想要完成将 Indexer 中的 obj 全部刷到 DeltaFIFO 中（list）。
需要注意，在这个过程中，如果 DeltaFIFO 的 items 中已经存在该 obj，就不需要放了。因为我们的目的就是同步本地之间的 obj 信息，
既然在 items 中已经存在了该信息，并且该信息一定是本地最新的，未来也会被处理同步到本地所有存储中，因此这里就不需要再添加了。
具体处理细节看下面代码注解。
```go
func (f *DeltaFIFO) Resync() error {
	f.lock.Lock()
	defer f.lock.Unlock()
	// knownObjects 可以理解为 Indexer
	if f.knownObjects == nil {
		return nil
	}
	// 将 Indexer 中所有的 obj 刷到 DeltaFIFO 中
	keys := f.knownObjects.ListKeys()
	for _, k := range keys {
		if err := f.syncKeyLocked(k); err != nil {
			return err
		}
	}
	return nil
}

func (f *DeltaFIFO) syncKeyLocked(key string) error {
	// 通过 key 在 Indexer 中获得 obj
	obj, exists, err := f.knownObjects.GetByKey(key)
	...
	// 计算 DeltaFIFO 中 Obj 的 key
	id, err := f.KeyOf(obj)
	...
	// 如果在 items 中已经存在该 obj，就不需要再添加了
	if len(f.items[id]) > 0 {
		return nil
	}
	// 如果在 items 中没有该 obj，就添加 Sync 类型的 Deltas
	if err := f.queueActionLocked(Sync, obj); err != nil {
		return fmt.Errorf("couldn't queue object: %v", err)
	}
	return nil
}
```
**watch**
```go
func (r *Reflector) ListAndWatch(stopCh <-chan struct{}) error { 
	// list
	...
	// resync
	...
	// watch
		for {
	    ...
		w, err := r.listerWatcher.Watch(options)
		...
		// 开始 watch
		if err := r.watchHandler(start, w, &resourceVersion, resyncerrc, stopCh); err != nil {
			// 如果不是 stopCh 发来的主动停止，就记录日志
			if err != errorStopRequested {
				...
			}
			// 注意这里返回的为 nil，结合 BackoffUntil 函数看
			return nil
		}
	}
}
```
整个 watch 包在一个 for 死循环中，具体的 watch 行为通过`watchHandler`函数来实现，其内部循环监听 watch 对象（由 listerWatcher.Watch 产生）的`ResultChan`。
如果发来 evenet，并且没有出错，就按照四种类型进行处理：
分别为**Added、Modified、Deleted、Bookmark**，表示有 Obj 被：添加、修改、删除，以及版本更新。之后对于前三种类型，
分别调用 store(DeltaFIFO)的`Add、Update、Delete`方法，
向 DeltaFIFO 中添加 DeltaType 为`Added、Updated、Deleted`的 Delta。后续通过 Pop 函数中的 HandleDeltas 消费这些 Deltas。

### Replace 方法
```go
func (f *DeltaFIFO) Replace(list []interface{}, resourceVersion string) error {
	...
}
```
该方法简单来说分为两步骤：
1. 将 list 中的所有 obj，通过`queueActionLocked`添加状态为`Replaced`的 Delta
2. 找出在本地需要删除的 obj，添加状态为`Deleted`的 Delta。寻找需要删除的 obj 如下：
* 如果 knownObjects（也就是 Indexer）不为空，那就通过`ListKeys`获取 Indexer 的全部 key 集合，记为 knowKeys，之后查找 knowKeys 中存在，但是 list 中不存在的 key，那对应的 obj 就是需要删除的。
* 如果 knownObjects 为空，那就只好退而求其次，遍历 DeltaFIFO 的 items 中全部的 key，查找在 list 中不存在的 key，如果存在，这就是需要删除的 obj。

>注意：老版本不存在 Replaced 状态，全使用 Sync 状态。因此为了兼容老版本，需要设置`emitDeltaTypeReplaced`为 true 来开启 Replaced 状态。
当前版本中，Replaced：从 API Server 侧 list 操作同步最新的 obj 集合。Sync：在本地（DeltaFIFO、Indexer、Listener）之间的同步。

>注意：DeltaFIFO 中的`knownObjects`本质上就是 Indexer，在 sharedIndexInformer 的 Run 方法中可以看到。在 controller 的 NewInformer 中也可以看到。
最主要还是了解 sharedIndexInformer。
```go
// sharedIndexInformer
func (s *sharedIndexInformer) Run(stopCh <-chan struct{}) {
	...
	fifo := NewDeltaFIFOWithOptions(DeltaFIFOOptions{
		KnownObjects:          s.indexer,
		EmitDeltaTypeReplaced: true,
	})
	...
}

// ==================================================================
// 在 controller 的 NewInformer 中
// 在新建 Informer 时调用 NewStore 创建 indexer，并调用 newInformer
func NewInformer(
	lw ListerWatcher,
	objType runtime.Object,
	resyncPeriod time.Duration,
	h ResourceEventHandler,
) (Store, Controller) {
	clientState := NewStore(DeletionHandlingMetaNamespaceKeyFunc)

	return clientState, newInformer(lw, objType, resyncPeriod, h, clientState)
}

// NewStore 生成 indexer，最终使用 ThreadSafeMap 来实现
func NewStore(keyFunc KeyFunc) Store {
	return &cache{
		cacheStorage: NewThreadSafeStore(Indexers{}, Indices{}),
		keyFunc:      keyFunc,
	}
}

// newInformer 中将 indexer 赋值给 DeltaFIFO 中的 KnownObjects
func newInformer(
	lw ListerWatcher,
	objType runtime.Object,
	resyncPeriod time.Duration,
	h ResourceEventHandler,
	clientState Store,
) Controller {
	...
	fifo := NewDeltaFIFOWithOptions(DeltaFIFOOptions{
		KnownObjects:          clientState,
		EmitDeltaTypeReplaced: true,
	})
	...
}
```

### HandleDeltas 方法
```go
func (s *sharedIndexInformer) HandleDeltas(obj interface{}) error {...}
```
该方法为 DeltaFIFO 中的`Pop`函数中 process 方法的具体实现。
其为 sharedIndexInformer 的函数，功能就是循环处理 item(Deltas)中的 Delta，对于每一个 Delta：按照操作类型分类，`Deleted`为一类，剩余操作`Sync, Replaced, Added, Updated`归为另一类：
1. 对于`Deleted`：首先调用 indexer 的**Delete**方法，在本地存储中删除该 Obj。之后调用 distribute 方法，对所有的 Listener 进行**deleteNotification**通知删除 Obj 消息；
2. 对于`Sync, Replaced, Added, Updated`：首先查看在 indexer 中是否能够 get 到该 Obj：
* 如果可以 get：调用 indexer 的**Update**方法，更新本地存储的 Obj，之后调用 distribute 方法，对所有的 Listener 进行**updateNotification**通知更新 Obj 消息；（**注意**：这部分的 distribute 针对 Sync 和部分 Replaced(见下述说明)只需要通知`syncingListeners`，而不是所有的 listeners。通过 distribute 方法最后的 bool 参数来设定，大部分情况设定为 false，说明通知所有的 listeners）
* 如果 get 不到：调用 indexer 的**Add**方法，在本地存储添加该 Obj，之后调用 distribute 方法，对所有的 Listener 进行**addNotification**通知添加 Obj 消息；

>部分 Replaced 的说明 \
这部分 DeltaType 为 Replaced 的 Delta 需要满足：**accessor 与 oldAccessor 的`ResourceVersion`一致**。
其中 accessor 可以理解为当前这个 Delta 的 obj。oldAccessor 的获取方式为：
> 1. 获取 Delta 中的 obj
> 2. 通过 Indexer 的 KeyOf，计算 obj 的 key
> 3. 通过 key 在 indexer 中找到资源，记为 oldObj
>
> 可以理解 oldAccessor 为 oldObj。简单来说，就是查看最新的 obj(API Server、DeltaFIFO)与本地（Indexer）的 obj 的资源版本号是否一致。

### distribute 方法
```go
func (p *sharedProcessor) distribute(obj interface{}, sync bool) {
	...
	if sync {
		for _, listener := range p.syncingListeners {
			listener.add(obj)
		}
	} else {
		for _, listener := range p.listeners {
			listener.add(obj)
		}
	}
}

type sharedProcessor struct {
	...
	listeners        []*processorListener
	syncingListeners []*processorListener
	...
}
```
`distribute`函数在 HandleDeltas 中被调用，用来将 Obj 的更新通知给对应的 Listener。该函数定义在`sharedProcessor`中，在 sharedProcessor 中包含`listeners`和`syncingListeners`，
通过设置函数第二个参数`sync`为 false 或 true，来选择通知给 listeners 集合或者 syncingListeners 集合。
两者的区别如下：`listeners`集合可以理解为是所有的 Listener 集合。`syncingListeners`表示期待或者需要`resync`的 Listener 集合（通过`processorListener`的`requestedResyncPeriod`来设置每一个 Listener 希望多久能够 Resync 一次）。

在具体的实现中，当我们通过`AddEventHandler`方法添加 handler 时，listeners 和 syncingListeners 始终一致，因为 AddEventHandler 内部使用 defalut 的同步时间，使得所有 Listener 的同步倒计时都是一致的。通过`AddEventHandlerWithResyncPeriod`方法添加 handler，因为个性化倒计时，所以 listeners 和 syncingListeners 可能会不一致。

```go
func (s *sharedIndexInformer) AddEventHandler(handler ResourceEventHandler) {
	s.AddEventHandlerWithResyncPeriod(handler, s.defaultEventHandlerResyncPeriod)
}

func (s *sharedIndexInformer)  AddEventHandlerWithResyncPeriod(handler ResourceEventHandler, resyncPeriod time.Duration) {
	...
	if resyncPeriod > 0 {
		...
		if resyncPeriod < s.resyncCheckPeriod {
			if s.started {
				// 如果 Informer 已经启动，Listener 设置的同步时间不能比 Informer 的小
				resyncPeriod = s.resyncCheckPeriod
			} else {
				// 如果 Infromer 没有启动，下调 Infrormer 的同步时间，已适应最小的 Listner 同步时间
				s.resyncCheckPeriod = resyncPeriod
				s.processor.resyncCheckPeriodChanged(resyncPeriod)
			}
		}
	}

	listener := newProcessListener(handler, resyncPeriod, determineResyncPeriod(resyncPeriod, s.resyncCheckPeriod), s.clock.Now(), initialBufferSize)
	...
}
```

>**注意：Listener 的同步时间`requestedResyncPeriod`的设置范围是有要求的** \
最基本的不能比`minimumResyncPeriod`（1秒）小。
其次，其和 Informer 内部的同步时间`resyncCheckPeriod`有关系，具体如下：
需要先理解 Listener 和 Informer 是多对一的关系，一个 Informer 对应多个 Listener，因此 Listener 设置的同步时间`requestedResyncPeriod`
在 Informer**启动之后**就不能比`resyncCheckPeriod`小（因为 Listener 的数据是从 Informer 内部来的，如果它同步的时间比 Informer 的时间还快，显然是没有意义的，若严格点说，也可以理解为是一种错误）。在 Infomer**未启动**时，会下调 Informer 的同步时间，已适应该 Listener 的同步时间。

## 一些思考
* 什么时候需要 Replace？以及 DeltaFIFO 中 Replaced 状态的产生方式？
>首先需要知道的是 Replaced 状态的产生，是由于 Reflector 从 API Server 中 list 所有的 Obj，这些 Obj 对应的 Delta 都会被打上 Replaced 的 DeltaType。那本质上来说，只有一种情况需要 list，也就是 Reflector 刚启动的时候，它会通过内部的`ListAndWatch`函数进行一次 list，后续就通过 watch event 来保证 API Server 和本地之间的同步。但是，我们平时也听过 relist，这种操作，也即是当遇到 watch event 出错(IO 错误)的时候，需要重新去向 API Server 请求一次所有的 Obj。这类场景的本质其实就是第一种，因为`ListAndWatch`是运行在`BackoffUntil`内的，当 ListAndWatch 因为非 stopChan 而发生退出时，就会由 BackoffUntil 在一定时间后拉起，这是就相当于 Reflector 刚启动。由此就可以清楚 Replaced 状态的产生，同它字面的意思一致，就是用 API Server 侧的 Obj 集合**替换**本地内容。

### TODO
* 在整个 k8s 体系下，是通过哪些手段减少对 kube-apiserver 的压力？
> 1. informer 机制：
> * 维护本地 store(Indexer)从而使得 `R` 操作直接访问 Inxer 即可。也即是通过 obj-key 在 indexer 中直接取到 obj。
>* ListAndWatch 机制，减少与 API Server 的交互，只有在起初通过一次 List 来全量获取，后续通过 watch 已增量的方式来更新。
>2. sharedInformer 机制：
>* singleton 模式：同一个资源只有一个 informer 实例，多个 listener 来绑定 informer，从而实现一种资源的改动，通过一个 informer 实例，通知给若干个 listener。避免多个 listener 都与 API Server 打交道。

* kube-apiserver 又是通过哪些手段减少对 etcd 的压力？
> watch cache 方面，待完善。

* 为什么需要提供自定义 resync 的接口？
> 从 listener 角度来看，是为了能够按照业务逻辑来定义个性化的同步时间。比如某些业务只需要一天同步一次，某些业务需要1小时同步一次。\
从 informer 的角度来看，同样的，一些自定义的 CRD，可能我们不需要那么频繁的同步，或者也可能需要非常频繁的同步。针对不同的资源类型，工厂默认的时间显然不能满足，因此不同的 informer 可以定义不同的同步时间。 \
注意的是：连接同一个 informer 的 listener 的同步时间，不能小于 informer 的同步时间。也即是一定是 informer 同步了之后，listener 才能同步。
