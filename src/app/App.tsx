import { useEffect, useMemo, useState } from "react";
import {
  Alert,
  Button,
  Card,
  Layout,
  Menu,
  Space,
  Spin,
  Table,
  Tag,
  Typography,
} from "antd";
import {
  DatabaseOutlined,
  ApiOutlined,
  DashboardOutlined,
  ReloadOutlined,
} from "@ant-design/icons";

type Meta = {
  name: string;
  version: string;
  listen: string;
  database: string;
  docsPath: string;
};

type Health = {
  status: string;
  version: string;
  time: string;
  database: string;
  databaseState: string;
  error?: string;
};

type Endpoint = {
  name: string;
  method: string;
  path: string;
  purpose: string;
};

const endpoints: Endpoint[] = [
  { name: "Metadata", method: "GET", path: "/api/meta", purpose: "service info for the shell" },
  { name: "Health", method: "GET", path: "/api/health", purpose: "liveness and sqlite ping" },
];

const sectionItems = [
  { key: "overview", icon: <DashboardOutlined />, label: "Overview" },
  { key: "api", icon: <ApiOutlined />, label: "API" },
  { key: "storage", icon: <DatabaseOutlined />, label: "Storage" },
];

export default function App() {
  const [section, setSection] = useState("overview");
  const [meta, setMeta] = useState<Meta | null>(null);
  const [health, setHealth] = useState<Health | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  const load = async () => {
    setLoading(true);
    setError(null);
    try {
      const [metaRes, healthRes] = await Promise.all([
        fetch("/api/meta"),
        fetch("/api/health"),
      ]);
      if (!metaRes.ok) {
        throw new Error(`meta request failed: ${metaRes.status}`);
      }
      if (!healthRes.ok) {
        throw new Error(`health request failed: ${healthRes.status}`);
      }
      setMeta((await metaRes.json()) as Meta);
      setHealth((await healthRes.json()) as Health);
    } catch (err) {
      setError(err instanceof Error ? err.message : "failed to load backend data");
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    void load();
  }, []);

  const statusTag = useMemo(() => {
    if (!health) return <Tag>unknown</Tag>;
    return health.status === "ok" ? <Tag color="green">healthy</Tag> : <Tag color="gold">degraded</Tag>;
  }, [health]);

  return (
    <Layout className="app-shell">
      <Layout.Sider className="app-sider" width={240}>
        <div className="brand">
          <Typography.Text className="brand-label">Web App Skeleton</Typography.Text>
          <Typography.Text type="secondary">React + antd + Huma</Typography.Text>
        </div>
        <Menu
          className="nav"
          selectedKeys={[section]}
          mode="inline"
          items={sectionItems}
          onClick={({ key }) => setSection(key)}
        />
      </Layout.Sider>

      <Layout>
        <Layout.Header className="app-header">
          <Space size={12}>
            <Typography.Title level={4} className="header-title">
              {meta?.name ?? "Web App Skeleton"}
            </Typography.Title>
            {statusTag}
          </Space>
          <Button icon={<ReloadOutlined />} onClick={() => void load()} loading={loading}>
            Refresh
          </Button>
        </Layout.Header>

        <Layout.Content className="app-content">
          {error ? <Alert type="error" showIcon message="Backend unavailable" description={error} /> : null}

          {loading && !meta ? (
            <div className="loading-state">
              <Spin size="large" />
            </div>
          ) : (
            <>
              {section === "overview" ? (
                <div className="panel-grid">
                  <Card title="Runtime">
                    <Space direction="vertical" size={8}>
                      <Typography.Text>Version: {meta?.version ?? "-"}</Typography.Text>
                      <Typography.Text>Listen: {meta?.listen ?? "-"}</Typography.Text>
                      <Typography.Text>Docs: {meta?.docsPath ?? "-"}</Typography.Text>
                    </Space>
                  </Card>
                  <Card title="Storage">
                    <Space direction="vertical" size={8}>
                      <Typography.Text>Database: {health?.database ?? meta?.database ?? "-"}</Typography.Text>
                      <Typography.Text>State: {health?.databaseState ?? "-"}</Typography.Text>
                      <Typography.Text>Checked at: {health?.time ?? "-"}</Typography.Text>
                    </Space>
                  </Card>
                  <Card title="API surface" className="wide-card">
                    <Table<Endpoint>
                      rowKey="path"
                      size="small"
                      pagination={false}
                      columns={[
                        { title: "Name", dataIndex: "name" },
                        { title: "Method", dataIndex: "method", width: 100 },
                        { title: "Path", dataIndex: "path" },
                        { title: "Purpose", dataIndex: "purpose" },
                      ]}
                      dataSource={endpoints}
                    />
                  </Card>
                </div>
              ) : null}

              {section === "api" ? (
                <Card title="API Endpoints">
                  <Table<Endpoint>
                    rowKey="path"
                    size="middle"
                    pagination={false}
                    columns={[
                      { title: "Name", dataIndex: "name" },
                      { title: "Method", dataIndex: "method", width: 120 },
                      { title: "Path", dataIndex: "path" },
                      { title: "Purpose", dataIndex: "purpose" },
                    ]}
                    dataSource={endpoints}
                  />
                </Card>
              ) : null}

              {section === "storage" ? (
                <Card title="SQLite">
                  <Space direction="vertical" size={12}>
                    <Typography.Text>Database path: {meta?.database ?? "-"}</Typography.Text>
                    <Typography.Text>Connection state: {health?.databaseState ?? "-"}</Typography.Text>
                    {health?.error ? <Typography.Text type="danger">{health.error}</Typography.Text> : null}
                  </Space>
                </Card>
              ) : null}
            </>
          )}
        </Layout.Content>
      </Layout>
    </Layout>
  );
}

