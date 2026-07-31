import { GithubOutlined } from "@ant-design/icons";
import {
  WorkbenchAppearanceButton,
  WorkbenchLanguageToggle,
  WorkbenchShell,
} from "@lwmacct/260627-antd-workbench";
import { Button, Tooltip } from "antd";
import { Navigate, Route, Routes } from "react-router-dom";
import { ImageBrowserPage } from "@/modules/image-browser/ImageBrowserPage";
import { DISPLAY_VERSION, SOURCE_URL } from "@/shared/appConfig";
import { useText } from "@/shared/i18n";

export function AppShell() {
  const text = useText();

  return (
    <WorkbenchShell
      brand={{ mark: "O", name: "OCI Get", version: DISPLAY_VERSION }}
      className="single-route-shell"
      flushContent
      nav={[]}
      utilities={
        <>
          <WorkbenchAppearanceButton />
          <WorkbenchLanguageToggle />
          <Tooltip title={text.app.sourceCode}>
            <Button
              aria-label={text.app.sourceCode}
              className="wb-header-action"
              href={SOURCE_URL}
              icon={<GithubOutlined />}
              rel="noopener noreferrer"
              target="_blank"
            />
          </Tooltip>
        </>
      }
      onSelectNav={() => undefined}
    >
      <Routes>
        <Route element={<Navigate replace to="/browser" />} path="/" />
        <Route element={<ImageBrowserPage />} path="/browser" />
        <Route element={<Navigate replace to="/browser" />} path="*" />
      </Routes>
    </WorkbenchShell>
  );
}
