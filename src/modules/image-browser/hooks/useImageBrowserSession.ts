import { useCallback, useEffect, useRef, useState } from "react";
import { downloadImageArchive, listImageFiles, listImagePlatforms } from "../api";
import type { ImageBrowserSession, ImageSource } from "../model";
import { DEFAULT_IMAGE_SOURCE } from "../model";
import {
  choosePlatform,
  normalizeImagePath,
  platformValue,
  sourceKey,
} from "../utils";

export function useImageBrowserSession() {
  const [draft, setDraft] = useState<ImageSource>({ ...DEFAULT_IMAGE_SOURCE });
  const [platformOptions, setPlatformOptions] = useState<string[]>([DEFAULT_IMAGE_SOURCE.platform]);
  const [platformLoading, setPlatformLoading] = useState(false);
  const [platformError, setPlatformError] = useState<string | null>(null);
  const [session, setSession] = useState<ImageBrowserSession | null>(null);
  const [directoryLoading, setDirectoryLoading] = useState(false);
  const [directoryError, setDirectoryError] = useState<string | null>(null);
  const [selectedPaths, setSelectedPaths] = useState<string[]>([]);
  const [archiveLoading, setArchiveLoading] = useState(false);
  const [archiveError, setArchiveError] = useState<string | null>(null);
  const directoryRequest = useRef({ controller: null as AbortController | null, id: 0 });
  const platformRequest = useRef({ controller: null as AbortController | null, id: 0 });

  useEffect(() => () => {
    directoryRequest.current.controller?.abort();
    platformRequest.current.controller?.abort();
  }, []);

  const draftSourceKey = sourceKey(draft);
  useEffect(() => {
    platformRequest.current.controller?.abort();
    setPlatformLoading(false);
    setPlatformError(null);
    setPlatformOptions(draft.platform ? [draft.platform] : []);
  }, [draftSourceKey]);

  const updateDraft = useCallback((patch: Partial<ImageSource>) => {
    setDraft((current) => ({ ...current, ...patch }));
  }, []);

  const discoverPlatforms = useCallback(async () => {
    const source = { ...draft, imageRef: draft.imageRef.trim() };
    if (!source.imageRef) return;

    platformRequest.current.controller?.abort();
    const controller = new AbortController();
    const id = platformRequest.current.id + 1;
    platformRequest.current = { controller, id };
    setPlatformLoading(true);
    setPlatformError(null);
    try {
      const platforms = await listImagePlatforms(source, controller.signal);
      if (platformRequest.current.id !== id) return;
      const values = platforms.map(platformValue).filter(Boolean);
      setPlatformOptions(values);
      const selected = choosePlatform(platforms, source.platform);
      setDraft((current) => sourceKey(current) === sourceKey(source)
        ? { ...current, platform: selected }
        : current);
    } catch (error) {
      if (!isAbortError(error) && platformRequest.current.id === id) {
        setPlatformError(errorMessage(error));
      }
    } finally {
      if (platformRequest.current.id === id) {
        setPlatformLoading(false);
      }
    }
  }, [draft]);

  const loadDirectory = useCallback(async (source: ImageSource, path: string) => {
    directoryRequest.current.controller?.abort();
    const controller = new AbortController();
    const id = directoryRequest.current.id + 1;
    directoryRequest.current = { controller, id };
    setDirectoryLoading(true);
    setDirectoryError(null);
    setArchiveError(null);
    try {
      const directory = await listImageFiles(source, normalizeImagePath(path), controller.signal);
      if (directoryRequest.current.id !== id) return false;
      setSession({
        source,
        directory: {
          ...directory,
          path: normalizeImagePath(directory.path),
        },
      });
      setSelectedPaths([]);
      return true;
    } catch (error) {
      if (!isAbortError(error) && directoryRequest.current.id === id) {
        setDirectoryError(errorMessage(error));
      }
      return false;
    } finally {
      if (directoryRequest.current.id === id) {
        setDirectoryLoading(false);
      }
    }
  }, []);

  const openImage = useCallback(async () => {
    const source = {
      imageRef: draft.imageRef.trim(),
      platform: draft.platform.trim(),
      insecure: draft.insecure,
    };
    if (!source.imageRef) return false;
    return loadDirectory(source, "/");
  }, [draft, loadDirectory]);

  const navigate = useCallback(async (path: string) => {
    if (!session) return false;
    return loadDirectory(session.source, path);
  }, [loadDirectory, session]);

  const reload = useCallback(async () => {
    if (!session) return false;
    return loadDirectory(session.source, session.directory.path);
  }, [loadDirectory, session]);

  const downloadSelected = useCallback(async () => {
    if (!session || selectedPaths.length === 0) return false;
    setArchiveLoading(true);
    setArchiveError(null);
    try {
      await downloadImageArchive(session.source, selectedPaths);
      return true;
    } catch (error) {
      setArchiveError(errorMessage(error));
      return false;
    } finally {
      setArchiveLoading(false);
    }
  }, [selectedPaths, session]);

  return {
    archiveError,
    archiveLoading,
    directoryError,
    directoryLoading,
    discoverPlatforms,
    downloadSelected,
    draft,
    navigate,
    openImage,
    platformError,
    platformLoading,
    platformOptions,
    reload,
    selectedPaths,
    session,
    setSelectedPaths,
    updateDraft,
  };
}

function errorMessage(error: unknown): string {
  return error instanceof Error ? error.message : String(error);
}

function isAbortError(error: unknown): boolean {
  return error instanceof Error && error.name === "AbortError";
}
