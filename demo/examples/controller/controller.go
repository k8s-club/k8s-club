package controller

import (
	"K8s_demo/demo/examples/client"
	"fmt"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/util/workqueue"
	"k8s.io/klog/v2"
	"time"
)

// 控制器
type Controller struct {
	// 支持插入index的本地缓存
	indexer cache.Indexer
	// 工作队列，把监听到的资源放入队列
	queue workqueue.RateLimitingInterface
	// informer控制器
	informer cache.Controller
}

func NewController(queue workqueue.RateLimitingInterface, indexer cache.Indexer, informer cache.Controller) *Controller {
	return &Controller{
		indexer:  indexer,
		queue:    queue,
		informer: informer,
	}
}

// Run 开始 watch 和同步
func (c *Controller) Run(threadNum int, stopC chan struct{}) {
	defer runtime.HandleCrash()

	// 停止控制器后关掉队列
	defer c.queue.ShutDown()

	klog.Info("Controller Started!")

	// 启动informer监听
	go c.informer.Run(stopC)

	// 等待所有相关的缓存同步，然后再开始处理队列中的资源
	if !cache.WaitForCacheSync(stopC, c.informer.HasSynced) {
		runtime.HandleError(fmt.Errorf("%s", "Timed out waiting for caches to sync"))
		return
	}
	// 启动worker数量
	for i := 0; i < threadNum; i++ {
		go wait.Until(c.runWorker, time.Second, stopC)
	}
	<-stopC

	klog.Info("Stopping controller")

}

func (c *Controller) runWorker() {
	for c.processNextItem() {

	}
}

func (c *Controller) processNextItem() bool {
	// 等到工作队列中有一个新元素
	key, quit := c.queue.Get()
	if quit {
		return false
	}
	// 告诉队列已经完成了处理此 key 的操作
	// 这将为其他 worker 解锁该 key
	// 这将确保安全的并行处理，因为永远不会并行处理具有相同 key 的两个Pod
	defer c.queue.Done(key)

	// 调用包含业务逻辑的方法
	err := c.syncToStdout(key.(string))
	c.handleErr(err, key) // 如果业务逻辑有错，需要handle
	return true

}

// syncToStdout 是控制器的业务逻辑实现
// 在此控制器中，它只是将有关 Pod 的信息打印到 stdout
// 如果发生错误，则简单地返回错误
// 此外重试逻辑不应成为业务逻辑的一部分。
func (c *Controller) syncToStdout(key string) error {
	// 从本地存储中获取 key 对应的对象
	obj, exists, err := c.indexer.GetByKey(key)
	if err != nil {
		klog.Errorf("Fetching object with key %s from store failed with %v", key, err)
		return err
	}

	// 如果不存在，打印不存在;如果存在，打印
	if !exists {
		fmt.Printf("Pod %s does not exists anymore\n", key)
	} else {
		fmt.Printf("Sync/Add/Update for Pod %s\n", obj.(*v1.Pod).GetName())
	}

	return nil
}

// handleErr 错误处理：检查是否发生错误，并确保重试次数
func (c *Controller) handleErr(err error, key interface{}) {
	// 忘记每次成功同步时 key 的#AddRateLimited历史记录。
	// 这样可以确保不会因过时的错误历史记录而延迟此 key 更新的以后处理。
	if err == nil {
		c.queue.Forget(key)
		return
	}
	// 如果有问题，重新放入控制器5次
	if c.queue.NumRequeues(key) < 5 {
		klog.Infof("Error syncing pod %v: %v", key, err)
		// 重新加入 key 到限速队列
		// 根据队列上的速率限制器和重新入队历史记录，稍后将再次处理该 key
		c.queue.AddRateLimited(key)
		return
	}
	// 多次重试，也无法成功处理该key
	c.queue.Forget(key)
	runtime.HandleError(err)
	klog.Infof("Dropping pod %q out of the queue: %v", key, err)

}

func main() {
	client := client.ClientSet.Client
	// 创建 资源 ListWatcher
	podListWatcher := cache.NewListWatchFromClient(client.CoreV1().RESTClient(), "pods", v1.NamespaceDefault, fields.Everything())
	// 创建队列
	queue := workqueue.NewRateLimitingQueue(workqueue.DefaultControllerRateLimiter())

	// 在 informer 的帮助下，将工作队列绑定到缓存
	// 这样，我们确保无论何时更新缓存，都将 pod key 添加到工作队列中
	// 注意，当我们最终从工作队列中处理元素时，我们可能会看到 Pod 的版本比响应触发更新的版本新
	indexer, informer := cache.NewIndexerInformer(podListWatcher, &v1.Pod{}, 0, cache.ResourceEventHandlerFuncs{
		// 回调：主要informer监听后，需要放入worker queue队列

		AddFunc: func(obj interface{}) {
			key, err := cache.MetaNamespaceKeyFunc(obj)
			if err != nil {
				klog.Errorf(err.Error())
			}
			queue.Add(key)
		},

		UpdateFunc: func(oldObj, newObj interface{}) {
			key, err := cache.MetaNamespaceKeyFunc(newObj)
			if err != nil {
				klog.Errorf(err.Error())
			}
			queue.Add(key)
		},

		DeleteFunc: func(obj interface{}) {
			key, err := cache.DeletionHandlingMetaNamespaceKeyFunc(obj)
			if err != nil {
				klog.Errorf(err.Error())
			}
			queue.Add(key)
		},
	}, cache.Indexers{})

	// controller
	controller := NewController(queue, indexer, informer)

	_ = indexer.Add(&v1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "mypod",
			Namespace: v1.NamespaceDefault,
		},
	})

	// start controller
	stopCh := make(chan struct{})
	defer close(stopCh)
	// 启动controller
	go controller.Run(1, stopCh)

	select {}

}
