import { useWorkbenchLocale } from "@lwmacct/260627-antd-workbench";

const zh = {
  app: {
    sourceCode: "源码",
  },
  browser: {
    imageSource: "OCI 来源",
    reference: "OCI 引用",
    referencePlaceholder: "ghcr.io/org/object:tag",
    referenceRequired: "请输入 OCI 引用",
    platform: "平台",
    platformPlaceholder: "linux/amd64",
    discoverPlatforms: "读取镜像平台",
    insecureRegistry: "允许不安全 Registry",
    openReference: "打开",
    sourceDraftNotice: "修改将在重新打开后生效。",
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
    openAReference: "尚未打开 OCI 引用",
    noPlatforms: "未发现可用平台",
    resolveFailed: "无法读取 OCI 对象",
    platformFailed: "无法读取镜像平台",
    archiveFailed: "无法下载所选文件",
    directory: "目录",
    file: "文件",
    symlink: "符号链接",
    other: "其他",
    ociArtifact: "OCI Artifact",
    artifactType: "Artifact 类型",
    manifestDigest: "Manifest 摘要",
    manifestMediaType: "Manifest 类型",
    mediaType: "媒体类型",
    digest: "摘要",
    copyDigest: "复制摘要",
    digestCopied: "已复制",
    copyDirectLink: "复制 Direct URL",
    directLinkCopied: "Direct URL 已复制",
    directLinkCopyFailed: "无法复制 Direct URL",
    created: "创建时间",
    payloads: "Payload 数量",
    emptyArtifact: "没有可下载的 payload",
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
    imageSource: "OCI source",
    reference: "OCI reference",
    referencePlaceholder: "ghcr.io/org/object:tag",
    referenceRequired: "Enter an OCI reference",
    platform: "Platform",
    platformPlaceholder: "linux/amd64",
    discoverPlatforms: "Discover image platforms",
    insecureRegistry: "Allow insecure registry",
    openReference: "Open",
    sourceDraftNotice: "Changes take effect after reopening.",
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
    openAReference: "No OCI reference open",
    noPlatforms: "No platforms found",
    resolveFailed: "Unable to read the OCI object",
    platformFailed: "Unable to discover image platforms",
    archiveFailed: "Unable to download the selected files",
    directory: "Directory",
    file: "File",
    symlink: "Symbolic link",
    other: "Other",
    ociArtifact: "OCI Artifact",
    artifactType: "Artifact type",
    manifestDigest: "Manifest digest",
    manifestMediaType: "Manifest type",
    mediaType: "Media type",
    digest: "Digest",
    copyDigest: "Copy digest",
    digestCopied: "Copied",
    copyDirectLink: "Copy Direct URL",
    directLinkCopied: "Direct URL copied",
    directLinkCopyFailed: "Unable to copy Direct URL",
    created: "Created",
    payloads: "Payloads",
    emptyArtifact: "No downloadable payloads",
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
