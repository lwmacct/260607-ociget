import {
  CloudServerOutlined,
  ReloadOutlined,
  SearchOutlined,
} from "@ant-design/icons";
import { Alert, AutoComplete, Button, Form, Input, Space, Switch, Typography } from "antd";
import type { ChangeEvent } from "react";
import type { OCISource } from "../model";
import type { Text } from "@/shared/i18n";

interface ImageSourceFormProps {
  objectLoading: boolean;
  draft: OCISource;
  platformError: string | null;
  platformLoading: boolean;
  platformOptions: string[];
  text: Text["browser"];
  onDiscoverPlatforms(): void;
  onOpenObject(): void;
  onUpdateDraft(patch: Partial<OCISource>): void;
}

export function ImageSourceForm({
  objectLoading,
  draft,
  platformError,
  platformLoading,
  platformOptions,
  text,
  onDiscoverPlatforms,
  onOpenObject,
  onUpdateDraft,
}: ImageSourceFormProps) {
  const referenceMissing = !draft.reference.trim();

  return (
    <div className="image-source">
      <div className="image-source__heading">
        <span className="image-source__icon"><CloudServerOutlined /></span>
        <Typography.Title level={4}>{text.imageSource}</Typography.Title>
      </div>

      <Form className="image-source__form" layout="vertical" onFinish={onOpenObject}>
        <Form.Item
          label={text.reference}
          required
          validateStatus={referenceMissing ? "error" : undefined}
          help={referenceMissing ? text.referenceRequired : undefined}
        >
          <Input
            autoComplete="off"
            placeholder={text.referencePlaceholder}
            value={draft.reference}
            onChange={(event: ChangeEvent<HTMLInputElement>) => onUpdateDraft({ reference: event.target.value })}
          />
        </Form.Item>

        <Form.Item label={text.platform}>
          <Space.Compact block>
            <AutoComplete
              className="image-source__platform"
              options={platformOptions.map((value) => ({ label: value, value }))}
              placeholder={text.platformPlaceholder}
              value={draft.platform}
              onChange={(platform: string) => onUpdateDraft({ platform })}
            />
            <Button
              aria-label={text.discoverPlatforms}
              disabled={referenceMissing}
              icon={<ReloadOutlined />}
              loading={platformLoading}
              title={text.discoverPlatforms}
              onClick={onDiscoverPlatforms}
            />
          </Space.Compact>
        </Form.Item>

        <div className="image-source__switch-row">
          <Typography.Text>{text.insecureRegistry}</Typography.Text>
          <Switch
            checked={draft.insecure}
            onChange={(insecure: boolean) => onUpdateDraft({ insecure })}
          />
        </div>

        {platformError ? (
          <Alert
            className="image-source__alert"
            description={platformError}
            showIcon
            title={text.platformFailed}
            type="warning"
          />
        ) : null}

        <Button
          block
          disabled={referenceMissing}
          htmlType="submit"
          icon={<SearchOutlined />}
          loading={objectLoading}
          type="primary"
        >
          {text.openReference}
        </Button>
      </Form>
    </div>
  );
}
