import {
  ArrowUpOutlined,
  DownloadOutlined,
  FolderOpenOutlined,
  ReloadOutlined,
} from "@ant-design/icons";
import { Breadcrumb, Button, Input, Space, Tag, Tooltip } from "antd";
import { useEffect, useMemo, useState, type ChangeEvent } from "react";
import type { ImageBrowserSession } from "../model";
import { imagePathCrumbs, normalizeImagePath, parentImagePath } from "../utils";
import type { Text } from "@/shared/i18n";

interface DirectoryToolbarProps {
  archiveLoading: boolean;
  directoryLoading: boolean;
  selectedCount: number;
  session: ImageBrowserSession;
  text: Text["browser"];
  onDownloadSelected(): void;
  onNavigate(path: string): void;
  onReload(): void;
}

export function DirectoryToolbar({
  archiveLoading,
  directoryLoading,
  selectedCount,
  session,
  text,
  onDownloadSelected,
  onNavigate,
  onReload,
}: DirectoryToolbarProps) {
  const currentPath = session.directory.path;
  const [pathInput, setPathInput] = useState(currentPath);
  const crumbs = useMemo(
    () => imagePathCrumbs(currentPath, text.root),
    [currentPath, text.root],
  );

  useEffect(() => setPathInput(currentPath), [currentPath]);

  function submitPath() {
    onNavigate(normalizeImagePath(pathInput));
  }

  return (
    <div className="directory-toolbar">
      <div className="directory-toolbar__context">
        <Space size={[6, 6]} wrap>
          <Tag icon={<FolderOpenOutlined />} title={session.source.imageRef}>
            {session.source.imageRef}
          </Tag>
          {session.source.platform ? <Tag>{session.source.platform}</Tag> : null}
          {session.source.insecure ? <Tag color="gold">HTTP</Tag> : null}
        </Space>
        <Space className="directory-toolbar__selection" size={8}>
          <span>{text.selected(selectedCount)}</span>
          <Button
            disabled={selectedCount === 0}
            icon={<DownloadOutlined />}
            loading={archiveLoading}
            size="small"
            onClick={onDownloadSelected}
          >
            {text.downloadSelected}
          </Button>
        </Space>
      </div>

      <div className="directory-toolbar__address">
        <Tooltip title={text.up}>
          <Button
            aria-label={text.up}
            disabled={currentPath === "/" || directoryLoading}
            icon={<ArrowUpOutlined />}
            onClick={() => onNavigate(parentImagePath(currentPath))}
          />
        </Tooltip>
        <Input.Search
          aria-label={text.path}
          enterButton
          loading={directoryLoading}
          placeholder={text.pathPlaceholder}
          value={pathInput}
          onChange={(event: ChangeEvent<HTMLInputElement>) => setPathInput(event.target.value)}
          onSearch={submitPath}
        />
        <Tooltip title={text.reload}>
          <Button
            aria-label={text.reload}
            icon={<ReloadOutlined />}
            loading={directoryLoading}
            onClick={onReload}
          />
        </Tooltip>
      </div>

      <Breadcrumb
        className="directory-toolbar__breadcrumbs"
        items={crumbs.map((crumb) => ({
          title: (
            <button
              className="directory-toolbar__crumb"
              disabled={directoryLoading || crumb.path === currentPath}
              type="button"
              onClick={() => onNavigate(crumb.path)}
            >
              {crumb.label}
            </button>
          ),
        }))}
      />
    </div>
  );
}
