import type { AuthorUserRef } from "../api/client";
import { isGeneratedAgentId } from "./readerInternalVisibility";

export type AuthorDisplaySource = {
  author_id: string;
  author_user?: AuthorUserRef;
  author_version?: string;
};

export function authorAvatarKey(source: AuthorDisplaySource): string {
  return source.author_user?.github_handle
    ?? source.author_user?.display_name
    ?? visibleAgentLabel(source)
    ?? "user";
}

export function authorDisplayLabel(source: AuthorDisplaySource, unknownLabel: string): string {
  return source.author_user?.display_name
    ?? visibleAgentLabel(source)
    ?? unknownLabel;
}

export function visibleAgentLabel(source: AuthorDisplaySource): string {
  const id = source.author_id.trim();
  if (id === "" || isGeneratedAgentId(id)) {
    return "";
  }
  return source.author_version ? `${id}@${source.author_version}` : id;
}

export function authorIdentityKey(source: AuthorDisplaySource): string {
  return source.author_user?.id ?? source.author_id;
}
