`K8s` 作为一个开源项目，鼓励全世界的参与者积极贡献力量，包括 `kubernetes/kubernetes` 主项目、`kubernetes/website`、`kubernetes/enhancements` 等 `K8s` 相关项目都是如此。本文将介绍给 `K8s` 提 `PR` 相关流程、注意事项等。

## 1. 发现 Bug 先提 Issue
首先恭喜你，通过认真仔细阅读 `K8s 源码`([https://github.com/kubernetes/kubernetes](https://github.com/kubernetes/kubernetes))，或在工作实践中偶然遇到了一个 `K8s bug`，第一步应该到官方 `issues`([https://github.com/kubernetes/kubernetes/issues](https://github.com/kubernetes/kubernetes/issues)) 下面查询一下，是否其他人已经提过相关或相同的 `issue` 了，如果没有查到相关 `issue`，那么就可以点击页面右上角 "`New issue`" 创建一个新 `issue`。

![new_issue.png](https://p1-juejin.byteimg.com/tos-cn-i-k3u1fbpfcp/95a703ef73b9429d8eb25f71af9847c9~tplv-k3u1fbpfcp-watermark.image)

然后，选择对应的 `issue` 类型，如果是 `bug` 则选择第一个 "`Bug Report`"：

![bug_report.png](https://p3-juejin.byteimg.com/tos-cn-i-k3u1fbpfcp/ee746afdef694f9d9dad3cb51fbdca26~tplv-k3u1fbpfcp-watermark.image)

接着，就需要填写具体 `bug` 的 `title, content`，可以根据默认模板准确填写如 `What happened, How to reproduce it, Environment`(`kubectl` 版本号、`OS` 类型) 等，尽量清晰、准备描述，可以直接将 `bug `对应的代码文件、行数标记出来，方便 `Reviewer` 快速识别 `issue` 的真伪。


## 2. Fork 代码进行 PR
`PR(Pull Request)` 第一步是 `fork` 一份 `K8s master` 分支代码到自己的个人仓库(`Repo`)，在 `GitHub` 界面上右上角点击 "`Fork`"，选择自己的个人 `GitHub` 账号，稍等几秒就可以看到成功 `fork` 到了自己仓库。

此时，就可以在本地通过 `git clone` 刚刚 `fork` 的 `repo`，一般默认拉下来是 `master` 分支，基于 `master` 分支创建一个新分支，命名清晰达意。然后就可以愉快的进行代码更改，增加相关注释等，修改完毕 `git commit` 即可。

> 注意：`commit message` 尽量清晰达意，不要使用 `@xxx` 特殊符号，末尾不需要加 . 标点符号等。
> 规范参见：[https://github.com/kubernetes/community/blob/master/contributors/guide/pull-requests.md#commit-message-guidelines](https://github.com/kubernetes/community/blob/master/contributors/guide/pull-requests.md#commit-message-guidelines)

## 3. 提交 PR
在个人分支推送到远端 `GitHub` 仓库后，就可以在页面发起 "`New pull request`"，选择个人的更改分支，目标分支是 `Kubernetes/master`，经过代码 "`Compare changes`" 再次确认本地需要 `PR` 的文件、代码，点击确认。

> Tips: `Git commit author` 一定要与 `CLA` 协议(下一步) 一致，否则 `label` 将会显示 `cncf-cla: no`，不能通过后面的 `merge` 校验。

下一步就是需要填写本次 `PR` 相关 `title, content`，建议参考 `content` 模板填写相关的内容项，选择对应的 `sig` 小组，`release-note` 标记等填写完整，否则可能因为必要信息不完整迟迟得不到 `code review`。

![pr_content.png](https://p9-juejin.byteimg.com/tos-cn-i-k3u1fbpfcp/f3eddba5c0554289a08c4108bb81f3cd~tplv-k3u1fbpfcp-watermark.image)

> `K8s PR` 中通过 `label` 来统一管理流程、状态变更。

`PR` 提交后，`k8s-ci-robot` 将会自动新增对应的 `label`，比如 `needs-sig, needs-triage` ，表示需要确认该 `PR` 属于哪个 `SIG(Special Interest Group)`，需要分类等，然后就需要等待相关 `K8s members` 来 `code review`，如果确认是此 `PR` 改动合理, 就会在下面的进行评论如 `/sig api-machinery, /triage accepted` 等，`robot` 收到这样的评论后，就会自动将 `needs-sig, needs-triage label` 去掉，新增评论中对应的 `label`。



## 4. 签 CLA 协议
`CLA(Contributor License Agreement)`：贡献者同意协议，这是参与 `K8s PR` 必须要签署的一个协议，分为个人版、企业版，普通用户选择个人版签订即可。

> 如果已经签过 `CLA` 协议，则在 `https://github.com/kubernetes/` 项目下面的所有项目都会共享协议，即只要在其中任一项目 `PR` 签订了 `CLA` 协议，其他项目都是通用的。

如果是第一次提交 `K8s PR`，则会收到机器人推送的签协议评论，如下：

![cla.png](https://p9-juejin.byteimg.com/tos-cn-i-k3u1fbpfcp/23d598793e6a448f948d7f7d2c3c5ea3~tplv-k3u1fbpfcp-watermark.image)

此时，就需要根据链接指引，去 [https://identity.linuxfoundation.org/](https://identity.linuxfoundation.org/) 签订协议，注册建议选择 `Log in with GitHub` 可以直接获取到 `GitHub username`，与上一步 `PR git commit author` 保持一致。按提示填写相关签约信息后，将收到正式的签约成功邮件，如下：

![cla_email.png](https://p1-juejin.byteimg.com/tos-cn-i-k3u1fbpfcp/b5c688f6ec6f4ce3990dda10bb17847f~tplv-k3u1fbpfcp-watermark.image)

到这一步，刷新 `PR` 页面或等一会查看是否 label 是否变为了 `cncf-cla: yes`，如果等了几个小时还没变更，可以手动评论：`/check-cla`，将会触发机器人重新验证 `CLA` 签约状态，并更新 `label`。

## 5. Reviewer 反馈
一旦 `PR` 提交后，机器人会触发 `label` 标记、`CLA` 验证、分配 `Reviewer`，针对每个 `PR` 一般默认分配两个 `Reviewer`，对应 `Reviewer` 将会收到邮件或 `Slack` 提醒，此时就静静等待他们来 `review` 相关代码改动。

此时，其他 `K8s member` 也可以主动参与此 `PR review`，右上角 `Reviewers` 里面就会看到所有人员，包括机器人默认分配的两个 `Reviewer`，以及其他主动参与的 `Reviewer`。


![reviewers_2.png](https://p3-juejin.byteimg.com/tos-cn-i-k3u1fbpfcp/8cc376a6127445f88a92428a1691636f~tplv-k3u1fbpfcp-watermark.image)

`Reviewer` 可以直接在代码上评论，也可以在最下面写评论，包括一些可以被机器人识别的命令，都是通过 `Comment` 触发的，所以需要仔细看 `Reviewer` 反馈的信息。

## 6. 跟进 Review

`PR` 大多数情况下都不是那么顺利就被 `merge`，`title/content` 描述可能不详细，代码注释不合适等，往往 `Reviewer` 们会给出很多审阅意见、建议，或相关 `PR` 已经有其他人提了，也可能会被否定、不被接受等，此时不要急，需要根据反馈意见修改、优化 `PR`，然后再次提交，此时可以评论 `@Reviewer PTAL` 再次审阅。

如此反复，直到 `PR` 最终被 `Merged` 或 `Closed`(未被采纳)，时间跨度可能快则几天、一周左右，满则几周、几个月都有可能，需要及时跟进、提醒 `review` 进度。



## 7. 代码 Squash
`Reviewer` 审阅觉得代码改动 `ok` 了，此时会看下 `git commit` 是不是已经 `squash`，如果没有则一般会评论提醒 `Author` 进行代码 `Squash`。

因为 `K8s PR` 数量太多，而每个 `PR` 对应 `git commit` 次数可能很多，所以 `K8s PR` 在 `merge` 之前，`Reviewer` 一般会提醒进行代码 `Squash`，将本次 `PR` 所有 `git commit` 合并为一个 `commit`，这样代码合并到主分支后，`git log` 查看的 `git commit` 记录就是一个，大大减小零碎的 `commit` 数量。

`git squash` 操作如下：
```shell
git rebase -i HEAD~3 // 数字表示要合并的 git commit 数量
```
在交互式 `editor` 中，将 `pick` 改为 `squash` 后保存：
```
pick 2ebe926 Original commit
squash 31f33e9 Address feedback
pick b0315fe Second unit of work
```

将会看到：
```
....
Successfully rebased and updated refs/heads/xxxx
```

最后执行 `git push --force` 将本地合并后的 `commit` 强制推送到远端，即完成了 `git squash`。然后就可以再次提醒 `Reviewer` 进行确认。


## 8. 终于等到 Approve
经过上面的 `Review & Squash`，终于得到了 `Reviewer` 的评论 `/lgtm, /approve`，恭喜你，表示此 `PR review` 通过了，这些评论将触发机器人 `merge` 代码到主分支，并标记下一次发版的 `Milestone` 如 `v1.22`。

![lgtm.png](https://p6-juejin.byteimg.com/tos-cn-i-k3u1fbpfcp/4499feae93c847aaa4b3ebca5897fceb~tplv-k3u1fbpfcp-watermark.image)

> 在 `merge` 到主分支之前，机器人会做各种 `CI test`、`check`，确保全部检查项都通过，才会真正 `merge PR` 代码到主分支。

至此，一个 `PR` 经过以上这些步骤，才最终被 `merge` 到主分支，`PR` 状态从 `Open` 变更为 `Merged`。相关联的 `Issues` 将会被机器人自动变更为 `Closed`。 

## 小结
`K8s` 作为一个开源项目，鼓励全世界的参与者积极贡献力量。本文介绍了一个 `K8s PR` 的完整流程，主要包括：提 `Issue`、`Fork` 代码、提交 `PR`、`CLA` 签约、`Review` 跟进、代码 `Squash` 等步骤，如果一切顺利，`PR` 才可能被 `merge` 到主分支。

掌握了以上 `PR` 流程，通过积极参与、贡献 `K8s` 项目，可以获得从 `Author`, `Contributor`, `Member`, `Chair`, `Lead` 的身份转变，为 `K8s` 开源事业贡献一份力。

*PS: 更多文章请关注公众号“稻草人生”*

