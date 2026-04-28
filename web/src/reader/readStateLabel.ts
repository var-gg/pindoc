import type { ReadState } from "../api/client";

type TFn = (key: string, ...args: Array<string | number>) => string;

export function readStateLabel(state: ReadState | undefined, t: TFn): string {
  switch (state ?? "unseen") {
    case "unseen":
      return t("reader.read_state.unseen");
    case "glanced":
      return t("reader.read_state.glanced");
    case "read":
      return t("reader.read_state.read");
    case "deeply_read":
      return t("reader.read_state.deeply_read");
  }
}
