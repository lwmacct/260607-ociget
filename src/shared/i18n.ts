import { useWorkbenchLocale } from "@lwmacct/260627-antd-workbench";

const zh = {
  app: {
    sourceCode: "源码",
  },
  browser: {
    imageSource: "镜像源",
    image: "镜像",
    imagePlaceholder: "ghcr.io/org/image:tag",
    imageRequired: "请输入镜像引用",
    platform: "平台",
    platformPlaceholder: "linux/amd64",
    discoverPlatforms: "读取镜像平台",
    insecureRegistry: "允许不安全 Registry",
    openImage: "打开镜像",
    sourceDraftNotice: "修改将在重新打开镜像后生效。",
    activeImage: "当前镜像",
    path: "路径",
    pathPlaceholder: "/usr/local/bin",
    goToPath: "转到路径",
    root: "根目录",
    up: "上一级",
    reload: "重新加载目录",
    name: "名称",
    type: "类型",
    size: "大小",
    mode: "权限",
    modified: "修改时间",
    download: "下载",
    downloadFile(name: string) {
      return `下载 ${name}`;
    },
    selected(count: number) {
      return `已选择 ${count} 项`;
    },
    downloadSelected: "下载所选文件",
    emptyDirectory: "目录为空",
    openAnImage: "打开镜像后浏览文件",
    openAnImageDescription: "在左侧输入镜像引用并选择平台。",
    noPlatforms: "未发现可用平台",
    browseFailed: "无法读取镜像目录",
    platformFailed: "无法读取镜像平台",
    archiveFailed: "无法下载所选文件",
    directory: "目录",
    file: "文件",
    symlink: "符号链接",
    other: "其他",
    linkTarget(target: string) {
      return `指向 ${target}`;
    },
  },
};

const en: typeof zh = {
  app: {
    sourceCode: "Source code",
  },
  browser: {
    imageSource: "Image source",
    image: "Image",
    imagePlaceholder: "ghcr.io/org/image:tag",
    imageRequired: "Enter an image reference",
    platform: "Platform",
    platformPlaceholder: "linux/amd64",
    discoverPlatforms: "Discover image platforms",
    insecureRegistry: "Allow insecure registry",
    openImage: "Open image",
    sourceDraftNotice: "Changes take effect after reopening the image.",
    activeImage: "Active image",
    path: "Path",
    pathPlaceholder: "/usr/local/bin",
    goToPath: "Go to path",
    root: "Root",
    up: "Up one level",
    reload: "Reload directory",
    name: "Name",
    type: "Type",
    size: "Size",
    mode: "Mode",
    modified: "Modified",
    download: "Download",
    downloadFile(name: string) {
      return `Download ${name}`;
    },
    selected(count: number) {
      return `${count} selected`;
    },
    downloadSelected: "Download selected files",
    emptyDirectory: "Empty directory",
    openAnImage: "Open an image to browse files",
    openAnImageDescription: "Enter an image reference and select a platform on the left.",
    noPlatforms: "No platforms found",
    browseFailed: "Unable to read the image directory",
    platformFailed: "Unable to discover image platforms",
    archiveFailed: "Unable to download the selected files",
    directory: "Directory",
    file: "File",
    symlink: "Symbolic link",
    other: "Other",
    linkTarget(target: string) {
      return `to ${target}`;
    },
  },
};

const dictionaries = { "en-US": en, "zh-CN": zh } as const;

export type Text = typeof zh;

export function useText(): Text {
  const { locale } = useWorkbenchLocale();
  return dictionaries[locale];
}
