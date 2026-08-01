import {
  WorkbenchCenterState,
  WorkbenchPage,
  WorkbenchSplitWorkspace,
  useWorkbenchLocale,
} from "@lwmacct/260627-antd-workbench";
import { Alert, Empty, Spin } from "antd";
import { DirectoryToolbar } from "./components/DirectoryToolbar";
import { FileTable } from "./components/FileTable";
import { ImageSourceForm } from "./components/ImageSourceForm";
import { useImageBrowserSession } from "./hooks/useImageBrowserSession";
import { useText } from "@/shared/i18n";

export function ImageBrowserPage() {
  const text = useText();
  const { locale } = useWorkbenchLocale();
  const browser = useImageBrowserSession();

  return (
    <WorkbenchSplitWorkspace
      className="image-browser-workspace"
      contentClassName="image-browser-workspace__content"
      sidebar={
        <ImageSourceForm
          directoryLoading={browser.directoryLoading}
          draft={browser.draft}
          platformError={browser.platformError}
          platformLoading={browser.platformLoading}
          platformOptions={browser.platformOptions}
          text={text.browser}
          onDiscoverPlatforms={() => void browser.discoverPlatforms()}
          onOpenImage={() => void browser.openImage()}
          onUpdateDraft={browser.updateDraft}
        />
      }
      sidebarClassName="image-browser-workspace__sidebar"
      sidebarWidth={320}
    >
      <WorkbenchPage className="image-browser-page">
        {browser.directoryError ? (
          <Alert
            closable
            description={browser.directoryError}
            showIcon
            title={text.browser.browseFailed}
            type="error"
          />
        ) : null}
        {browser.archiveError ? (
          <Alert
            closable
            description={browser.archiveError}
            showIcon
            title={text.browser.archiveFailed}
            type="error"
          />
        ) : null}

        {browser.session ? (
          <div className="image-browser-page__browser">
            <DirectoryToolbar
              archiveLoading={browser.archiveLoading}
              directoryLoading={browser.directoryLoading}
              selectedCount={browser.selectedPaths.length}
              session={browser.session}
              text={text.browser}
              onDownloadSelected={() => void browser.downloadSelected()}
              onNavigate={(path) => void browser.navigate(path)}
              onReload={() => void browser.reload()}
            />
            <FileTable
              entries={browser.session.directory.entries}
              loading={browser.directoryLoading}
              locale={locale}
              selectedPaths={browser.selectedPaths}
              source={browser.session.source}
              text={text.browser}
              onNavigate={(path) => void browser.navigate(path)}
              onSelectionChange={browser.setSelectedPaths}
            />
          </div>
        ) : (
          <WorkbenchCenterState>
            {browser.directoryLoading ? (
              <Spin size="large" />
            ) : (
              <Empty
                description={
                  <span>
                    <strong>{text.browser.openAnImage}</strong>
                    <small>{text.browser.openAnImageDescription}</small>
                  </span>
                }
              />
            )}
          </WorkbenchCenterState>
        )}
      </WorkbenchPage>
    </WorkbenchSplitWorkspace>
  );
}
