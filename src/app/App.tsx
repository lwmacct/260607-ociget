import { useMemo, useState } from "react";
import {
  Alert,
  Breadcrumb,
  Button,
  Checkbox,
  Empty,
  Form,
  Input,
  Layout,
  Space,
  Table,
  Tag,
  Tooltip,
  Typography,
} from "antd";
import type { ColumnsType } from "antd/es/table";
import {
  DownloadOutlined,
  FileOutlined,
  FolderOpenOutlined,
  FolderOutlined,
  ReloadOutlined,
  SearchOutlined,
} from "@ant-design/icons";

type EntryType = "directory" | "file" | "symlink" | "other";

type ImageEntry = {
  name: string;
  path: string;
  type: EntryType;
  size: number;
  mode: number;
  modTime?: string;
  linkName?: string;
};

type DirectoryResponse = {
  path: string;
  entries: ImageEntry[];
};

type BrowseForm = {
  imageRef: string;
  platform: string;
  insecure: boolean;
};

const defaultImage = "alpine:latest";
const defaultPlatform = "linux/amd64";

function normalizePath(path: string) {
  const trimmed = path.trim();
  if (!trimmed || trimmed === "/") return "/";
  return `/${trimmed.replace(/^\/+/, "").replace(/\/+$/, "")}`;
}

function parentPath(path: string) {
  const normalized = normalizePath(path);
  if (normalized === "/") return "/";
  const parts = normalized.split("/").filter(Boolean);
  parts.pop();
  return parts.length ? `/${parts.join("/")}` : "/";
}

function formatSize(size: number, type: EntryType) {
  if (type === "directory") return "-";
  if (size < 0) return "-";
  if (size < 1024) return `${size} B`;
  const units = ["KB", "MB", "GB", "TB"];
  let value = size / 1024;
  let unit = units[0];
  for (let i = 1; i < units.length && value >= 1024; i += 1) {
    value /= 1024;
    unit = units[i];
  }
  return `${value >= 10 ? value.toFixed(1) : value.toFixed(2)} ${unit}`;
}

function formatMode(mode: number) {
  if (!mode) return "-";
  return `0${(mode & 0o7777).toString(8)}`;
}

function formatDate(value?: string) {
  if (!value) return "-";
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return "-";
  if (date.getUTCFullYear() <= 1) return "-";
  return date.toLocaleString();
}

function entryIcon(type: EntryType) {
  if (type === "directory") return <FolderOutlined className="entry-icon directory" />;
  return <FileOutlined className="entry-icon file" />;
}

function downloadHref(imageRef: string, path: string, platform: string, insecure: boolean) {
  const query = new URLSearchParams();
  if (platform.trim()) query.set("platform", platform.trim());
  if (insecure) query.set("insecure", "true");
  const suffix = query.toString();
  return `/download/${imageRef}/-/${path}${suffix ? `?${suffix}` : ""}`;
}

export default function App() {
  const [form] = Form.useForm<BrowseForm>();
  const [imageRef, setImageRef] = useState(defaultImage);
  const [platform, setPlatform] = useState(defaultPlatform);
  const [insecure, setInsecure] = useState(false);
  const [path, setPath] = useState("/");
  const [directory, setDirectory] = useState<DirectoryResponse | null>(null);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const loadDirectory = async (nextPath = path, values?: Partial<BrowseForm>) => {
    const nextImage = values?.imageRef?.trim() || imageRef.trim();
    const nextPlatform = values?.platform?.trim() ?? platform.trim();
    const nextInsecure = values?.insecure ?? insecure;
    const normalizedPath = normalizePath(nextPath);

    setLoading(true);
    setError(null);
    try {
      const query = new URLSearchParams({
        ref: nextImage,
        path: normalizedPath,
      });
      if (nextPlatform) query.set("platform", nextPlatform);
      if (nextInsecure) query.set("insecure", "true");

      const res = await fetch(`/api/images/files?${query.toString()}`);
      if (!res.ok) {
        const body = await res.json().catch(() => null);
        const detail = typeof body?.detail === "string" ? body.detail : `HTTP ${res.status}`;
        throw new Error(detail);
      }

      const data = (await res.json()) as DirectoryResponse;
      setImageRef(nextImage);
      setPlatform(nextPlatform);
      setInsecure(nextInsecure);
      setPath(normalizePath(data.path));
      setDirectory(data);
    } catch (err) {
      setError(err instanceof Error ? err.message : "failed to load image directory");
    } finally {
      setLoading(false);
    }
  };

  const crumbs = useMemo(() => {
    const normalized = normalizePath(path);
    const parts = normalized.split("/").filter(Boolean);
    return [
      { title: "root", path: "/" },
      ...parts.map((part, index) => ({
        title: part,
        path: `/${parts.slice(0, index + 1).join("/")}`,
      })),
    ];
  }, [path]);

  const columns: ColumnsType<ImageEntry> = [
    {
      title: "Name",
      dataIndex: "name",
      ellipsis: true,
      render: (_, entry) => (
        <button
          className={`entry-name ${entry.type === "directory" ? "clickable" : ""}`}
          type="button"
          onClick={() => {
            if (entry.type === "directory") void loadDirectory(entry.path);
          }}
        >
          {entryIcon(entry.type)}
          <span>{entry.name}</span>
          {entry.type === "symlink" && entry.linkName ? <span className="link-target">to {entry.linkName}</span> : null}
        </button>
      ),
    },
    {
      title: "Type",
      dataIndex: "type",
      width: 120,
      render: (type: EntryType) => <Tag>{type}</Tag>,
    },
    {
      title: "Size",
      dataIndex: "size",
      width: 120,
      align: "right",
      render: (size: number, entry) => formatSize(size, entry.type),
    },
    {
      title: "Mode",
      dataIndex: "mode",
      width: 100,
      render: (mode: number) => <Typography.Text code>{formatMode(mode)}</Typography.Text>,
    },
    {
      title: "Modified",
      dataIndex: "modTime",
      width: 210,
      render: (value?: string) => formatDate(value),
    },
    {
      title: "",
      key: "actions",
      width: 72,
      align: "right",
      render: (_, entry) =>
        entry.type === "file" ? (
          <Tooltip title="Download">
            <Button
              aria-label={`Download ${entry.name}`}
              href={downloadHref(imageRef, entry.path, platform, insecure)}
              icon={<DownloadOutlined />}
              size="small"
            />
          </Tooltip>
        ) : null,
    },
  ];

  return (
    <Layout className="app-shell">
      <Layout.Header className="app-header">
        <div className="brand">
          <FolderOpenOutlined />
          <Typography.Title level={4} className="brand-title">
            OCI Image Browser
          </Typography.Title>
        </div>
      </Layout.Header>

      <Layout.Content className="app-content">
        <section className="query-bar">
          <Form<BrowseForm>
            form={form}
            className="query-form"
            initialValues={{ imageRef: defaultImage, platform: defaultPlatform, insecure: false }}
            layout="vertical"
            onFinish={(values) => void loadDirectory("/", values)}
          >
            <Form.Item
              label="Image"
              name="imageRef"
              rules={[{ required: true, message: "Image reference is required" }]}
            >
              <Input placeholder="ghcr.io/org/image:tag" />
            </Form.Item>
            <Form.Item label="Platform" name="platform">
              <Input placeholder="linux/amd64" />
            </Form.Item>
            <Form.Item className="check-item" name="insecure" valuePropName="checked">
              <Checkbox>Insecure registry</Checkbox>
            </Form.Item>
            <Form.Item className="submit-item">
              <Space>
                <Button icon={<SearchOutlined />} loading={loading} type="primary" htmlType="submit">
                  Browse
                </Button>
                <Tooltip title="Reload current directory">
                  <Button icon={<ReloadOutlined />} loading={loading} onClick={() => void loadDirectory(path)} />
                </Tooltip>
              </Space>
            </Form.Item>
          </Form>
        </section>

        {error ? <Alert className="error-alert" type="error" showIcon message={error} /> : null}

        <section className="browser-panel">
          <div className="browser-toolbar">
            <Breadcrumb
              items={crumbs.map((crumb) => ({
                title: (
                  <button className="breadcrumb-button" type="button" onClick={() => void loadDirectory(crumb.path)}>
                    {crumb.title}
                  </button>
                ),
              }))}
            />
            <Space size={8} className="path-actions">
              <Tag>{imageRef}</Tag>
              {platform ? <Tag>{platform}</Tag> : null}
              <Button disabled={path === "/"} size="small" onClick={() => void loadDirectory(parentPath(path))}>
                Up
              </Button>
            </Space>
          </div>

          <Table<ImageEntry>
            className="file-table"
            columns={columns}
            dataSource={directory?.entries ?? []}
            loading={loading}
            locale={{
              emptyText: directory ? <Empty description="Empty directory" /> : <Empty description="Browse an image" />,
            }}
            pagination={false}
            rowKey={(entry) => entry.path}
            scroll={{ x: 860 }}
            size="middle"
          />
        </section>
      </Layout.Content>
    </Layout>
  );
}
