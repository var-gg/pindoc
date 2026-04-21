// Tiny fetch wrapper. Keeps response shapes in one place so components
// don't all re-derive types from JSON. Everything is read-only — writes go
// through the MCP path, not here.

export type Project = {
  id: string;
  slug: string;
  name: string;
  description?: string;
  color?: string;
  primary_language: string;
  areas_count: number;
  artifacts_count: number;
  created_at: string;
};

export type Area = {
  id: string;
  slug: string;
  name: string;
  description?: string;
  parent_slug?: string;
  is_cross_cutting: boolean;
  artifact_count: number;
  children_slugs?: string[];
};

export type ArtifactRef = {
  id: string;
  slug: string;
  type: string;
  title: string;
  area_slug: string;
  completeness: string;
  status: string;
  review_state: string;
  author_id: string;
  published_at?: string;
  updated_at: string;
};

export type Artifact = ArtifactRef & {
  body_markdown: string;
  tags: string[];
  author_version?: string;
  superseded_by?: string;
  created_at: string;
};

export type SearchHit = {
  artifact_id: string;
  slug: string;
  type: string;
  title: string;
  area_slug: string;
  heading?: string;
  snippet: string;
  distance: number;
};

const base = "";

async function j<T>(path: string): Promise<T> {
  const res = await fetch(base + path, { headers: { Accept: "application/json" } });
  if (!res.ok) {
    const body = await res.text().catch(() => "");
    throw new Error(`${res.status} ${res.statusText}: ${body.slice(0, 200)}`);
  }
  return res.json() as Promise<T>;
}

export const api = {
  currentProject: () => j<Project>("/api/projects/current"),
  areas: () => j<{ project_slug: string; areas: Area[] }>("/api/areas"),
  artifacts: (params?: { area?: string; type?: string }) => {
    const qs = new URLSearchParams();
    if (params?.area) qs.set("area", params.area);
    if (params?.type) qs.set("type", params.type);
    const q = qs.toString();
    return j<{ project_slug: string; artifacts: ArtifactRef[] }>(
      "/api/artifacts" + (q ? `?${q}` : ""),
    );
  },
  artifact: (idOrSlug: string) =>
    j<Artifact>(`/api/artifacts/${encodeURIComponent(idOrSlug)}`),
  search: (q: string) =>
    j<{ query: string; hits: SearchHit[]; notice?: string }>(
      `/api/search?q=${encodeURIComponent(q)}`,
    ),
};
