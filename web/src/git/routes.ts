export function gitCommitPath(project: string, repoID: string, sha: string): string {
  return `/p/${encodeURIComponent(project)}/git/${encodeURIComponent(repoID)}/commit/${encodeURIComponent(sha)}`;
}

export function isCommitQuery(query: string): boolean {
  return /^[0-9a-f]{7,40}$/i.test(query.trim());
}

export function shortSha(sha: string | undefined | null): string {
  const value = (sha ?? "").trim();
  return value.length > 7 ? value.slice(0, 7) : value;
}
