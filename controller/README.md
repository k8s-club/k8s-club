# 写Controller的一些tips

对象的定义的一些参考和约束:  https://github.com/kubernetes/community/blob/master/contributors/devel/sig-architecture/api-conventions.md

Controller的编写的一些参考和约束: https://github.com/kubernetes/community/blob/master/contributors/design-proposals/architecture/principles.md

> 这两个doc，我认为可以当成圣经去读，**慢慢读，反复写，反复重构**

- 定义好`CRD`或者通过`extension-APIServer`扩展的对象
- 将对象拆分为`spec`和`status`两块，各自负责自己的语义，spec负责描述需要将对象调协到样子(在最早的代码中，使用desriedStateOfWorld描述), status描述当前对象处于什么样的状态
- 了解`ListAndWatch`, `Informer`, `WorkQueue`， 尤其是WorkQueue，是Reconcile逻辑的核心点
