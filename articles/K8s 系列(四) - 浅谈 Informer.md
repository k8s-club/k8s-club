## 1. 概述
进入 K8s 的世界，会发现有很多的 Controller，它们都是为了完成某类资源(如 pod 是通过 DeploymentController, ReplicaSetController 进行管理)的调谐，目标是保持用户期望的状态。

K8s 中有几十种类型的资源，如何能让 K8s 内部以及外部用户方便、高效的获取某类资源的变化，就是本文 Informer 要实现的。本文将从 Reflector(反射器)、DeltaFIFO(增量队列)、Indexer(索引器)、Controller(控制器)、SharedInformer(共享资源通知器)、processorListener(事件监听处理器)、workqueue(事件处理工作队列) 等方面进行解析。

> 本文及后续相关文章都基于 K8s v1.22

![K8s-informer](https://github.com/k8s-club/k8s-club/raw/main/images/Informer/K8s-informer.png)


## 2. 从 Reflector 说起
Reflector 的主要职责是从 apiserver 拉取并持续监听(ListAndWatch) 相关资源类型的增删改(Add/Update/Delete)事件，存储在由 DeltaFIFO 实现的本地缓存(local Store) 中。

首先看一下 Reflector 结构体定义：

```go
// staging/src/k8s.io/client-go/tools/cache/reflector.go
type Reflector struct {
	// 通过 file:line 唯一标识的 name
	name string

	// 下面三个为了确认类型
	expectedTypeName string
	expectedType     reflect.Type
	expectedGVK      *schema.GroupVersionKind

	// 存储 interface: 具体由 DeltaFIFO 实现存储
	store Store
	// 用来从 apiserver 拉取全量和增量资源
	listerWatcher ListerWatcher

	// 下面两个用来做失败重试
	backoffManager         wait.BackoffManager
	initConnBackoffManager wait.BackoffManager

	// informer 使用者重新同步的周期
	resyncPeriod time.Duration
	// 判断是否满足可以重新同步的条件
	ShouldResync func() bool
	
	clock clock.Clock
	
	// 是否要进行分页 List
	paginatedResult bool
	
	// 最后同步的资源版本号，以此为依据，watch 只会监听大于此值的资源
	lastSyncResourceVersion string
	// 最后同步的资源版本号是否可用
	isLastSyncResourceVersionUnavailable bool
	// 加把锁控制版本号
	lastSyncResourceVersionMutex sync.RWMutex
	
	// 每页大小
	WatchListPageSize int64
	// watch 失败回调 handler
	watchErrorHandler WatchErrorHandler
}
```
从结构体定义可以看到，通过指定目标资源类型进行 ListAndWatch，并可进行分页相关设置。

第一次拉取全量资源(目标资源类型) 后通过 syncWith 函数全量替换(Replace) 到 DeltaFIFO queue/items 中，之后通过持续监听 Watch(目标资源类型) 增量事件，并去重更新到 DeltaFIFO queue/items 中，等待被消费。

watch 目标类型通过 Go reflect 反射实现如下：
```go
// staging/src/k8s.io/client-go/tools/cache/reflector.go
// watchHandler watches w and keeps *resourceVersion up to date.
func (r *Reflector) watchHandler(start time.Time, w watch.Interface, resourceVersion *string, errc chan error, stopCh <-chan struct{}) error {

	...
	if r.expectedType != nil {
		if e, a := r.expectedType, reflect.TypeOf(event.Object); e != a {
			utilruntime.HandleError(fmt.Errorf("%s: expected type %v, but watch event object had type %v", r.name, e, a))
			continue
		}
	}
	if r.expectedGVK != nil {
		if e, a := *r.expectedGVK, event.Object.GetObjectKind().GroupVersionKind(); e != a {
			utilruntime.HandleError(fmt.Errorf("%s: expected gvk %v, but watch event object had gvk %v", r.name, e, a))
			continue
		}
	}
	...
}
```
> - 通过反射确认目标资源类型，所以命名为 Reflector 还是比较贴切的；
> - List/Watch 的目标资源类型在 NewSharedIndexInformer.ListerWatcher 进行了确定，但 Watch 还会在 watchHandler 中再次比较一下目标类型；


## 3. 认识 DeltaFIFO

还是先看下 DeltaFIFO 结构体定义：

```go
// staging/src/k8s.io/client-go/tools/cache/delta_fifo.go
type DeltaFIFO struct {
	// 读写锁、条件变量
	lock sync.RWMutex
	cond sync.Cond

	// kv 存储：objKey1->Deltas[obj1-Added, obj1-Updated...]
	items map[string]Deltas

	// 只存储所有 objKeys
	queue []string

	// 是否已经填充：通过 Replace() 接口将第一批对象放入队列，或者第一次调用增、删、改接口时标记为true
	populated bool
	// 通过 Replace() 接口将第一批对象放入队列的数量
	initialPopulationCount int

	// keyFunc 用来从某个 obj 中获取其对应的 objKey
	keyFunc KeyFunc

	// 已知对象，其实就是 Indexer
	knownObjects KeyListerGetter

	// 队列是否已经关闭
	closed bool

	// 以 Replaced 类型发送(为了兼容老版本的 Sync)
	emitDeltaTypeReplaced bool
}
```

DeltaType 可分为以下类型：
```
// staging/src/k8s.io/client-go/tools/cache/delta_fifo.go
type DeltaType string

const (
	Added   DeltaType = "Added"
	Updated DeltaType = "Updated"
	Deleted DeltaType = "Deleted"
	Replaced DeltaType = "Replaced" // 第一次或重新同步
	Sync DeltaType = "Sync" // 老版本重新同步叫 Sync
)
```

通过上面的 Reflector 分析可以知道，DeltaFIFO 的职责是通过队列加锁处理(queueActionLocked)、去重(dedupDeltas)、存储在由 DeltaFIFO 实现的本地缓存(local Store) 中，包括 queue(仅存 objKeys) 和 items(存 objKeys 和对应的 Deltas 增量变化)，并通过 Pop 不断消费，通过 Process(item) 处理相关逻辑。

![K8s-DeltaFIFO](https://github.com/k8s-club/k8s-club/raw/main/images/Informer/K8s-DeltaFIFO.png)


## 4. 索引 Indexer
上一步 ListAndWatch 到的资源已经存储到 DeltaFIFO 中，接着调用 Pop 从队列进行消费。实际使用中，Process 处理函数由 sharedIndexInformer.HandleDeltas 进行实现。HandleDeltas 函数根据上面不同的 DeltaType 分别进行 Add/Update/Delete，并同时创建、更新、删除对应的索引。

具体索引实现如下：
```go
// staging/src/k8s.io/client-go/tools/cache/index.go
// map 索引类型 => 索引函数
type Indexers map[string]IndexFunc

// map 索引类型 => 索引值 map
type Indices map[string]Index

// 索引值 map: 由索引函数计算所得索引值(indexedValue) => [objKey1, objKey2...]
type Index map[string]sets.String
```

索引函数(IndexFunc)：就是计算索引的函数，这样允许扩展多种不同的索引计算函数。默认也是最常用的索引函数是：`MetaNamespaceIndexFunc`。
索引值(indexedValue)：有些地方叫 indexKey，表示由索引函数(IndexFunc) 计算出来的索引值(如 ns1)。
对象键(objKey)：对象 obj 的 唯一 key(如 ns1/pod1)，与某个资源对象一一对应。

![K8s-indexer](https://github.com/k8s-club/k8s-club/raw/main/images/Informer/K8s-indexer.png)

可以看到，Indexer 由 ThreadSafeStore 接口集成，最终由 threadSafeMap 实现。

> - 索引函数 IndexFunc(如 MetaNamespaceIndexFunc)、KeyFunc(如 MetaNamespaceKeyFunc) 区别：前者表示如何计算索引，后者表示如何获取对象键(objKey)；
> - 索引键(indexKey，有些地方是 indexedValue)、对象键(objKey) 区别：前者表示由索引函数(IndexFunc) 计算出来的索引键(如 ns1)，后者则是 obj 的 唯一 key(如 ns1/pod1)；

## 5. 总管家 Controller
Controller 作为核心中枢，集成了上面的组件 Reflector、DeltaFIFO、Indexer、Store，成为连接下游消费者的桥梁。

Controller 由 controller 结构体进行具体实现：
> 在 K8s 中约定俗成：大写定义的 interface 接口，由对应小写定义的结构体进行实现。

```go
// staging/src/k8s.io/client-go/tools/cache/controller.go
type controller struct {
	config         Config
	reflector      *Reflector // 上面已分析的组件
	reflectorMutex sync.RWMutex
	clock          clock.Clock
}

type Config struct {
	// 实际由 DeltaFIFO 实现
	Queue

	// 构造 Reflector 需要
	ListerWatcher

	// Pop 出来的 obj 处理函数
	Process ProcessFunc

	// 目标对象类型
	ObjectType runtime.Object

	// 全量重新同步周期
	FullResyncPeriod time.Duration

	// 是否进行重新同步的判断函数
	ShouldResync ShouldResyncFunc

	// 如果为 true，Process() 函数返回 err，则再次入队 re-queue
	RetryOnError bool

	// Watch 返回 err 的回调函数
	WatchErrorHandler WatchErrorHandler

	// Watch 分页大小
	WatchListPageSize int64
}
```

Controller 中以 goroutine 协程方式启动 Run 方法，会启动 Reflector 的 ListAndWatch()，用于从 apiserver 拉取全量和监听增量资源，存储到 DeltaFIFO。接着，启动 processLoop 不断从 DeltaFIFO Pop 进行消费。在 sharedIndexInformer 中 Pop 出来进行处理的函数是 HandleDeltas，一方面维护 Indexer 的 Add/Update/Delete，另一方面调用下游 sharedProcessor 进行 handler 处理。

## 6. 启动 SharedInformer
SharedInformer 接口由 SharedIndexInformer 进行集成，由 sharedIndexInformer(这里看到了吧，又是大写定义的 interface 接口，由对应小写定义的结构体进行实现) 进行实现。

看一下结构体定义：
```go
// staging/src/k8s.io/client-go/tools/cache/shared_informer.go
type SharedIndexInformer interface {
	SharedInformer
	// AddIndexers add indexers to the informer before it starts.
	AddIndexers(indexers Indexers) error
	GetIndexer() Indexer
}

type sharedIndexInformer struct {
	indexer    Indexer
	controller Controller

	// 处理函数，将是重点
	processor *sharedProcessor

	// 检测 cache 是否有变化，一把用作调试，默认是关闭的
	cacheMutationDetector MutationDetector

	// 构造 Reflector 需要
	listerWatcher ListerWatcher

	// 目标类型，给 Reflector 判断资源类型
	objectType runtime.Object

	// Reflector 进行重新同步周期
	resyncCheckPeriod time.Duration

	// 如果使用者没有添加 Resync 时间，则使用这个默认的重新同步周期
	defaultEventHandlerResyncPeriod time.Duration
	clock                           clock.Clock

	// 两个 bool 表达了三个状态：controller 启动前、已启动、已停止
	started, stopped bool
	startedLock      sync.Mutex

	// 当 Pop 正在消费队列，此时新增的 listener 需要加锁，防止消费混乱
	blockDeltas sync.Mutex

	// Watch 返回 err 的回调函数
	watchErrorHandler WatchErrorHandler
}

type sharedProcessor struct {
	listenersStarted bool
	listenersLock    sync.RWMutex
	listeners        []*processorListener
	syncingListeners []*processorListener // 需要 sync 的 listeners
	clock            clock.Clock
	wg               wait.Group
}
```

从结构体定义可以看到，通过集成的 controller(上面已分析) 进行 Reflector ListAndWatch，并存储到 DeltaFIFO，并启动 Pop 消费队列，在 sharedIndexInformer 中 Pop 出来进行处理的函数是 HandleDeltas。

所有的 listeners 通过 sharedIndexInformer.AddEventHandler 加入到 processorListener 数组切片中，并通过判断当前 controller 是否已启动做不同处理如下：

```go
// staging/src/k8s.io/client-go/tools/cache/shared_informer.go
func (s *sharedIndexInformer) AddEventHandlerWithResyncPeriod(handler ResourceEventHandler, resyncPeriod time.Duration) {
	...

	// 如果还没有启动，则直接 addListener 加入即可返回
	if !s.started {
		s.processor.addListener(listener)
		return
	}

	// 加锁控制
	s.blockDeltas.Lock()
	defer s.blockDeltas.Unlock()

	s.processor.addListener(listener)
	
	// 遍历所有对象，发送到刚刚新加入的 listener
	for _, item := range s.indexer.List() {
		listener.add(addNotification{newObj: item})
	}
}
```

接着，在 HandleDeltas 中，根据 obj 的 Delta 类型(Added/Updated/Deleted/Replaced/Sync) 调用 sharedProcessor.distribute 给所有监听 listeners 处理。

## 7. 注册 SharedInformerFactory
SharedInformerFactory 作为使用 SharedInformer 的工厂类，提供了高内聚低耦合的工厂类设计模式，其结构体定义如下：
```go
// staging/src/k8s.io/client-go/informers/factory.go
type SharedInformerFactory interface {
	internalinterfaces.SharedInformerFactory // 重点内部接口
	ForResource(resource schema.GroupVersionResource) (GenericInformer, error)
	WaitForCacheSync(stopCh <-chan struct{}) map[reflect.Type]bool

	Admissionregistration() admissionregistration.Interface
	Internal() apiserverinternal.Interface
	Apps() apps.Interface
	Autoscaling() autoscaling.Interface
	Batch() batch.Interface
	Certificates() certificates.Interface
	Coordination() coordination.Interface
	Core() core.Interface
	Discovery() discovery.Interface
	Events() events.Interface
	Extensions() extensions.Interface
	Flowcontrol() flowcontrol.Interface
	Networking() networking.Interface
	Node() node.Interface
	Policy() policy.Interface
	Rbac() rbac.Interface
	Scheduling() scheduling.Interface
	Storage() storage.Interface
}

// staging/src/k8s.io/client-go/informers/internalinterfaces/factory_interfaces.go
type SharedInformerFactory interface {
	Start(stopCh <-chan struct{}) // 启动 SharedIndexInformer.Run
	InformerFor(obj runtime.Object, newFunc NewInformerFunc) cache.SharedIndexInformer // 目标类型初始化
}
```

以 PodInformer 为例，说明使用者如何构建自己的 Informer，PodInformer 定义如下：
```go
// staging/src/k8s.io/client-go/informers/core/v1/pod.go
type PodInformer interface {
	Informer() cache.SharedIndexInformer
	Lister() v1.PodLister
}

由小写的 podInformer 实现(又看到了吧，大写接口小写实现的 K8s 风格)：

type podInformer struct {
	factory          internalinterfaces.SharedInformerFactory
	tweakListOptions internalinterfaces.TweakListOptionsFunc
	namespace        string
}

func (f *podInformer) defaultInformer(client kubernetes.Interface, resyncPeriod time.Duration) cache.SharedIndexInformer {
	return NewFilteredPodInformer(client, f.namespace, resyncPeriod, cache.Indexers{cache.NamespaceIndex: cache.MetaNamespaceIndexFunc}, f.tweakListOptions)
}

func (f *podInformer) Informer() cache.SharedIndexInformer {
	return f.factory.InformerFor(&corev1.Pod{}, f.defaultInformer)
}

func (f *podInformer) Lister() v1.PodLister {
	return v1.NewPodLister(f.Informer().GetIndexer())
}
```

由使用者传入目标类型(&corev1.Pod{})、构造函数(defaultInformer)，调用 SharedInformerFactory.InformerFor 实现目标 Informer 的注册，然后调用 SharedInformerFactory.Start 进行 Run，就启动了上面分析的 SharedIndexedInformer -> Controller -> Reflector -> DeltaFIFO 流程。

> 通过使用者自己传入目标类型、构造函数进行 Informer 注册，实现了 SharedInformerFactory 高内聚低耦合的设计模式。

## 8. 回调 processorListener
所有的 listerners 由 processorListener 实现，分为两组：listeners, syncingListeners，分别遍历所属组全部 listeners，将数据投递到 processorListener 进行处理。

> - 因为各 listeners 设置的 resyncPeriod 可能不一致，所以将没有设置(resyncPeriod = 0) 的归为 listeners 组，将设置了 resyncPeriod 的归到 syncingListeners 组；
> - 如果某个 listener 在多个地方(sharedIndexInformer.resyncCheckPeriod, sharedIndexInformer.AddEventHandlerWithResyncPeriod)都设置了 resyncPeriod，则取最小值 minimumResyncPeriod；

```go
// staging/src/k8s.io/client-go/tools/cache/shared_informer.go
func (p *sharedProcessor) distribute(obj interface{}, sync bool) {
	p.listenersLock.RLock()
	defer p.listenersLock.RUnlock()

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
```

从代码可以看到 processorListener 巧妙地使用了两个 channel(addCh, nextCh) 和一个 pendingNotifications(由 slice 实现的滚动 Ring) 进行 buffer 缓冲，默认的 initialBufferSize = 1024。既做到了高效传递数据，又不阻塞上下游处理，值得学习。

![K8s-processorListener](https://github.com/k8s-club/k8s-club/raw/main/images/Informer/K8s-processorListener.png)


## 9. workqueue 忙起来
通过上一步 processorListener 回调函数，交给内部 ResourceEventHandler 进行真正的增删改(CUD) 处理，分别调用 OnAdd/OnUpdate/OnDelete 注册函数进行处理。

为了快速处理而不阻塞 processorListener 回调函数，一般使用 workqueue 进行异步化解耦合处理，其实现如下：

![K8s-workqueue](https://github.com/k8s-club/k8s-club/raw/main/images/Informer/K8s-workqueue.png)

从图中可以看到，workqueue.RateLimitingInterface 集成了 DelayingInterface，DelayingInterface 集成了 Interface，最终由 rateLimitingType 进行实现，提供了 rateLimit 限速、delay 延时入队(由优先级队列通过小顶堆实现)、queue 队列处理 三大核心能力。

另外，在代码中可看到 K8s 实现了三种 RateLimiter：BucketRateLimiter, ItemExponentialFailureRateLimiter, ItemFastSlowRateLimiter，Controller 默认采用了前两种如下：

```go
// staging/src/k8s.io/client-go/util/workqueue/default_rate_limiters.go
func DefaultControllerRateLimiter() RateLimiter {
	return NewMaxOfRateLimiter(
		NewItemExponentialFailureRateLimiter(5*time.Millisecond, 1000*time.Second),
		// 10 qps, 100 bucket size.  This is only for retry speed and its only the overall factor (not per item)
		&BucketRateLimiter{Limiter: rate.NewLimiter(rate.Limit(10), 100)},
	)
}
```

这样，在用户侧可以通过调用 workqueue 相关方法进行灵活的队列处理，比如失败多少次就不再重试，失败了延时入队的时间控制，队列的限速控制(QPS)等，实现非阻塞异步化逻辑处理。

## 10. 小结
本文通过分析 K8s 中 Reflector(反射器)、DeltaFIFO(增量队列)、Indexer(索引器)、Controller(控制器)、SharedInformer(共享资源通知器)、processorListener(事件监听处理器)、workqueue(事件处理工作队列) 等组件，对 Informer 实现机制进行了解析，通过源码、图文方式说明了相关流程处理，以期更好的理解 K8s Informer 运行流程。

可以看到，K8s 为了实现高效、非阻塞的核心流程，大量采用了 goroutine 协程、channel 通道、queue 队列、index 索引、map 去重等方式；并通过良好的接口设计模式，给使用者开放了很多扩展能力；采用了统一的接口与实现的命名方式等，这些都值得深入学习与借鉴。


*PS: 更多内容请关注 [k8s-club](https://github.com/k8s-club/k8s-club)*


### 参考资料
1. [Kubernetes 官方文档](https://kubernetes.io/)
2. [Kubernetes 源码](https://github.com/kubernetes/kubernetes)
3. [Kubernetes Architectural Roadmap](https://github.com/kubernetes/community/blob/master/contributors/design-proposals/architecture/architectural-roadmap.md)

