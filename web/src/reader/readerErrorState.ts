export type ReaderErrorKind = "not_found" | "generic";

export function classifyReaderError(message: string): ReaderErrorKind {
  if (/(^|[^0-9])404([^0-9]|$)/.test(message) || /artifact not found/i.test(message)) {
    return "not_found";
  }
  return "generic";
}

export function readerErrorTitleKey(kind: ReaderErrorKind): string {
  return kind === "not_found" ? "wiki.error_not_found_title" : "wiki.error_generic_title";
}

export function shouldShowReaderErrorDevHint(kind: ReaderErrorKind, isDev: boolean): boolean {
  return isDev && kind === "generic";
}
