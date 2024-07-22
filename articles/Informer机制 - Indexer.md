# Indexer(local store)
local store是Informer机制中的本地存储（也会被称为Indexer，但是为了和内部的实现机制中的Indxers区别开(多了个's')，我们接下来将Indexer这个模块称作local store更加合适一些。

## local store存在的意义

最主要的目的就是为了**减少对apiServer的访问压力**。在K8s内部，每一种资源的Informer机制都会使用对应的local store来缓存本地该资源的状态，并只在informer首次启动时全量拉取(list)一次，后续通过watch增量更新local store。从而在worker期望get、list对应的资源时，不必访问远端的apiServer，而是直接访问本地的local store即可。同时支持在本地local store和DeltaFIFO之间的信息定时reSync来reconcile。

## local store与apiSerer的数据同步

本地的local store中的数据与远端apiserer测的最新数据通过`ListAndWatch`机制来同步，也即是首先通过List所有的资源，之后通过Watch来同步数据。如果出现了IO错误，比如：网络错误等。这时会从apiServer重新reList该资源所有的最新数据，并再次进入watch。需要注意的是，reList的数据，首先都到DeltaFIFO中，再通过HandleDeltas将最新的数据同步到Listeners和local store中。同时，local store和deltafifo之间也支持定期进行reSync。

## 重点概念
在local store中最主要的是有4个概念需要理解：
1. `Indexers`: type **Indexers** map[string]IndexFunc
2. `IndexFunc`: type **IndexFunc** func(obj interface{}) ([]string, error)
3. `Indices`: type **Indices** map[string]Index
4. `Index`: type **Index** map[string]sets.String

这几个概念可能会有些许的容易混淆，接下来我们详细解释一波：

1. `Indexers`：索引函数的集合，它为一个map，其key为索引器的名字IndexName(自定义，但要唯一)，value为对应的索引函数IndexFunc
2. `IndexFunc`: 索引函数，它接收一个obj，并实现逻辑来取出/算出该obj的索引数组。需要注意是索引数组，具体取什么或算出什么作为索引完全是我们可以自定义的。
3. `Indices`: 索引数据集合，它为一个map，其key和`Indexers`中的key对应，表示索引器的名字。Value为当前到达数据通过该索引函数计算出来的Index。
4. `Index`: 索引与数据key集合，它的key为索引器计算出来的索引数组中的每一项，value为对应的资源的key(默认namespace/name)集合。

让我们通过一个简单的例子，更加直观的理解。
![indexer.png](../images/Indexer.png)

首先来了ABC三个obj等待被存入Indexer中，第一步将obj们存储于items，在items中以key和obj的方式来存储，这里是真正存储obj真身的地方。下面开始构建和更新索引。第二步，从Indexer中遍历所有的索引方法，我们以`ByName`对应的索引方法`NameIndexFunc`为例，该索引方法能够按照name属性中的多个名字来进行索引。第三步骤，在Indices中拿到`ByName`对应的索引存储`NameIndex`，并通过刚才获得的NameIndexFunc，将obj的key放入NameIndex之中。这就完成了索引的存储。

当然示例中展示的有限，还有更新索引、删除索引等一些功能。结合源码也比较好理解。

### 补充
为了加深对store中四个概念的理解，以下`Indexers`、`IndexFunc`、`Indices`与`Index`进行数据示例。
1. **Indexers** map[string]IndexFunc：包含多个索引函数，为了计算资源对象的键值方法。
```bigquery
说明：
Indexers: {
    "索引器名1": 索引函数1,
    "索引器名2": 索引函数2,
}

示例：
Indexers: {
    "namespace": MetaNamespaceIndexFunc,
    "label": MetaLabelIndexFunc,
    "annotation": MetaAnnotationIndexFunc,
}

```
2. **IndexFunc** func(obj interface{}) ([]string, error)
   就是用来求出索引键的方法，如**cache.MetaNamespaceIndexFunc** (k8s内置的索引方法)，也可以自定义实现不同的索引器。
```bigquery
说明：
Indexers: {
    "索引器名1": IndexFunc1,
    "索引器名2": IndexFunc2,
}

示例：
Indexers: {
    "namespace": MetaNamespaceIndexFunc,
    "label": MetaLabelIndexFunc,
    "annotation": MetaAnnotationIndexFunc,
}

```   
3. **Indices** map[string]Index：包含所有索引器及其key-value对象(即：Index对象)
```bigquery
说明：
Indices: {
    "索引器1": {
        "索引键1": ["对象1", "对象2"], 
        "索引键2": ["对象3", "对象4", "对象5"], 
    },
    "索引器2": {
        "索引键3": ["对象1", "对象2"], 
        "索引键4": ["对象3"], 
    }

}

示例：
Indices: {
    "namespace": {
        "default": ["default/kube-root-ca.crt", "default/configmap-test1", "default/configmap-test2"], 
        ...
    },
    "labels-test": {
        "label-test2": ["default/configmap-test2"], 
        "label-test": ["default/configmap-test1"], 
    }
    "annotation-test": {
        "annotations-test": ["default/configmap-test1"],
        "annotations-test2": ["default/configmap-test2"],
    }
    ...
}
```
4. **Index** map[string]sets.String
   就是某个索引键下的所有对象，方便快速查找。
```bigquery
说明：
Indices: {
    "索引器1": Index对象1,
    "索引器2": Index对象2,

}

示例：
Indices: {
    "namespace": {
        "default": ["default/kube-root-ca.crt", "default/configmap-test1", "default/configmap-test2"], 
        ...
    },
    "labels-test": {
        "label-test2": ["default/configmap-test2"], 
        "label-test": ["default/configmap-test1"], 
    }
    "annotation-test": {
        "annotations-test": ["default/configmap-test1"],
        "annotations-test2": ["default/configmap-test2"],
    }
    ...
}
```   

p.s.：详细代码请参考：demo/examples/indexer/indexinformer_test.go

## local store源码解析

`Indexer` \
定义了两方面的接口：第一类为**存储类型**的接口Store，包含了Add、Update、Delete、List、ListKeys、Get、GetByKey、Replace、Resync等数据存储、读取的常规操作；第二类为**索引类型**的接口，(接口名中包含index)。
```go
type Indexer interface {
	Store
	// 通过indexers[indexName]获得indexFunc，通过indexFunc(obj)获得indexValues
	// 通过Indices[indexName]获得对应的Index，最后返回Index[indexValues]中对应的所有资源对象的key
	// 注意indexValues可以为数组
	Index(indexName string, obj interface{}) ([]interface{}, error)
	// 通过Indices[indexName]获得对应的Index，之后获得Index[indexValues]，
	// 并排序得到有序key集合
	IndexKeys(indexName, indexedValue string) ([]string, error)
	// 获得该IndexName对应的所有Index中的index_key集合
	ListIndexFuncValues(indexName string) []string
	// 返回Index中对应indexedValue的obj集合
	ByIndex(indexName, indexedValue string) ([]interface{}, error)
	// 返回indexers
	GetIndexers() Indexers

	// 添加Indexer
	AddIndexers(newIndexers Indexers) error
}
```
`cache` \
实现了`Indexer`接口，内部定义了`ThreadSafeStore`接口类型的cacheStorage，用来实现基于索引的本地存储。以及`KeyFunc`代表使用的索引值生成方法。
```go
// `*cache` implements Indexer in terms of a ThreadSafeStore and an
// associated KeyFunc.
type cache struct {
	// ThreadSafeStore由 threadSafeMap 实现
	cacheStorage ThreadSafeStore
	//默认使用 MetaNamespaceKeyFunc 也即是key为namespace/name
	keyFunc KeyFunc
}
```

`ThreadSafeStore` \
接口定义了常规的存储、读取、更新接口，以及对于索引的一些接口。 \
注意：添加新的索引`addIndexers`只能在local store还没有启动，也就是还没有数据存储的时候才能够使用。如果local store已经启动，调用该方法会报错。
```go
type ThreadSafeStore interface {
	Add(key string, obj interface{})
	Update(key string, obj interface{})
	Delete(key string)
	Get(key string) (item interface{}, exists bool)
	List() []interface{}
	ListKeys() []string
	Replace(map[string]interface{}, string)
	Index(indexName string, obj interface{}) ([]interface{}, error)
	IndexKeys(indexName, indexKey string) ([]string, error)
	ListIndexFuncValues(name string) []string
	ByIndex(indexName, indexKey string) ([]interface{}, error)
	GetIndexers() Indexers

	// AddIndexers adds more indexers to this store.  If you call this after you already have data
	// in the store, the results are undefined.
	AddIndexers(newIndexers Indexers) error
	// Resync is a no-op and is deprecated
	Resync() error
}
```

`threadSafeMap` \
实现了`ThreadSafeStore`接口，此处为真正实现local store(Indexer)的地方，通过`items`来存储数据、`indexers`来存储索引方法、`indices`来存储索引，实现基于索引的存储。并实现了实现了`ThreadSafeStore`的所有接口。
```go
// threadSafeMap implements ThreadSafeStore
type threadSafeMap struct {
	lock  sync.RWMutex
	items map[string]interface{}

	// indexers maps a name to an IndexFunc
	indexers Indexers
	// indices maps a name to an Index
	indices Indices
}
```
其中最重要的还是理解[重点概念](#重点概念)，并结合示例理解透，这样再去看`threadSafeMap` 内部各种方法的实现就会好理解很多。

## 一些思考
* 如果在local store中已经存在数据，可以再添加新的索引方式indexFunc(indexers)吗？
> 不可以。添加新的索引方式通过函数`AddIndexers`来实现。内部首先判断indexer中是否存在数据(查看其中的items的大小是否为0)，如果存在数据，则返回err，不做任何操作。如果不存在数据，查看当前添加的indexers中的indexName和已存在的indexName是否有重复的，一旦重复就返回err。通过以上两种判断就可以将新的Indexers添加至当前的Indexers中。代码逻辑如下：
```go
func (c *threadSafeMap) AddIndexers(newIndexers Indexers) error {
	c.lock.Lock()
	defer c.lock.Unlock()

	if len(c.items) > 0 {
		return fmt.Errorf("cannot add indexers to running index")
	}

	oldKeys := sets.StringKeySet(c.indexers)
	newKeys := sets.StringKeySet(newIndexers)

	if oldKeys.HasAny(newKeys.List()...) {
		return fmt.Errorf("indexer conflict: %v", oldKeys.Intersection(newKeys))
	}

	for k, v := range newIndexers {
		c.indexers[k] = v
	}
	return nil
}
```

* 如果 indexFunc 返回的 key 列表为空 `[]string{}`，那么对于这个 obj 还会添加到索引中去吗？
> 这种情况表示在此 indexName 建立的索引中不关心这个 obj，所以不会给该 indexName 对应的 Index 的索引中中添加这个 obj 的 key(namespace/name)。

* client 端 watch 到 Api Server 测对象的变更（add/update/delete）之后，是先变更本地数据，还是本地索引？会出现通过索引到拿到 key，但是最终的 obj 已经被删除的情况吗？
> 不会出现这种情况。对于 infromer 机制而言，client 端使用的 Indexer 是「带索引能力的存储」，对于索引和最终数据的变更都会通过一个锁包装成原子操作，所以不会出现通过 Get 查询到索引和数据不一致的情况。
```go
func (c *threadSafeMap) Update(key string, obj interface{}) {
	c.lock.Lock()
	defer c.lock.Unlock()
	oldObject := c.items[key]
	c.items[key] = obj
	c.index.updateIndices(oldObject, obj, key)
}

func (c *threadSafeMap) Get(key string) (item interface{}, exists bool) {
	c.lock.RLock()
	defer c.lock.RUnlock()
	item, exists = c.items[key]
	return item, exists
}
```