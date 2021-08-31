# DeltaFIFO
DeltaFIFO队列在k8s的informer机制中非常重要。简单来说，其是一个生产者消费者队列，拥有FIFO的特性，同时其操作的资源对象为Delta。每一个Delta包含一个操作类型和操作对象。其在informer机制中的位置如下图中所示，它作为了远端（Apiserver、etcd）和本地（indexer和Listener）之间的传输桥梁。
本文首先介绍其数据结构，之后介绍PUSH操作（远端向其中放Delta）、再介绍POP操作（本地处理其中的Deltas）。最后列出自己的思考与现存的疑惑。

![framework.png](https://github.com/NoicFank/picture/raw/main/deltaFIFO/framework.png)


## 数据结构
这部分，我们首先查看源码中对DeltaFIFO的定义，之后结合一个例子介绍一些理解。
```go
type DeltaFIFO struct {
	// 用来对'queue'、'items'、'closed'进行并发控制
	lock sync.RWMutex
	cond sync.Cond
	// key->Deltas的map结构，每一个Deltas包含至少一个Delta
	items map[string]Deltas
	// queue存储items中的key，保证FIFO特性，与items一一对应
	queue []string
	// 标示第一批items已经到达。 貌似是用来判断是否Sync结束，这个还需要再理解具体做什么了～
	populated bool
    // 第一批到达的items的个数，用来判断是否完成Sync（也即是最终减小为了0，就表示初始的同步做完了、FIFO保证处理顺序）
	initialPopulationCount int
	// 生成Obj对应Key的方法
	keyFunc KeyFunc
	// 目前理解为Indexer
	knownObjects KeyListerGetter
	// 标示DeltaFIFO关闭了，会把现有的处理完
	closed bool
	// 为tru表示当执行Replace操作时，状态Type为Replaced。为false时，不启用Replaced状态，使用Sync来代替。这是为了向后兼容。
	emitDeltaTypeReplaced bool
}
```

下面可视化DeltaQueue中最主要的两个存储结构**queue**和**items**。

![deltas-example.png](https://github.com/NoicFank/picture/raw/main/deltaFIFO/deltas-example.png)

如图中所示，总结部分特点如下：
**queue**
* 存储key，对于key的生成方式`keyOf`，默认是取obj的namespace/name，若namespace为空，即直接为name。
* 是“有序”的，用来提供DeltaFIFO中FIFO的特性
* 与items中的key一一对应(正常情况下数量不多不少，刚好对应)
* 其中的key都是唯一的(在函数`queueActionLocked`中实现，该函数向DeltaFIFO添加元素)

**items**
* key与queue中key的生成方式一致
* values中存储的为Deltas数组，同时保证其中必须至少有一个Delta
* 每一个Delta中包含:Type(操作类型)和Obj(对应的对象)，Type的类型如下

Type的类型
* Added ：增加
* Updated：更新
* Deleted：删除
* Replaced：重新list(relist)，这个状态是由于watch event出错，导致需要进行relist来进行全盘同步。需要设置`EmitDeltaTypeReplaced=true`才能显示这个状态，否为默认为Sync。
* Sync：同步

## PUSH操作
PUSH操作具体通过`queueActionLocked`函数来实现，下面说明该函数的步骤：
func (f *DeltaFIFO) queueActionLocked(actionType DeltaType, obj interface{})
1. 通过`KeyOf`计算得到obj对应的key
2. 通过key取items中的元素OldDeltas，同时将当前的delta{DeltaType,Obj}append进去，得到newDeltas (oldDeltas可能为空)
3. 对newDeltas进行去重`dedupDeltas`
3. 如果queue中不存在key，则向queue添加当前的key
4. 更新items[key]为newDeltas
5. 通过sync.cond的`Broadcast`通知所有消费者(POP)开始消费

举个例子：接着上述图片中queue和items的现状，我们现在向其中push一个对Obj2的Update操作，此时结果如下图所示，因为已经存在Obj2-key，所以直接在Obj2-key对应的deltas中添加一个新的delta即可。

![deltas-add-obj2.png](https://github.com/NoicFank/picture/raw/main/deltaFIFO/deltas-add-obj2.png)

如果我们push一个Obj4的Deleted操作，因为此前没有Obj4-key，所以在items中新建对应的条目，同时在queue中添加Obj4-key来排队。

![deltas-add-obj4.png](https://github.com/NoicFank/picture/raw/main/deltaFIFO/deltas-add-obj4.png)

** / 因此DeltasFIFO的思想即是通过queue来实现FIFO，之后通过items来合并同一个Obj在排队期内的所有操作。/ **

## POP操作
1. 取queue中的第一个元素queue[0]，记为id，同时该元素需要出队列。如果队列为空，进入`Wait`，等待生产者进行Broadcast
2. 判断`initialPopulationCount`如果大于0就减1，表示在初始sync阶段
3. 获取items[id]，记为item，并在items中`删除`key为id的项
4. 调用process方法处理item：（该方法具体通过HandleDeltas实现，通过不同的操作类型，对indxer和Listener执行不同的操作）
6. 如果process执行出错，调用`addIfNotPresent`，将id和items[id]，放回queue和items中

**HandleDeltas实现逻辑**
该方法在shared_informer中，其就是循环处理item(Deltas)中的Delta，对于每一个Delta：按照操作类型分类，`Deleted`为一类，剩余操作`Sync, Replaced, Added, Updated`归为另一类：
1. 对于`Deleted`：首先调用indexer的**Delete**方法，在本地存储中删除该Obj。之后调用distribute方法，对所有的Listener进行**deleteNotification**通知删除Obj消息；
2. 对于`Sync, Replaced, Added, Updated`：首先查看在indexer中是否能够get到该Obj：
* 如果可以get：调用indexer的**Update**方法，更新本地存储的Obj，之后调用distribute方法，对所有的Listener进行**updateNotification**通知更新Obj消息；（**注意**：这部分的distribute针对Sync和Replaced只需要通知`syncingListeners`，而不是所有的Listeners。通过distribute方法最后的bool参数来设定，大部分情况设定为false，说明通知所有的Listeners）
* 如果get不到：调用indexer的**Add**方法，在本地存储添加该Obj，之后调用distribute方法，对所有的Listener进行**addNotification**通知添加Obj消息；

## 一些思考
* 为什么使用DeltaFIFO，而不是直接使用一个FIFO？
最重要的就是合并请求。也即是在Queue中的key被不断POP处理的过程中，会有大量同一个Obj的请求到来，这些请求可能散布在整个请求流中，也即是不是连续的。比如下面的例子：在7次请求中，包含4次对Obj 1的请求，请求顺序如下：1->20->1->1->3->5->1,如果直接使用FIFO，那么在处理完第一个1之后，需要处理20，之后又需要处理1的请求，后续同理，这样对Obj 1重复多次做了处理，这不是我们希望的。所以在DeltaFIFO中，我们将这一时间段内对同一个Obj的请求都合并为Deltas，每一次的请求作为其中的一个Delta。这里的一段时间指的是这个Obj对应的key如队列queue开始到出队列的这段时间内。
* Replaced状态表明watch event出现了错误，需要进行relist，这里relist需要和apiServer打交道真的进行一次list操作吗？
No，不会和ApiServer打交道。这里的relist操作是将indexer中的所有存储的Obj拉到DeltaFIFO中一趟，同时对于在items中已经有的元素就不会再重复添加了。这部分体现的是reconcile的思想，也即是将现有状态向目标状态推进。需要知道的是，全局只会和apiServer打交道进行一次list，后续的同步通过watch来保证。此外，如果indexer为空，那么这里的relist不执行任何的操作。

## 一些疑惑
* dedupDelats为什么只挑Deleted状态的进行去重？为什么只需要倒数两个比较去重呢，为了性能考虑吗？那在items中会不会出现<Deleted、Obj>、<Added、Obj>、<Deleted、Obj>的情况？
* DeltaFIFO中的ListKeys方法，返回DeltaFIFO中当前的key列表，源码是遍历items中的key返回对应的key列表。这个实现可以理解，但是当前queue中不是已经存储了key列表了吗？
* Replaced状态添加的意义是？它和Sync在什么情况下需要做区别？目前就看到HandleDeltas中对Sync和Replaced进行了稍微不同的计算，其实默认Replaced为Sync貌似也可以。
* 看注释说Replaced状态是因为watch Event出错了，那出现watch event出错，是不是表明apiServer或etcd测有新的更新，但是没有成功更新到本地。那么在这个状态下，这个relist操作为什么不访问ApiServer，只通过同步indexer就可以重新实现同步呢？

## TODO
* Replace方法还需要进一步理解，哪些Obj需要加Deleted状态。