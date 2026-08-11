import {
  CopyOutlined,
  DownloadOutlined,
  FileOutlined,
  InboxOutlined,
} from "@ant-design/icons";
import { Button, Descriptions, Empty, Table, Tag, Tooltip, Typography } from "antd";
import { useMemo } from "react";
import type { ColumnsType } from "antd/es/table";
import type { ArtifactBlob, ArtifactSession } from "../model";
import { artifactBlobHref } from "../api";
import { formatFileDate, formatFileSize } from "../utils";
import type { Text } from "@/shared/i18n";

interface ArtifactDetailsProps {
  locale: string;
  session: ArtifactSession;
  text: Text["browser"];
}

const createdAnnotation = "org.opencontainers.image.created";

export function ArtifactDetails({ locale, session, text }: ArtifactDetailsProps) {
  const { object, source } = session;
  const columns = useMemo<ColumnsType<ArtifactBlob>>(() => [
    {
      title: text.name,
      dataIndex: "name",
      ellipsis: true,
      render: (name: string) => (
        <span className="artifact-blob__name">
          <FileOutlined />
          <span>{name}</span>
        </span>
      ),
    },
    {
      title: text.mediaType,
      dataIndex: "mediaType",
      ellipsis: true,
      responsive: ["md"],
      render: (mediaType: string) => <Typography.Text code>{mediaType}</Typography.Text>,
    },
    {
      title: text.size,
      dataIndex: "size",
      align: "right",
      width: 120,
      render: (size: number) => formatFileSize(size, "file"),
    },
    {
      title: text.digest,
      dataIndex: "digest",
      ellipsis: true,
      responsive: ["lg"],
      render: (digest: string) => (
        <Typography.Text
          className="artifact-blob__digest"
          copyable={{ icon: <CopyOutlined />, text: digest, tooltips: [text.copyDigest, text.digestCopied] }}
        >
          {digest}
        </Typography.Text>
      ),
    },
    {
      title: "",
      key: "actions",
      align: "right",
      width: 56,
      render: (_, blob) => (
        <Tooltip title={text.downloadFile(blob.name)}>
          <Button
            aria-label={text.downloadFile(blob.name)}
            href={artifactBlobHref(object.objectId, blob.digest)}
            icon={<DownloadOutlined />}
            size="small"
          />
        </Tooltip>
      ),
    },
  ], [object.objectId, text]);

  return (
    <div className="artifact-view">
      <div className="artifact-view__header">
        <div className="artifact-view__context">
          <Tag icon={<InboxOutlined />} color="cyan">{text.ociArtifact}</Tag>
          <Tag title={source.reference}>{source.reference}</Tag>
          {source.insecure ? <Tag color="gold">HTTP</Tag> : null}
        </div>
        <Descriptions
          className="artifact-view__metadata"
          column={{ xs: 1, sm: 1, md: 2, lg: 2, xl: 3, xxl: 3 }}
          items={[
            { key: "artifactType", label: text.artifactType, children: <Typography.Text code>{object.artifactType}</Typography.Text> },
            { key: "manifestDigest", label: text.manifestDigest, children: <Typography.Text copyable>{object.manifestDigest}</Typography.Text> },
            { key: "created", label: text.created, children: formatFileDate(object.annotations?.[createdAnnotation], locale) },
            { key: "manifestMediaType", label: text.manifestMediaType, children: <Typography.Text code>{object.manifestMediaType}</Typography.Text> },
            { key: "payloads", label: text.payloads, children: object.blobs.length },
          ]}
          size="small"
        />
      </div>
      <Table<ArtifactBlob>
        className="artifact-table"
        columns={columns}
        dataSource={object.blobs}
        locale={{
          emptyText: <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description={text.emptyArtifact} />,
        }}
        pagination={false}
        rowKey={(blob: ArtifactBlob) => `${blob.digest}:${blob.name}`}
        scroll={{ x: 720 }}
        size="middle"
      />
    </div>
  );
}
