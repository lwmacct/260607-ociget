import {
  CloudServerOutlined,
  ReloadOutlined,
  SearchOutlined,
} from "@ant-design/icons";
import { Alert, AutoComplete, Button, Form, Input, Space, Switch, Typography } from "antd";
import type { ChangeEvent } from "react";
import type { ImageSource } from "../model";
import type { Text } from "@/shared/i18n";

interface ImageSourceFormProps {
  directoryLoading: boolean;
  draft: ImageSource;
  platformError: string | null;
  platformLoading: boolean;
  platformOptions: string[];
  text: Text["browser"];
  onDiscoverPlatforms(): void;
  onOpenImage(): void;
  onUpdateDraft(patch: Partial<ImageSource>): void;
}

export function ImageSourceForm({
  directoryLoading,
  draft,
  platformError,
  platformLoading,
  platformOptions,
  text,
  onDiscoverPlatforms,
  onOpenImage,
  onUpdateDraft,
}: ImageSourceFormProps) {
  const imageMissing = !draft.imageRef.trim();

  return (
    <div className="image-source">
      <div className="image-source__heading">
        <span className="image-source__icon"><CloudServerOutlined /></span>
        <Typography.Title level={4}>{text.imageSource}</Typography.Title>
      </div>

      <Form className="image-source__form" layout="vertical" onFinish={onOpenImage}>
        <Form.Item
          label={text.image}
          required
          validateStatus={imageMissing ? "error" : undefined}
          help={imageMissing ? text.imageRequired : undefined}
        >
          <Input
            autoComplete="off"
            placeholder={text.imagePlaceholder}
            value={draft.imageRef}
            onChange={(event: ChangeEvent<HTMLInputElement>) => onUpdateDraft({ imageRef: event.target.value })}
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
              disabled={imageMissing}
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
          disabled={imageMissing}
          htmlType="submit"
          icon={<SearchOutlined />}
          loading={directoryLoading}
          type="primary"
        >
          {text.openImage}
        </Button>
      </Form>
    </div>
  );
}
