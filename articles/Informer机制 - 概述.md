# K8s Informer机制概述
本文写于2021年9月3日，kubernetes版本v1.22.1。天气：多云 ☁️ ～\
ps：如理解有偏差，欢迎随时指正。
## 前言
K8s中所有的对象都可以理解为是一种资源，包括：Pod、Node、PV、PVC、ns、configmap、service等等。对于每一种内建资源，K8s都已经实现了对应的Informer机制。其中包含一个Informer和Lister，Informer是一种SharedIndexInformer类型，也是我们后续说明的重点。Lister提供List和Pods方法，能够按照namespace和selector按需列取对应资源。
```go
type PodInformer interface {
	Informer() cache.SharedIndexInformer
	Lister() v1.PodLister
}
```
此时会有以下疑问
* 为什么需要 informer 机制呢？
>引用《Kubernetes源码剖析》一书中的介绍：在Kubernetes系统中，组件之间的通过HTTP协议进行通信，在不依赖任何中间件的情况下需要保证消息的实时性、可靠性、顺序性等。那么Kubernetes是如何做到的呢？答案就是Informer机制。

* informer机制是怎么实现的呢？
>这就是本文的主要内容，首先通过[大的框架](#大的框架)从整体框架的角度了解informer机制的流程，之后在[各组件简介](#各组件简介)了解学习其中不同组件的角色和功能，并通过[关键方法之源码解析](#关键方法之源码解析)从源码的角度解析一些重要的函数，帮助进一步理解思想。同时[Informer机制中的数据同步流向](#Informer机制中的数据同步流向)在整个informer机制中是非常核心的，需要理解清楚。最后例举[一些思考](#一些思考)。end：一些[TODO](#TODO)等待完善。

* informer机制对我们开发者具体有什么用呢？
>最直接的就是：可以非常方便的动态获取各种资源的实时变化，开发者只需要在对应的informer上调用`AddEventHandler`，添加相应的逻辑处理`AddFunc`、`DeleteFunc`、`UpdateFun`，就可以处理资源的`Added`、`Deleted`、`Updated`动态变化。这样，整个开发流程就变得非常简单，开发者只需要注重回调的逻辑处理，而不用关心具体事件的生成和派发。

### 本文的组织结构如下：
* [大的框架](#大的框架)
* [各组件简介](#各组件简介)
* [Informer机制中的数据同步流向](#Informer机制中的数据同步流向)
* [关键方法之源码解析](#关键方法之源码解析)
* [一些思考](#一些思考)
* [TODO](#TODO)

## 大的框架
![framework.png](https://github.com/NoicFank/picture/raw/main/deltaFIFO/framework.png)

kubernetes Informer机制的整体框架如上图所示，我们从使用者的角度出发，可以发现只有绿色的部分需要我们关心/实现。也就是：
1. 调用`AddEventHandler`，添加相应的逻辑处理`AddFunc`、`DeleteFunc`、`UpdateFun`
2. 实现worker逻辑从workqueue中消费obj-key即可。

可以发现用户需要实现的只是自身业务的逻辑，所有的数据存储、同步、分发都由kubernetes内建的client-go完成了，
也就是图中剩余的蓝色的部分，其中包含：
1. SharedIndexInformer：内部包含controller和Indexer，手握控制器和存储，并实现了[sharedIndexInformer共享机制](#sharedIndexInformer共享机制)
2. [Reflector](#Reflector)：这是远端（APiServer）和本地（DeltaFIFO、Indexer、Listener）之间数据同步逻辑的核心，通过[ListAndWatch方法](#ListAndWatch方法)来实现
3. [DeltaFIFO](#DeltaFIFO)：Reflector中存储待处理obj(确切说是Delta)的地方，存储本地**最新的**数据，提供数据Add、Delete、Update方法，以及执行relist的[Replace方法](#Replace方法)
4. [Indexer(Local Store)](#Indexer(Local%20Store))：本地**最全的**数据存储，提供数据存储和数据索引功能。
5. [HandleDeltas](#HandleDeltas方法)：消费DeltaFIFO中排队的Delta，同时更新给Indexer，并通过[distribute方法](#distribute方法)派发给对应的Listener集合
6. [workqueue](#workqueue)：回调函数处理得到的obj-key需要放入其中，待worker来消费，支持延迟、限速、去重、并发、标记、通知、有序。

### sharedIndexInformer共享机制

对于同一个资源，会存在多个Listener去关心它的变化，如果每一个Listener都来实例化一个对应的Informer实例，那么会存在非常多冗余的List、watch操作，导致ApiServer的压力山大。因此一个良好的设计思路为：`Singleton模式`，一个资源只实例化一个Informer，后续所有的Listener都共享这一个Informer实例即可。这就是K8s中Informer的共享机制。

下面我们通过源码看看K8s内部是如何实现Informer的共享机制的：\
所有的Informer都通过同一个工厂`SharedInformerFactory`来生成：
* 其内部存在一个map，名为`informers`来存储所有当前已经实例化的所有informer。
* 通过`InformerFor`这个方法来实现共享机制，也就是Singleton模式，具体见下述代码和注解。
```go
type sharedInformerFactory struct {
	...
	// 工厂级别(所有informer)默认的resync时间
	defaultResync    time.Duration
	// 每个informer具体的resync时间
	customResync     map[reflect.Type]time.Duration
	// informer实例的map
	informers map[reflect.Type]cache.SharedIndexInformer
    ...
}

// 共享机制 通过InformerFor来完成
func (f *sharedInformerFactory) InformerFor(
	obj runtime.Object, 
	newFunc internalinterfaces.NewInformerFunc,
) cache.SharedIndexInformer {
	...
	informerType := reflect.TypeOf(obj)
	// 如果已经有informer实例 就直接返回该实例
	informer, exists := f.informers[informerType]
	if exists {
		return informer
	}
	// 如果不存在该类型的informer
    // 1. 设置informer的resync时间
	resyncPeriod, exists := f.customResync[informerType]
	if !exists {
		resyncPeriod = f.defaultResync
	}
    // 2. 实例化该informer
	informer = newFunc(f.client, resyncPeriod)
	// 3. 在map中记录该informer
	f.informers[informerType] = informer

	return informer
}
```
>**多个同步时间说明**：\
sharedInformerFactory中存在一个默认同步时间defaultResync，这是所有从这个工厂生产出来的Informer的默认同步时间，当然每个informer可以自定义同步时间，就存储在customResync中。此时关联这个informer的多个listeners的默认同步时间就是对应informer的同步时间，同样的Listener也可以设置自己的同步时间，就产生了`syncingListeners`。

**共享机制example：podInformer** \
结合文章开头所说，每一个内建资源都有对应的Informer机制，同时内部包含一个Informer和Lister，我们以pod为例子说明整个共享informer的流程。
见下述代码及注解。
```go
// 1. 通过Pods实例化podInformer，可以看到传入了factory
func (v *version) Pods() PodInformer {
	return &podInformer{factory: v.factory, namespace: v.namespace, tweakListOptions: v.tweakListOptions}
}
// 2. podInformer内部就两个属性：Informer和Lister
type PodInformer interface {
	Informer() cache.SharedIndexInformer
	Lister() v1.PodLister
}
// 2.1 通过InformerFor获得podInformer实例
func (f *podInformer) Informer() cache.SharedIndexInformer {
	return f.factory.InformerFor(&corev1.Pod{}, f.defaultInformer)
}
// 2.2 获取podInformer实例的Indexer，之后实例话PodListener，提供List和Pods功能
func (f *podInformer) Lister() v1.PodLister {
	return v1.NewPodLister(f.Informer().GetIndexer())
}
```

## 各组件简介
### Reflector
首先了解其包含流程sharedIndexInformer->controller->reflector，因此一个informer就有一个Reflector。
Reflector负责监控对应的资源，其中包含ListerWatcher、store(DeltaFIFO)、lastSyncResourceVersion、resyncPeriod等信息，
当资源发生变化时，会触发相应obj的变更事件，并将该obj的delta放入DeltaFIFO中。
它提供一个非常重要的[ListAndWatch方法](#ListAndWatch方法)

### DeltaFIFO
DeltaFIFO队列在Reflector内部，它作为了远端（Apiserver）和本地（Indexer、Listener）之间的传输桥梁。简单来说，它是一个生产者消费者队列，拥有FIFO的特性，操作的资源对象为Delta。每一个Delta包含一个操作类型和操作对象。更多内容见[Informer机制-DeltaFIFO](./Informer机制%20-%20DeltaFIFO.md)

### Indexer(Local Store)
Indexer(local store)是Informer机制中本地最全的数据存储，其通过DeltaFIFO中最新的Delta不停的更新自身信息，同时需要在本地(DeltaFIFO、Indexer、Listener)之间执行同步，以上两个更新和同步的步骤都由Reflector的ListAndWatch来触发。同时在本地crash，需要进行replace时，也需要查看到Indexer中当前存储的所有key。更多内容见[Informer机制-Indexer初探](./Informer机制%20-%20Indexer.md)

>**注意**：Reflector包含DeltaFIFO，也即是Reflector中的store使用的为DeltaFIFO。在DeltaFIFO中有Indexer，也即是其中的KnownObjects属性为Indexer。因此除了controller拥有Indexer、reflector之外，DeltaFIFO也拥有Indexer。

### Listener
通过informer的`AddEventHandler`或`AddEventHandlerWithResyncPeriod`就可以向informer注册新的Listener，这些Listener共享同一个informer，
也就是说一个informer可以拥有多个Listener，是一对多的关系。
当HandleDeltas处理DeltaFIFO中的Delta时，会将这些更新事件派发给注册的Listener。当然这里具体派发给哪些Listener有一定的规则，具体如下：
* 派发给`listeners`：DeltaType为`Added`、`Updated`、`Deleted`、新旧资源版本号不一致的`Replaced`
* 派发给`syncingListeners`：DeltaType为`Sync`、新旧资源版本号一致的`Replaced`

> **syncingListeners 与 listeners** \
> deltaFIFO设计之初就分为了两条线，一个是正常CUD的listeners，一个是sync的listener(syncingListeners)。当我们通过AddEventHandler方法添加handler时，listeners和syncingListeners始终一致，因为它们的同步倒计时一致。通过AddEventHandlerWithResyncPeriod方法添加handler，因为个性化倒计时，所以listeners和syncingListeners会不一致。

### workqueue
Listener通过回调函数接收到对应的event之后，需要将对应的obj-key放入workqueue中，从而方便多个worker去消费。workqueue内部主要有queue、dirty、processing三个结构，其中queue为slice类型保证了有序性，dirty与processing为hashmap，提供去重属性。使用workqueue的优势：
* 并发：支持多生产者、多消费者
* 去重：由dirty保证一段时间内的一个元素只会被处理一次
* 有序：FIFO特性，保证处理顺序，由queue来提供
* 标记：标示以恶搞元素是否正在被处理，由processing来提供
* 延迟：支持延迟队列，延迟将元素放入队列
* 限速：支持限速队列，对放入的元素进行速率限制
* 通知：ShutDown告知该workqueue不再接收新的元素

## Informer机制中的数据同步流向
这部分为Informer机制中数据同步的核心思路。需要知道有四类数据存储需要同步：ApiServer、DeltaFIFO、Listener、Indexer。对于这四部分，可以简单理解：**Apiserver侧为最权威的数据、DeltaFIFO为本地最新的数据、Indexer为本地最全的数据、Listener为用户侧做逻辑用的数据。**。在这其中，存在两条同步通路，一条为远端与本地之间的通路，另一条为本地内部的通路，接下来，让我们对这两条通路进行详细的理解。

### 远端通路：远端(ApiServer) ⇔ 本地(DeltaFIFO、Indexer、Listener)
远端通路可以理解为两类，第一类为通过`List`行为产生的同步行为，这类event的DeltaType为`Replaced`，同时只有在Reflector初始启动时才会产生。另一类为通过`Watch`行为产生的同步行为，对于watch到的`Added、Modified、Deleted`类型的event，对应的DeltaType为`Added、Updated、Deleted`。以上步骤为Reflector的`ListAndWatch`方法将ApiServer测的obj同步到本地DeltaFIFO中。当对应event的Delta放入DeltaFIFO之后，就通过Controller的`HandleDeltas`	方法，将对应的Delta更新到Indexer和Listener上。具体更新步骤见：[HandleDeltas实现逻辑](#HandleDeltas方法)

### 本地通路：本地(DeltaFIFO、Indexer、SyncingListener）之间同步
本地通路是通过Reflector的`ListAndWatch`方法中运行一个goroutine来执行定期的`Resync`操做。首先通过ShouldResync计算出`syncingListener`,之后其中的store.Resync从Indxer拉一遍所有objs到DeltaFIFO中(list)，其中的Delta为`Sync`状态。如果DeltaFIFO的items中存在该obj，就不会添加该obj的sync delta。之后handleDeltas就会同步DeltaFIFO中的Sync Delta给syncingListeners和Indexer。当然这个过程中，别的状态的delta会被通知给所有的listener和Indexer。站在Indexer的角度，这也是一种更新到最新状态的过程。站在本地的视角，DeltaFIFO、Indexer、Listener都是从DelataFIFO中接收ApiServer发来最新数据。

## 关键方法之源码解析
### ListAndWatch方法
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
该方法就是Informer机制的**核心**，在看源码的过程中就能感受到它把大的逻辑框架穿起来了。其内部做了三件事：**list->resync->watch**，每一件都特别重要。
同时在Reflector的Run方法中，由`BackoffUntil`函数保护`ListAndWatch`运行。如果遇到watch event出错(IO失败)，ListAndWatch会退出，此时由BackoffUntil函数负责重启，可以理解BackoffUntil为ListAndWatch的'监工'。除了stopChan发来停止消息以外，如果ListAndWatch'罢工'(遇到错误退出)，都会负责再重启，来恢复ListAndWatch的'工作'。

下面，我们对这三部分 **list->resync->watch** 进行说明。

**list** 
```go
func (r *Reflector) ListAndWatch(stopCh <-chan struct{}) error { 
	// list
	if err := func() error {
		...
		// 1. 开启goroutine 执行list
		go func() {
			defer func() {
				if r := recover(); r != nil {
					// list失败，向panicCh发送信号
					panicCh <- r
				}
			}()
			// 执行list操作，从ApiServer测获取所有obj集合
			...
			// 成功完成list
			close(listCh)
		}()
		// 2. 等待执行list操作的goroutine结束，或者stopCh、panicCh终止
		select {
		case <-stopCh:
			return nil
		case r := <-panicCh:
			panic(r)
		case <-listCh:
		}
		...
		// 待研究watch cache是什么？
		if options.ResourceVersion == "0" && paginatedResult {
			r.paginatedResult = true
		}

		r.setIsLastSyncResourceVersionUnavailable(false) // list was successful
		...
		listMetaInterface, err := meta.ListAccessor(list)
		...
		resourceVersion = listMetaInterface.GetResourceVersion()
		// 从list中整理所有obj为一个数组
		items, err := meta.ExtractList(list)

		// 3. 将ApiServer测的最新Obj集合同步到DeltaFIFO中 最终调用DeltaFIFO的Replace方法
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
list操作在ListAndWatch中只会运行一次，简单来说，也可看作三个步骤：
1. 派发goroutine去ApiServer拉取最新的Obj集合
2. 等待goroutine结束，`listCh`接收到信号，表示list完成。或者`stopCh`、`panicCh`发来信号。其中stopCh表示调用者需要停止，panicCh表示goroutine的list过程出错了
3. 整理ApiServer测拉取到的最新obj集合，同时`syncWith`到DeltaFIFO中（最终调用DeltaFIFO的Replace方法）。

> 注意：对于relist操作，目前理解：是由于watch阶段遇到错误导致ListAndWatch退出，但是退出的err=nil，此时通过外层的Backoffuntil来负责重启ListAndWatch，
这样又回执行一遍新的List，开启新的Resyn goroutine，再持续watch。这也就是DeltaFIFO中DeltaType为`Replace`的Delta产生的源头

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

这部分是通过派发goroutine来完成的，内部通过for死循环来定期执行`Resync`操作，`resyncChan()`会定期向`resyncCh`发来信号，定期的时间由resyncPeriod属性来设置。
整个过程直到`cancelCh`或者`stopCh`发来停止信号，其中
cancelCh表示本次ListAndWatch结束了，stopCh表示上层(调用者)发来停止信号。
在每次的`Resync`操作操作中：
1. 首先调用`ShouldResync`函数，其具体实现在sharedProcessor中，其会根据每一个Listener的同步时间来选出当前期待/需要进行Resync的Listener放入`syncingListeners`中。
```go
func (p *sharedProcessor) shouldResync() bool {
	p.listenersLock.Lock()
	defer p.listenersLock.Unlock()

	p.syncingListeners = []*processorListener{}

	resyncNeeded := false
	now := p.clock.Now()
	// 遍历所有的Listener，将同步时间已经到了的
	// Listener加入syncingListeners
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
2. 调用store.Resync()，具体由DeltaFIFO中的Resync来实现，想要完成将Indexer中的obj全部刷到DeltaFIFO中（list）。
需要注意，在这个过程中，如果DeltaFIFO的items中已经存在该obj，就不需要放了。因为我们的目的就是同步本地之间的obj信息，
既然在items中已经存在了该信息，并且该信息一定是本地最新的，未来也会被处理同步到本地所有存储中，因此这里就不需要再添加了。
具体处理细节看下面代码注解。
```go
func (f *DeltaFIFO) Resync() error {
	f.lock.Lock()
	defer f.lock.Unlock()
	// knownObjects可以理解为Indexer
	if f.knownObjects == nil {
		return nil
	}
	// 将Indexer中所有的obj刷到DeltaFIFO中
	keys := f.knownObjects.ListKeys()
	for _, k := range keys {
		if err := f.syncKeyLocked(k); err != nil {
			return err
		}
	}
	return nil
}

func (f *DeltaFIFO) syncKeyLocked(key string) error {
	// 通过key在Indexer中获得obj
	obj, exists, err := f.knownObjects.GetByKey(key)
	...
	// 计算DeltaFIFO中Obj的key
	id, err := f.KeyOf(obj)
	...
	// 如果在items中已经存在该obj，就不需要再添加了
	if len(f.items[id]) > 0 {
		return nil
	}
	// 如果在items中没有该obj，就添加Sync类型的Deltas
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
		// 开始watch
		if err := r.watchHandler(start, w, &resourceVersion, resyncerrc, stopCh); err != nil {
			// 如果不是stopCh发来的主动停止，就记录日志
			if err != errorStopRequested {
				...
			}
			// 注意这里返回的为nil，结合BackoffUntil函数看
			return nil
		}
	}
}
```
整个watch包在一个for死循环中，具体的watch行为通过`watchHandler`函数来实现，其内部循环监听watch对象（由listerWatcher.Watch产生）的`ResultChan`。
如果发来evenet，并且没有出错，就按照四种类型进行处理：
分别为**Added、Modified、Deleted、Bookmark**，表示有Obj被：添加、修改、删除，以及版本更新。之后对于前三种类型，
分别调用store(DeltaFIFO)的`Add、Update、Delete`方法，
向DeltaFIFO中添加DeltaType为`Added、Updated、Deleted`的Delta。后续通过Pop函数中的HandleDeltas消费这些Deltas。

### Replace方法
```go
func (f *DeltaFIFO) Replace(list []interface{}, resourceVersion string) error {
	...
}
```
该方法简单来说分为两步骤：
1. 将list中的所有obj，通过`queueActionLocked`添加状态为`Replaced`的Delta
2. 找出在本地需要删除的obj，添加状态为`Deleted`的Delta。寻找需要删除的obj如下：
* 如果knownObjects（也就是Indexer）不为空，那就通过`ListKeys`获取Indexer的全部key集合，记为knowKeys，之后查找knowKeys中存在，但是list中不存在的key，那对应的obj就是需要删除的。
* 如果knownObjects为空，那就只好退而求其次，遍历DeltaFIFO的items中全部的key，查找在list中不存在的key，如果存在，这就是需要删除的obj。

>注意：老版本不存在Replaced状态，全使用Sync状态。因此为了兼容老版本，需要设置`emitDeltaTypeReplaced`为true来开启Replaced状态。
当前版本中，Replaced：从ApiServer测lis操作同步最新的obj集合。Sync：在本地（DeltaFIFO、Indexer、Listener）之间的同步。

>注意：DeltaFIFO中的`knownObjects`本质上就是Indexer，在sharedIndexInformer的Run方法中可以看到。在controller的NewInformer中也可以看到。
最主要还是了解sharedIndexInformer。
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
// 在controller的NewInformer中
// 在新建Informer时调用NewStore创建indexer，并调用newInformer
func NewInformer(
	lw ListerWatcher,
	objType runtime.Object,
	resyncPeriod time.Duration,
	h ResourceEventHandler,
) (Store, Controller) {
	clientState := NewStore(DeletionHandlingMetaNamespaceKeyFunc)

	return clientState, newInformer(lw, objType, resyncPeriod, h, clientState)
}

// NewStore生成indexer，最终使用ThreadSafeMap来实现
func NewStore(keyFunc KeyFunc) Store {
	return &cache{
		cacheStorage: NewThreadSafeStore(Indexers{}, Indices{}),
		keyFunc:      keyFunc,
	}
}

// newInformer中将indexer赋值给DeltaFIFO中的KnownObjects
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

### HandleDeltas方法
```go
func (s *sharedIndexInformer) HandleDeltas(obj interface{}) error {...}
```
该方法为DeltaFIFO中的`Pop`函数中process方法的具体实现。
其为sharedIndexInformer的函数，功能就是循环处理item(Deltas)中的Delta，对于每一个Delta：按照操作类型分类，`Deleted`为一类，剩余操作`Sync, Replaced, Added, Updated`归为另一类：
1. 对于`Deleted`：首先调用indexer的**Delete**方法，在本地存储中删除该Obj。之后调用distribute方法，对所有的Listener进行**deleteNotification**通知删除Obj消息；
2. 对于`Sync, Replaced, Added, Updated`：首先查看在indexer中是否能够get到该Obj：
* 如果可以get：调用indexer的**Update**方法，更新本地存储的Obj，之后调用distribute方法，对所有的Listener进行**updateNotification**通知更新Obj消息；（**注意**：这部分的distribute针对Sync和部分Replaced(见下述说明)只需要通知`syncingListeners`，而不是所有的listeners。通过distribute方法最后的bool参数来设定，大部分情况设定为false，说明通知所有的listeners）
* 如果get不到：调用indexer的**Add**方法，在本地存储添加该Obj，之后调用distribute方法，对所有的Listener进行**addNotification**通知添加Obj消息；

>部分Replaced的说明 \
这部分DeltaType为Replaced的Delta需要满足：**accessor与oldAccessor的`ResourceVersion`一致**。
其中accessor可以理解为当前这个Delta的obj。oldAccessor的获取方式为：
> 1. 获取Delta中的obj
> 2. 通过Indexer的KeyOf，计算obj的key
> 3. 通过key在indexer中找到资源，记为oldObj
>
> 可以理解oldAccessor为oldObj。简单来说，就是查看最新的obj(apiServer、DeltaFIFO)与本地（Indexer）的obj的资源版本号是否一致。

### distribute方法
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
`distribute`函数在HandleDeltas中被调用，用来将Obj的更新通知给对应的Listener。该函数定义在`sharedProcessor`中，在sharedProcessor中包含`listeners`和`syncingListeners`，
通过设置函数第二个参数`sync`为false或true，来选择通知给listeners集合或者syncingListeners集合。
两者的区别如下：`listeners`集合可以理解为是所有的Listener集合。`syncingListeners`表示期待或者需要`resync`的Listener集合（通过`processorListener`的`requestedResyncPeriod`来设置每一个Listener希望多久能够Resync一次）。

在具体的实现中，当我们通过`AddEventHandler`方法添加handler时，listeners和syncingListeners始终一致，因为AddEventHandler内部使用defalut的同步时间，使得所有Listener的同步倒计时都是一致的。通过`AddEventHandlerWithResyncPeriod`方法添加handler，因为个性化倒计时，所以listeners和syncingListeners可能会不一致。

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
				// 如果Informer已经启动，Listener设置的同步时间不能比Informer的小
				resyncPeriod = s.resyncCheckPeriod
			} else {
				// 如果Infromer没有启动，下调Infrormer的同步时间，已适应最小的Listner同步时间
				s.resyncCheckPeriod = resyncPeriod
				s.processor.resyncCheckPeriodChanged(resyncPeriod)
			}
		}
	}

	listener := newProcessListener(handler, resyncPeriod, determineResyncPeriod(resyncPeriod, s.resyncCheckPeriod), s.clock.Now(), initialBufferSize)
	...
}
```

>**注意：Listener的同步时间`requestedResyncPeriod`的设置范围是有要求的** \
最基本的不能比`minimumResyncPeriod`（1秒）小。
其次，其和Informer内部的同步时间`resyncCheckPeriod`有关系，具体如下：
需要先理解Listener和Informer是多对一的关系，一个Informer对应多个Listener，因此Listener设置的同步时间`requestedResyncPeriod`
在Informer**启动之后**就不能比`resyncCheckPeriod`小（因为Listener的数据是从Informer内部来的，如果它同步的时间比Informer的时间还快，显然是没有意义的，若严格点说，也可以理解为是一种错误）。在Infomer**未启动**时，会下调Informer的同步时间，已适应该Listener的同步时间。

## 一些思考
* 什么时候需要Replace？以及DeltaFIFO中Replaced状态的产生方式？
>首先需要知道的是Replaced状态的产生，是由于Reflector从ApiServer中list所有的Obj，这些Obj对应的Delta都会被打上Replaced的DeltaType。那本质上来说，只有一种情况需要list，也就是Reflector刚启动的时候，它会通过内部的`ListAndWatch`函数进行一次list，后续就通过watch event来保证ApiServer和本地之间的同步。但是，我们平时也听过relist，这种操作，也即是当遇到watch event出错(IO错误)的时候，需要重新去向ApiServer请求一次所有的Obj。这类场景的本质其实就是第一种，因为`ListAndWatch`是运行在`BackoffUntil`内的，当ListAndWatch因为非stopChan而发生退出时，就会由BackoffUntil在一定时间后拉起，这是就相当于Reflector刚启动。由此就可以清楚Replaced状态的产生，同它字面的意思一致，就是用ApiServer测的Obj集合**替换**本地内容。

### TODO
* 在整个k8s体系下，是通过哪些手段减少对kube-apiserver的压力？
> 1. informer机制：
> * 维护本地store(Indexer)从而使得 `R` 操作直接访问Inxer即可。也即是通过obj-key在indexer中直接取到obj。
>* ListAndWatch机制，减少与ApiServer的交互，只有在起初通过一次List来全量获取，后续通过watch已增量的方式来更新。
>2. sharedInformer机制：
>* singleton模式：同一个资源只有一个informer实例，多个listener来绑定informer，从而实现一种资源的改动，通过一个informer实例，通知给若干个listener。避免多个listener都与ApiServer打交道。

* kube-apiserver又是通过哪些手段减少对etcd的压力？
> watch cache方面，待完善。

* 为什么需要提供自定义resync的接口？
> 从listener角度来看，是为了能够按照业务逻辑来定义个性化的同步时间。比如某些业务只需要一天同步一次，某些业务需要1小时同步一次。\
从informer的角度来看，同样的，一些自定义的CRD，可能我们不需要那么频繁的同步，或者也可能需要非常频繁的同步。针对不同的资源类型，工厂默认的时间显然不能满足，因此不同的informer可以定义不同的同步时间。 \
注意的是：连接同一个informer的listener的同步时间，不能小于informer的同步时间。也即是一定是informer同步了之后，listener才能同步。
