import {
  DownloadOutlined,
  FileOutlined,
  FolderOutlined,
  LinkOutlined,
  QuestionCircleOutlined,
} from "@ant-design/icons";
import { Button, Empty, Table, Tag, Tooltip, Typography } from "antd";
import type { Key, ReactNode } from "react";
import { useMemo } from "react";
import type { ColumnsType } from "antd/es/table";
import type { WorkbenchLocale } from "@lwmacct/260627-antd-workbench";
import type { EntryType, ImageEntry, ImageSource } from "../model";
import { downloadHref, formatFileDate, formatFileMode, formatFileSize } from "../utils";
import type { Text } from "@/shared/i18n";

interface FileTableProps {
  entries: ImageEntry[];
  loading: boolean;
  locale: WorkbenchLocale;
  selectedPaths: string[];
  source: ImageSource;
  text: Text["browser"];
  onNavigate(path: string): void;
  onSelectionChange(paths: string[]): void;
}

export function FileTable({
  entries,
  loading,
  locale,
  selectedPaths,
  source,
  text,
  onNavigate,
  onSelectionChange,
}: FileTableProps) {
  const columns = useMemo<ColumnsType<ImageEntry>>(() => [
    {
      title: text.name,
      dataIndex: "name",
      ellipsis: true,
      render: (_, entry) => (
        <button
          className={`file-entry${entry.type === "directory" ? " file-entry--directory" : ""}`}
          disabled={entry.type !== "directory"}
          type="button"
          onClick={() => onNavigate(entry.path)}
        >
          <span className={`file-entry__icon file-entry__icon--${entry.type}`}>
            {entryIcon(entry.type)}
          </span>
          <span className="file-entry__name">{entry.name}</span>
          {entry.type === "symlink" && entry.linkName ? (
            <span className="file-entry__target">{text.linkTarget(entry.linkName)}</span>
          ) : null}
        </button>
      ),
    },
    {
      title: text.type,
      dataIndex: "type",
      width: 132,
      responsive: ["md"],
      render: (type: EntryType) => <Tag>{entryTypeLabel(type, text)}</Tag>,
    },
    {
      title: text.size,
      dataIndex: "size",
      align: "right",
      width: 118,
      responsive: ["sm"],
      render: (size: number, entry) => formatFileSize(size, entry.type),
    },
    {
      title: text.mode,
      dataIndex: "mode",
      width: 96,
      responsive: ["lg"],
      render: (mode: number) => <Typography.Text code>{formatFileMode(mode)}</Typography.Text>,
    },
    {
      title: text.modified,
      dataIndex: "modTime",
      width: 196,
      responsive: ["xl"],
      render: (value?: string) => formatFileDate(value, locale),
    },
    {
      title: "",
      key: "actions",
      align: "right",
      width: 56,
      render: (_, entry) => entry.type === "file" ? (
        <Tooltip title={text.downloadFile(entry.name)}>
          <Button
            aria-label={text.downloadFile(entry.name)}
            href={downloadHref(source, entry.path)}
            icon={<DownloadOutlined />}
            size="small"
          />
        </Tooltip>
      ) : null,
    },
  ], [locale, onNavigate, source, text]);

  return (
    <Table<ImageEntry>
      className="file-table"
      columns={columns}
      dataSource={entries}
      loading={loading}
      locale={{ emptyText: <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description={text.emptyDirectory} /> }}
      pagination={false}
      rowKey="path"
      rowSelection={{
        selectedRowKeys: selectedPaths,
        onChange: (keys: Key[]) => onSelectionChange(keys.map(String)),
        getCheckboxProps: (entry: ImageEntry) => ({ disabled: entry.type !== "file", name: entry.name }),
      }}
      scroll={{ x: 560 }}
      size="middle"
    />
  );
}

function entryIcon(type: EntryType): ReactNode {
  switch (type) {
    case "directory":
      return <FolderOutlined />;
    case "file":
      return <FileOutlined />;
    case "symlink":
      return <LinkOutlined />;
    case "other":
      return <QuestionCircleOutlined />;
  }
}

function entryTypeLabel(type: EntryType, text: Text["browser"]): string {
  switch (type) {
    case "directory":
      return text.directory;
    case "file":
      return text.file;
    case "symlink":
      return text.symlink;
    case "other":
      return text.other;
  }
}
