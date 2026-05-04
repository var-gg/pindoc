import { visit } from "unist-util-visit";

// remarkGithubAlerts upgrades GitHub-style alert blockquotes — a blockquote
// whose first paragraph starts with `[!NOTE]`, `[!TIP]`, `[!IMPORTANT]`,
// `[!WARNING]`, or `[!CAUTION]` — into a flagged blockquote that the React
// renderer turns into an iconified callout. The marker text is stripped so
// it never appears in the rendered output.
//
// We attach className + data-alert via mdast `data.hProperties`, which
// react-markdown forwards onto the rendered <blockquote>. The actual title
// row (icon + localized label) is rendered by the React component override
// in Markdown.tsx so labels can be translated and icons themed.

const ALERT_PATTERN = /^\[!(NOTE|TIP|IMPORTANT|WARNING|CAUTION)\][\t ]*/i;

type MdNode = {
  type: string;
  value?: string;
  children?: MdNode[];
  data?: {
    hName?: string;
    hProperties?: Record<string, unknown>;
  };
};

export function remarkGithubAlerts() {
  return (tree: unknown) => {
    visit(tree as never, "blockquote", (node: MdNode) => {
      const children = node.children;
      if (!children || children.length === 0) return;

      const firstParaIdx = children.findIndex((c) => c.type === "paragraph");
      if (firstParaIdx === -1) return;

      const para = children[firstParaIdx];
      const paraChildren = para.children;
      if (!paraChildren || paraChildren.length === 0) return;

      const head = paraChildren[0];
      if (head.type !== "text" || !head.value) return;

      const match = ALERT_PATTERN.exec(head.value);
      if (!match) return;

      const alertType = match[1].toLowerCase();
      const remainder = head.value.slice(match[0].length);
      const trimmedRemainder = remainder.replace(/^[\t ]+/, "");

      if (trimmedRemainder.length > 0) {
        // [!NOTE] inline title — keep the rest of the line as content.
        head.value = trimmedRemainder;
      } else {
        // [!NOTE] alone on its line — drop the marker text node and any
        // immediate soft break, then trim leading whitespace so the
        // alert body starts cleanly.
        paraChildren.shift();
        if (paraChildren[0]?.type === "break") {
          paraChildren.shift();
        }
        const next = paraChildren[0];
        if (next?.type === "text" && typeof next.value === "string") {
          next.value = next.value.replace(/^[\r\n]+/, "").replace(/^[\t ]+/, "");
          if (!next.value) paraChildren.shift();
        }
        if (paraChildren.length === 0) {
          children.splice(firstParaIdx, 1);
        }
      }

      const data = (node.data = node.data ?? {});
      const existingProps = (data.hProperties ?? {}) as Record<string, unknown>;
      const existingClass = existingProps.className;
      const baseClasses = Array.isArray(existingClass)
        ? existingClass.map(String)
        : typeof existingClass === "string"
          ? existingClass.split(/\s+/).filter(Boolean)
          : [];
      data.hProperties = {
        ...existingProps,
        className: [...baseClasses, "markdown-alert", `markdown-alert-${alertType}`],
        "data-alert": alertType,
      };
    });
  };
}
