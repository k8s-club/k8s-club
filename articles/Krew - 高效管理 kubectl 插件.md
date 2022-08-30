TOC
- [1. 概述](#1-概述)
- [2. Krew 安装](#2-Krew-安装)
- [3. 管理 kubectl 插件](#3-管理-kubectl-插件)
  - [3.1 常用命令](#31-常用命令)
  - [3.2 Demo](#32-Demo)
- [4. 如何提交新 plugin](#4-如何提交新-plugin)
  - [4.1 编写 plugin](#41-编写-plugin)
  - [4.2 编译打包](#42-编译打包)
  - [4.3 编写 manifest](#43-编写-manifest)
  - [4.4 提交 plugin PR](#44-提交-plugin-PR)
  - [4.5 插件版本更新](#45-插件版本更新)
- [5. 小结](#5-小结)


## 1. 概述
K8s 为方便用户拓展、灵活使用各组件，提供了很多开放性的扩展点，比如 Aggregated APIServer、自定义 Scheduler、CSI、CNI、CRI、kubectl 命令行插件等，让用户可以根据需要，在各层次实现自己的扩展点。

本文将介绍 kubectl 命令行插件的管理工具 - Krew，实现第三方或自定义插件的快速安装、使用、更新、卸载等。其支持了主流的 Linux、Darwin、Windows 平台及其对应的 386、amd64、arm64 指令集架构，可根据用户的系统自动安装对应的平台版本。

> Krew：取名参考了 Mac 中包管理工具 Homebrew (brew)，代表 Kubernetes brew，表示 K8s 中 kubectl 第三方插件工具的包管理。

从 Krew [官网](https://krew.sigs.k8s.io/) 可看到，目前已有超过 200 个第三方提交的插件，可通过 krew 快速安装使用。

## 2. Krew 安装

下载对应平台的版本：

```shell
(
  set -x; cd "$(mktemp -d)" &&
  OS="$(uname | tr '[:upper:]' '[:lower:]')" &&
  ARCH="$(uname -m | sed -e 's/x86_64/amd64/' -e 's/\(arm\)\(64\)\?.*/\1\2/' -e 's/aarch64$/arm64/')" &&
  KREW="krew-${OS}_${ARCH}" &&
  curl -fsSLO "https://github.com/kubernetes-sigs/krew/releases/latest/download/${KREW}.tar.gz" &&
  tar zxvf "${KREW}.tar.gz" &&
  ./"${KREW}" install krew
)
```
添加到 PATH env：
```shell
export PATH="${KREW_ROOT:-$HOME/.krew}/bin:$PATH"
```

重启对应的 shell(bash/zsh) 即可生效，通过 `kubectl krew version` 可验证是否安装成功。

```shell
$ kubectl krew version

OPTION            VALUE
GitTag            v0.4.3
GitCommit         dbfefa5
IndexURI          https://github.com/kubernetes-sigs/krew-index.git
BasePath          /home/dev/.krew
IndexPath         /home/dev/.krew/index/default
InstallPath       /home/dev/.krew/store
BinPath           /home/dev/.krew/bin
DetectedPlatform  linux/amd64
```

> 以上使用 MacOS/Linux - bash/zsh 为例说明安装步骤。其他平台可参考 [官网安装](https://krew.sigs.k8s.io/docs/user-guide/setup/install/) 即可。

## 3. 管理 kubectl 插件
### 3.1 常用命令

使用 `kubectl krew` 可查看 krew 支持的所有子命令，每个子命令都可追加 `--help` 查看对应命令更详细的说明，包括支持的参数、格式等。

```shell
$ kubectl krew

krew is the kubectl plugin manager.
You can invoke krew through kubectl: "kubectl krew [command]..."

Usage:
  kubectl krew [command]

Available Commands:
  completion  generate the autocompletion script for the specified shell
  help        Help about any command
  index       Manage custom plugin indexes
  info        Show information about an available plugin
  install     Install kubectl plugins
  list        List installed kubectl plugins
  search      Discover kubectl plugins
  uninstall   Uninstall plugins
  update      Update the local copy of the plugin index
  upgrade     Upgrade installed plugins to newer versions
  version     Show krew version and diagnostics

Flags:
  -h, --help      help for krew
  -v, --v Level   number for the log level verbosity

Use "kubectl krew [command] --help" for more information about a command.
```

### 3.2 Demo

我们以 kc (kubeconfig manager) 插件为 Demo，进行插件的安装、使用、更新等操作。

安装插件：
```shell
$ kubectl krew install kc

Updated the local copy of plugin index.
Installing plugin: kc


Installed plugin: kc
\
 | Use this plugin:
 | 	kubectl kc
 | Documentation:
 | 	https://github.com/sunny0826/kubecm
/
WARNING: You installed plugin "kc" from the krew-index plugin repository.
   These plugins are not audited for security by the Krew maintainers.
   Run them at your own risk.
```

查看已安装插件：
```shell
$ kubectl krew list

PLUGIN  VERSION
kc      v0.19.3
krew    v0.4.3
```

使用插件：
```shell
$ kubectl kc list

这里将列出当前已添加的 kubeconfig 列表。
```

查看插件信息：
```shell
$ kubectl krew info kc

NAME: kc
INDEX: default
URI: https://github.com/sunny0826/kubecm/releases/download/v0.19.3/kubecm_0.19.3_Linux_x86_64.tar.gz
SHA256: a7cdfd72f8867c38abe7ec0785b92732e02b10d6919564a0f4a7d843ca1ac305
VERSION: v0.19.3
HOMEPAGE: https://github.com/sunny0826/kubecm
DESCRIPTION: 
List, switch, add, delete and more interactive operations to manage kubeconfig.
It also supports kubeconfig management from cloud.
```

更新插件版本：
```shell
$ kubectl krew upgrade kc

Updated the local copy of plugin index.
Upgrading plugin: kc
failed to upgrade plugin "kc": can't upgrade, the newest version is already installed
```

卸载插件：
```shell
$ kubectl krew uninstall kc

Uninstalled plugin: kc
```

## 4. 如何提交新 plugin
### 4.1 编写 plugin
在自己的本地，使用各种语言开发工具，然后构建二进制、可执行脚本文件均可，并支持多种 platform OS + Arch。
> 插件命名：不能与已有插件同名，不能太宽泛，简明达意即可。[命名参考](https://krew.sigs.k8s.io/docs/developer-guide/develop/naming-guide/)

### 4.2 编译打包
将编译的二进制、可执行脚本、License、README 等全部文件，按多种 platform OS + Arch 平台，打包为目标格式 xxx.tar.gz 或 xxx.zip 文件。

### 4.3 编写 manifest
参照 [官方文档](https://krew.sigs.k8s.io/docs/developer-guide/plugin-manifest/) 编写对应插件的 manifest yaml，选择对应的 URI: platform OS + Arch，以及对应打包文件的 sha256 sum 摘要值。

### 4.4 提交 plugin PR
在 [官方插件](https://github.com/kubernetes-sigs/krew-index) 项目中，将上一步编写的 manifest yaml 放到 krew-index/plugins/ 目录，然后提交 PR，等待 Merged 之后用户就可以通过 Krew 进行安装了。

> 可参考 kc 插件 PR：https://github.com/kubernetes-sigs/krew-index/pull/2545

### 4.5 插件版本更新
可配置 GitHub Actions 自动将 `.krew.yaml` 同步到 `krew-index` 仓库，并通过 `krew-release-bot` 自动提交 PR 进行版本更新。

> 可参考 krew-release-bot 配置：https://github.com/rajatjindal/krew-release-bot

## 5. 小结

本文通过介绍 kubectl 命令行插件的管理工具 - Krew，实现第三方或自定义插件的快速安装、使用、更新、卸载等，并通过一个实际的 Demo 演示了一个插件的常用操作命令。小结如下：

- kubectl 无缝融合：与 K8s 官方客户端工具 kubectl 无缝融合，将下载安装的插件，自动部署到 kubectl 插件可执行路径，可通过 kubectl xxx(插件名) 命令可直接使用，方便快捷；
- 快速共享：用户通过 Krew 插件可快速安装使用别人贡献的优秀插件，充分利用了开源社区的自主贡献和共享特性；
- 多平台支持：支持主流的 Linux、Darwin、Windows 平台及其对应的 386、amd64、arm64 指令集架构；
- Git 管理插件：一个与插件同名的 manifest yaml 即表示对应的插件，在 GitHub 项目中实现插件的快速的新增、更新；


*PS: 更多内容请关注 [k8s-club](https://github.com/k8s-club/k8s-club)*


### 参考资料
1. [Krew 官方文档](https://krew.sigs.k8s.io/)
2. [Krew 插件列表](https://krew.sigs.k8s.io/plugins/)
3. [Krew 源码](https://github.com/kubernetes-sigs/krew)
4. [Krew-index 提交插件](https://github.com/kubernetes-sigs/krew-index)
5. [Krew-release-bot](https://github.com/rajatjindal/krew-release-bot)
