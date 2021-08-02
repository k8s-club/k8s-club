[toc]


# 前沿
本文是笔者初探，有任何问题欢迎随时改正。

主要搞清楚以下几个点：
>* List-Watch机制是什么？ 用在k8s中的哪里
>* 为什么需要List-Watch机制？ 能带来什么好处？ 
>* list-watch机制怎么做的？
>* 从源码看list-watch机制

# List-watch机制是什么？

## k8s中各模块之间的协作流程

# 源码解析
首先找到调用ListWatch最主要的部分的代码，之后从这一点出发来理解整个部分。

可以看到在Reflector的Run方法中，调用了wait.BackoffUntil方法，该方法会一直去执行ListAndWatch知道stopCh发来停止的信号。那么BackOffUntil具体是怎么去执行的呢？
```go
// Run repeatedly uses the reflector's ListAndWatch to fetch all the
// objects and subsequent deltas.
// Run will exit when stopCh is closed.
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

## BackoffUntil

BackoffUntil方法的具体实现如下:
* 首先非常明显，该方法是一个死循环，直到stopCh发来停止的消息
* 其次，看到clock，同时有<-t.c()，大概能推测这是一个定时循环调用的方法
* 在每次调用中，会执行f()方法，也即是上一步传入的ListAndWatch方法

这样，总体来说，BackoffUntil就是定时执行ListAndWatch直到stopCh发来停止消息，那具体是怎么样的定时方案呢？我们继续看。

> 注意：这里在for循环开始需要判断stopCh是否发来停止消息，我理解，是为了避免在select中<-t.C()和stopCh都发来消息，但是t抢到select，然后又执行了一次f()的错误情况（因为此时stopCh已经发来停止信号，不应该再执行f()了）。因此在for的开始检查stopCh就可以保证stopCh发来消息就能够及时停止


```go
// BackoffUntil loops until stop channel is closed, run f every duration given by BackoffManager.
//
// If sliding is true, the period is computed after f runs. If it is false then
// period includes the runtime for f.
func BackoffUntil(f func(), backoff BackoffManager, sliding bool, stopCh <-chan struct{}) {
	var t clock.Timer
	for {
		select {
		case <-stopCh:
			return
		default:
		}

		if !sliding {
			t = backoff.Backoff()
		}

		func() {
			defer runtime.HandleCrash()
			f()
		}()

		if sliding {
			t = backoff.Backoff()
		}

		// NOTE: b/c there is no priority selection in golang
		// it is possible for this to race, meaning we could
		// trigger t.C and stopCh, and t.C select falls through.
		// In order to mitigate we re-check stopCh at the beginning
		// of every loop to prevent extra executions of f().
		select {
		case <-stopCh:
			return
		case <-t.C():
		}
	}
}
```

### BackoffManager
具体执行的定时方案和**BackoffManager**相关，查看一下其在Reflect中的信息：
```go
// Reflector watches a specified resource and causes all changes to be reflected in the given store.
type Reflector struct {
  ...
	// listerWatcher is used to perform lists and watches.
	listerWatcher ListerWatcher

	// backoff manages backoff of ListWatch
	backoffManager wait.BackoffManager
  ...
}
```

再看看Reflect的构造方法**NewReflector**：
```go
// NewReflector creates a new Reflector object which will keep the
// given store up to date with the server's contents for the given
// resource. Reflector promises to only put things in the store that
// have the type of expectedType, unless expectedType is nil. If
// resyncPeriod is non-zero, then the reflector will periodically
// consult its ShouldResync function to determine whether to invoke
// the Store's Resync operation; `ShouldResync==nil` means always
// "yes".  This enables you to use reflectors to periodically process
// everything as well as incrementally processing the things that
// change.
func NewReflector(lw ListerWatcher, expectedType interface{}, store Store, resyncPeriod time.Duration) *Reflector {
	return NewNamedReflector(naming.GetNameFromCallsite(internalPackages...), lw, expectedType, store, resyncPeriod)
}
```
其中调用NewNamedReflector来实现，接着看NewNamedReflector，就找到具体的定时方案了：
* 正常情况下，每800ms执行一下
* 如果Api Server出故障了，那就等待[30,60)s，这里具体的等待时间为30s+rand(0,1)\*1.0\*30s,其中的1.0就是jitter。
* 正常情况下2min内如果没有发现ApiServer故障，就认为ApiServer是正常的

>注意：这jitter的数值只有在大于0的时候才会生效，如果小于0就会被忽略，也即是不会延后等待时间。同时，jitter（延后）的时间是jitter(这里为1.0）* 基础时间（这里为800ms）* rang(0，1)。
```go
// NewNamedReflector same as NewReflector, but with a specified name for logging
func NewNamedReflector(name string, lw ListerWatcher, expectedType interface{}, store Store, resyncPeriod time.Duration) *Reflector {
	realClock := &clock.RealClock{}
	r := &Reflector{
		name:          name,
		listerWatcher: lw,
		store:         store,
		// We used to make the call every 1sec (1 QPS), the goal here is to achieve ~98% traffic reduction when
		// API server is not healthy. With these parameters, backoff will stop at [30,60) sec interval which is
		// 0.22 QPS. If we don't backoff for 2min, assume API server is healthy and we reset the backoff.
		backoffManager:    wait.NewExponentialBackoffManager(800*time.Millisecond, 30*time.Second, 2*time.Minute, 2.0, 1.0, realClock),
		resyncPeriod:      resyncPeriod,
		clock:             realClock,
		watchErrorHandler: WatchErrorHandler(DefaultWatchErrorHandler),
	}
	r.setExpectedType(expectedType)
	return r
}
```

列一下，ExponentialBackoffManager官方解释：
```go
/*
NewExponentialBackoffManager returns a manager for managing exponential backoff. Each backoff is jittered and backoff will not exceed the given max. If the backoff is not called within resetDuration, the backoff is reset. This backoff manager is used to reduce load during upstream unhealthiness.
*/
func NewExponentialBackoffManager(initBackoff, maxBackoff, resetDuration time.Duration, backoffFactor, jitter float64, c clock.Clock) BackoffManager {
  ...
}
```
ok，到这里定时调用的方案简单知道了，就开始看ListAndWatch的实现吧。

## ListAndWatch

``` go
/* 
/client-go@v0.19.8/tools/cache/reflector.go
ListAndWatch first lists all items and get the resource version at the moment of call, and then use the resource version to watch. It returns error if ListAndWatch didn't even try to initialize watch.
*/
func (r *Reflector) ListAndWatch(stopCh <-chan struct{}) error {
  ...
}
```

# 参考资料
* 