import { useCallback, useEffect, useRef, useState } from "react";
import { downloadImageArchive, listImageFiles, listObjectPlatforms, resolveOCIObject } from "../api";
import type { BrowserSession, OCIImageObject, OCISource } from "../model";
import { DEFAULT_OCI_SOURCE } from "../model";
import {
  choosePlatform,
  normalizeImagePath,
  platformValue,
  sourceKey,
} from "../utils";

export function useImageBrowserSession() {
  const [draft, setDraft] = useState<OCISource>({ ...DEFAULT_OCI_SOURCE });
  const [platformOptions, setPlatformOptions] = useState<string[]>([DEFAULT_OCI_SOURCE.platform]);
  const [platformLoading, setPlatformLoading] = useState(false);
  const [platformError, setPlatformError] = useState<string | null>(null);
  const [session, setSession] = useState<BrowserSession | null>(null);
  const [objectLoading, setObjectLoading] = useState(false);
  const [objectError, setObjectError] = useState<string | null>(null);
  const [selectedPaths, setSelectedPaths] = useState<string[]>([]);
  const [archiveLoading, setArchiveLoading] = useState(false);
  const [archiveError, setArchiveError] = useState<string | null>(null);
  const directoryRequest = useRef({ controller: null as AbortController | null, id: 0 });
  const resolveRequest = useRef({ controller: null as AbortController | null, id: 0 });
  const platformRequest = useRef({ controller: null as AbortController | null, id: 0 });

  useEffect(() => () => {
    directoryRequest.current.controller?.abort();
    resolveRequest.current.controller?.abort();
    platformRequest.current.controller?.abort();
  }, []);

  const draftSourceKey = sourceKey(draft);
  useEffect(() => {
    platformRequest.current.controller?.abort();
    setPlatformLoading(false);
    setPlatformError(null);
    setPlatformOptions(draft.platform ? [draft.platform] : []);
  }, [draftSourceKey]);

  const updateDraft = useCallback((patch: Partial<OCISource>) => {
    setDraft((current) => ({ ...current, ...patch }));
  }, []);

  const discoverPlatforms = useCallback(async () => {
    const source = { ...draft, reference: draft.reference.trim() };
    if (!source.reference) return;

    platformRequest.current.controller?.abort();
    const controller = new AbortController();
    const id = platformRequest.current.id + 1;
    platformRequest.current = { controller, id };
    setPlatformLoading(true);
    setPlatformError(null);
    try {
      const platforms = await listObjectPlatforms(source, controller.signal);
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

  const loadDirectory = useCallback(async (
    object: OCIImageObject,
    source: OCISource,
    path: string,
  ) => {
    directoryRequest.current.controller?.abort();
    const controller = new AbortController();
    const id = directoryRequest.current.id + 1;
    directoryRequest.current = { controller, id };
    setObjectLoading(true);
    setObjectError(null);
    setArchiveError(null);
    try {
      const directory = await listImageFiles(object.objectId, normalizeImagePath(path), controller.signal);
      if (directoryRequest.current.id !== id) return false;
      setSession({
        kind: "image",
        source,
        object,
        directory: {
          ...directory,
          path: normalizeImagePath(directory.path),
        },
      });
      setSelectedPaths([]);
      return true;
    } catch (error) {
      if (!isAbortError(error) && directoryRequest.current.id === id) {
        setObjectError(errorMessage(error));
      }
      return false;
    } finally {
      if (directoryRequest.current.id === id) {
        setObjectLoading(false);
      }
    }
  }, []);

  const openObject = useCallback(async () => {
    const source = {
      reference: draft.reference.trim(),
      platform: draft.platform.trim(),
      insecure: draft.insecure,
    };
    if (!source.reference) return false;
    resolveRequest.current.controller?.abort();
    directoryRequest.current.controller?.abort();
    const controller = new AbortController();
    const id = resolveRequest.current.id + 1;
    resolveRequest.current = { controller, id };
    setObjectLoading(true);
    setObjectError(null);
    setArchiveError(null);
    try {
      const object = await resolveOCIObject(source, controller.signal);
      if (resolveRequest.current.id !== id) return false;
      if (object.kind === "artifact") {
        setSession({ kind: "artifact", source, object });
        setSelectedPaths([]);
        return true;
      }
      const directory = await listImageFiles(object.objectId, "/", controller.signal);
      if (resolveRequest.current.id !== id) return false;
      setSession({ kind: "image", source, object, directory });
      setSelectedPaths([]);
      return true;
    } catch (error) {
      if (!isAbortError(error) && resolveRequest.current.id === id) {
        setObjectError(errorMessage(error));
      }
      return false;
    } finally {
      if (resolveRequest.current.id === id) setObjectLoading(false);
    }
  }, [draft]);

  const navigate = useCallback(async (path: string) => {
    if (!session || session.kind !== "image") return false;
    return loadDirectory(session.object, session.source, path);
  }, [loadDirectory, session]);

  const reload = useCallback(async () => {
    if (!session || session.kind !== "image") return false;
    return loadDirectory(session.object, session.source, session.directory.path);
  }, [loadDirectory, session]);

  const downloadSelected = useCallback(async () => {
    if (!session || session.kind !== "image" || selectedPaths.length === 0) return false;
    setArchiveLoading(true);
    setArchiveError(null);
    try {
      await downloadImageArchive(session.object.objectId, selectedPaths);
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
    discoverPlatforms,
    downloadSelected,
    draft,
    navigate,
    objectError,
    objectLoading,
    openObject,
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
