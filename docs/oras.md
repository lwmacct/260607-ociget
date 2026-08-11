# ORAS 使用指南

ORAS 可以把普通文件作为 OCI Artifact 推送到兼容 OCI Distribution API 的
Registry，例如 GHCR。它适合保存二进制文件、归档、SBOM、签名和其他不需要作为
容器镜像运行的产物，也可以在正确准备 rootfs layer 和 image config 后推送标准 OCI
镜像。

## 普通 Artifact 的最小推送流程

先定义源文件和目标引用，再确认工具与源文件：

```sh
_artifact_file='/path/to/artifact'
_target_ref='ghcr.io/owner/repository:tag'

oras version
ls -lh "$_artifact_file"
file "$_artifact_file"
```

在源文件所在目录执行推送，并显式指定图层的媒体类型：

```sh
_artifact_dir="$(dirname "$_artifact_file")"
_artifact_name="$(basename "$_artifact_file")"

cd "$_artifact_dir"
oras push "$_target_ref" "${_artifact_name}:application/octet-stream"
```

最后一个参数使用的是 `文件路径:媒体类型` 语法。
`application/octet-stream` 是未知或通用二进制文件的稳妥默认值。如果产物已有标准或
团队约定的媒体类型，应优先使用更具体的类型。

## 为什么使用相对路径

ORAS 默认会校验文件路径。直接传入绝对路径会报错：

```text
Error: absolute file path detected. If it's intentional, use --disable-path-validation flag to skip this check
```

推荐切换到文件所在目录，再传相对路径。这样既能通过路径校验，也能让拉取后的文件名
保持简洁：

```sh
cd "$_artifact_dir"
oras push "$_target_ref" "${_artifact_name}:application/octet-stream"
```

`--disable-path-validation` 可以跳过检查，但只有在确实需要保留特殊路径时才应使用。
对于可分发产物，相对路径通常更容易预测，也能减少意外暴露本地目录结构的风险。

## 验证推送结果

推送命令以退出码 0 结束并输出 `Pushed` 后，再解析一次标签：

```sh
oras resolve registry.example.com/team/repository:v1
```

它返回 manifest digest。CI 中应记录该 digest，并在需要不可变引用时使用
`repository@sha256:...`，因为标签可以被重新指向。

查看完整 manifest：

```sh
oras manifest fetch --pretty registry.example.com/team/repository:v1
```

manifest 中可以核对每个 layer 的文件名、媒体类型、大小和 digest。

## 拉取产物

建议拉取到明确的新目录：

```sh
mkdir -p ./oras-output
oras pull \
  --output ./oras-output \
  registry.example.com/team/repository:v1
```

对发布流程或可复现构建，优先按 digest 拉取：

```sh
_manifest_digest="$(oras resolve registry.example.com/team/repository:v1)"
oras pull \
  --output ./oras-output \
  "registry.example.com/team/repository@$_manifest_digest"
```

如果不希望覆盖已有文件，可加 `--keep-old-files`。不要对不受信任的 artifact 使用
`--allow-path-traversal`。

## 多文件和自定义 Artifact Type

一次可以推送多个文件，并分别指定媒体类型：

```sh
_first_file='replace-with-first-relative-path'
_second_file='replace-with-second-relative-path'

oras push "$_target_ref" \
  "${_first_file}:application/octet-stream" \
  "${_second_file}:text/plain"
```

默认情况下，未指定 artifact type 的 OCI 1.1 artifact 可能显示为
`application/vnd.unknown.artifact.v1`。需要让消费者明确识别产物用途时，可设置自定义
artifact type：

```sh
oras push \
  --artifact-type application/vnd.example.release.v1 \
  "$_target_ref" \
  "${_artifact_name}:application/octet-stream"
```

artifact type 描述整个产物，文件后的媒体类型描述单个 layer，两者用途不同。

## Artifact 与标准 OCI 镜像

直接推送普通文件时，文件只是一个 blob，并不是容器运行时可以应用到 rootfs 的 tar
layer。即使设置了 `--artifact-type`，也不会自动把它转换成标准容器镜像。

标准 OCI 镜像至少需要：

- `application/vnd.oci.image.manifest.v1+json` 类型的 manifest。
- `application/vnd.oci.image.config.v1+json` 类型的 image config。
- `application/vnd.oci.image.layer.v1.tar` 或其压缩变体类型的 rootfs layer。
- config 中与未压缩 layer 内容完全对应的 `rootfs.diff_ids`。
- config 中正确的 `os` 和 `architecture`。

能够使用 Docker 或 BuildKit 时，应优先用 Dockerfile 构建。它们会自动处理 tar layer、
diff ID、压缩、历史记录和平台元数据。远程 Docker daemon 不方便使用，或者只需封装
少量已准备好的文件时，可以用 ORAS 手动完成。

### 使用 ORAS 构造标准镜像

下面的脚本接收源文件、目标引用和 `os/architecture` 三个参数。它生成可复现的未压缩
tar layer，计算 diff ID，生成 OCI image config，推送后再验证远端 manifest：

```bash
#!/usr/bin/env bash

set -euo pipefail

__main() {
  if (( $# != 3 )); then
    echo "usage: $0 SOURCE_FILE TARGET_REF OS/ARCHITECTURE" >&2
    return 2
  fi

  local _source_file
  local _target_ref
  local _platform
  local _os
  local _architecture
  local _source_dir
  local _source_name
  local _work_dir
  local _layer_tar
  local _config_json
  local _created
  local _diff_id
  local _created_by

  _source_file="$(realpath "$1")"
  _target_ref="$2"
  _platform="$3"
  _os="${_platform%%/*}"
  _architecture="${_platform#*/}"

  if [[ "$_os" == "$_platform" || "$_architecture" == */* ]]; then
    echo "platform must be OS/ARCHITECTURE" >&2
    return 2
  fi

  _source_dir="$(dirname "$_source_file")"
  _source_name="$(basename "$_source_file")"
  _work_dir="$(mktemp -d)"
  _layer_tar="${_work_dir}/layer.tar"
  _config_json="${_work_dir}/config.json"
  _created='1970-01-01T00:00:00Z'
  _created_by="COPY ${_source_name} /${_source_name}"

  trap 'rm -r -- "$_work_dir"' EXIT

  tar \
    --format=posix \
    --sort=name \
    --mtime="$_created" \
    --owner=0 \
    --group=0 \
    --numeric-owner \
    --pax-option=delete=atime,delete=ctime \
    -C "$_source_dir" \
    -cf "$_layer_tar" \
    "$_source_name"

  _diff_id="sha256:$(sha256sum "$_layer_tar" | cut -d ' ' -f 1)"

  jq -n \
    --arg _created "$_created" \
    --arg _architecture "$_architecture" \
    --arg _os "$_os" \
    --arg _diff_id "$_diff_id" \
    --arg _created_by "$_created_by" \
    '{
      created: $_created,
      architecture: $_architecture,
      os: $_os,
      config: {},
      rootfs: {
        type: "layers",
        diff_ids: [$_diff_id]
      },
      history: [{
        created: $_created,
        created_by: $_created_by
      }]
    }' > "$_config_json"

  (
    cd "$_work_dir"
    oras push \
      --image-spec v1.0 \
      --config config.json:application/vnd.oci.image.config.v1+json \
      "$_target_ref" \
      layer.tar:application/vnd.oci.image.layer.v1.tar
  )

  oras resolve --full-reference "$_target_ref"
  oras manifest fetch "$_target_ref" | jq -e '
    .schemaVersion == 2 and
    .mediaType == "application/vnd.oci.image.manifest.v1+json" and
    .config.mediaType == "application/vnd.oci.image.config.v1+json" and
    (.layers | length == 1) and
    .layers[0].mediaType == "application/vnd.oci.image.layer.v1.tar"
  '

  trap - EXIT
  rm -r -- "$_work_dir"
}

__main "$@"
```

这个配置对应 `FROM scratch` 后只复制一个文件的语义，`config` 为空，因此没有默认
`Entrypoint` 或 `Cmd`。直接运行镜像时需要显式指定层内的文件路径。若要设置默认入口，
应在 image config 的 `config.Entrypoint` 中声明。

示例使用未压缩 tar，因此 layer blob digest 与 diff ID 相同。如果改用
`application/vnd.oci.image.layer.v1.tar+gzip`，manifest 中记录压缩文件的 digest，
而 `rootfs.diff_ids` 仍必须记录压缩前 tar 内容的 digest。

`FROM scratch` 不会提供动态链接器、共享库、Shell、CA 证书或时区数据。放入其中的
可执行文件通常需要静态链接，并且其架构必须与 image config 声明的平台一致。

推送新标签不会删除旧标签。迁移命名方案时，应先验证新引用，再根据保留策略单独处理
旧标签。

## GHCR 认证

如果当前环境已经登录 GHCR，直接执行 `oras push` 即可。不要读取或输出已有认证文件来
确认登录状态。

需要重新登录时，通过标准输入传递 token，避免把 token 直接写进命令参数或 shell
历史：

```sh
printf '%s' "$GHCR_TOKEN" | \
  oras login ghcr.io --username "$GHCR_USER" --password-stdin
```

推送私有仓库时，token 至少需要对目标 package 的写权限。GHCR 还可能受组织 SSO、
package 权限继承和仓库可见性策略影响。

## 实用检查清单

- 推送前确认 Registry、仓库名和标签，避免把产物写到错误位置。
- 对通用二进制文件显式使用 `application/octet-stream`，不要依赖默认 layer 类型。
- 不要把普通文件 blob 当作容器 rootfs layer；标准镜像必须使用 tar layer 和 image
  config。
- 手动制作镜像时，确保 `rootfs.diff_ids` 使用未压缩 layer 的 digest。
- 使用相对路径，使 artifact 内部文件名稳定且不包含本机路径。
- 推送后用 `oras resolve` 保存 manifest digest。
- 发布和部署优先使用 digest；标签适合人类识别，但不是不可变标识。
- 拉取到单独目录，必要时使用 `--keep-old-files` 防止覆盖。
- 认证信息只通过安全的凭据存储或 `--password-stdin` 提供，不打印配置文件或 token。
- 仅对本地测试 Registry 使用 `--plain-http` 或 `--insecure`，不要用于生产环境。
