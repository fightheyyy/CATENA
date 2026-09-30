import { useCallback, useEffect, useRef, useState } from "react";
import { api } from "./api";
import { emptyWorkspace } from "./workspace";
import type { Route } from "./navigation";
import type { EvolutionJob } from "./types";

export function useWorkspace(route: Route, owner: string) {
  const [data, setData] = useState(emptyWorkspace);
  const [loaded, setLoaded] = useState<Set<Route>>(() => new Set());
  const [status, setStatus] = useState({ route, loading: false, error: "", updatedAt: "" });
  const pending = useRef<AbortController | null>(null);

  useEffect(() => {
    setData(emptyWorkspace());
    setLoaded(new Set());
  }, [owner]);

  const refresh = useCallback(async () => {
    pending.current?.abort();
    if (!owner) return;
    const controller = new AbortController();
    pending.current = controller;
    setStatus((current) => ({ ...current, route, loading: true, error: "" }));
    try {
      const patch = await api.workspace(route, controller.signal);
      if (controller.signal.aborted) return;
      setData((current) => ({ ...current, ...patch }));
      setLoaded((current) => new Set(current).add(route));
      setStatus({ route, loading: false, error: "", updatedAt: new Date().toISOString() });
    } catch (cause) {
      if (controller.signal.aborted) return;
      controller.abort();
      setStatus((current) => ({ ...current, route, loading: false,
        error: cause instanceof Error ? cause.message : "Request failed" }));
    }
  }, [owner, route]);

  useEffect(() => {
    void refresh();
    return () => pending.current?.abort();
  }, [refresh]);

  const updateJobs = useCallback((jobs: EvolutionJob[]) => {
    setData((current) => {
      const replacements = new Map(jobs.map((job) => [job.job_id, job]));
      return { ...current, evolutionJobs: [
        ...jobs.filter((job) => !current.evolutionJobs.some((item) => item.job_id === job.job_id)),
        ...current.evolutionJobs.map((job) => replacements.get(job.job_id) ?? job),
      ] };
    });
  }, []);

  const removeJob = useCallback((jobID: string) => {
    setData((current) => ({ ...current, evolutionJobs: current.evolutionJobs.filter((job) => job.job_id !== jobID) }));
  }, []);

  return { data, refresh, updateJobs, removeJob, ready: loaded.has(route),
    loading: status.route !== route || status.loading,
    error: status.route === route ? status.error : "", updatedAt: status.route === route ? status.updatedAt : "" };
}
