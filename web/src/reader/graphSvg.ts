import { visualLabel, visualRelation, visualRelationClass, visualTypeVariant } from "./visualLanguage";

export type GraphPoint = {
  x: number;
  y: number;
};

export const MINI_GRAPH_CENTER: GraphPoint = { x: 140, y: 100 };
export const FULL_GRAPH_VIEWBOX = { width: 1000, height: 640 } as const;

export function graphRadialPositions(
  count: number,
  center: GraphPoint = MINI_GRAPH_CENTER,
  radiusX = 96,
  radiusY = 66,
): GraphPoint[] {
  if (count <= 0) return [];
  const start = -Math.PI / 2;
  return Array.from({ length: count }, (_, i) => {
    const angle = start + (i * 2 * Math.PI) / count;
    return {
      x: Math.round(center.x + Math.cos(angle) * radiusX),
      y: Math.round(center.y + Math.sin(angle) * radiusY),
    };
  });
}

export function graphFullPositions(count: number): GraphPoint[] {
  if (count <= 0) return [];
  const center = { x: FULL_GRAPH_VIEWBOX.width / 2, y: FULL_GRAPH_VIEWBOX.height / 2 };
  if (count === 1) return [center];
  if (count <= 12) {
    return graphRadialPositions(count, center, 360, 220);
  }
  const goldenAngle = Math.PI * (3 - Math.sqrt(5));
  const maxRadiusX = FULL_GRAPH_VIEWBOX.width / 2 - 90;
  const maxRadiusY = FULL_GRAPH_VIEWBOX.height / 2 - 56;
  return Array.from({ length: count }, (_, i) => {
    const progress = Math.sqrt((i + 0.5) / count);
    const angle = i * goldenAngle - Math.PI / 2;
    return {
      x: Math.round(center.x + Math.cos(angle) * maxRadiusX * progress),
      y: Math.round(center.y + Math.sin(angle) * maxRadiusY * progress),
    };
  });
}

export function graphTypeClassSuffix(type: string): string {
  const fallback = type.toLowerCase().replace(/[^a-z0-9]+/g, "") || "default";
  return visualTypeVariant(type) ?? fallback;
}

export function graphRelationClass(relation: string): string {
  return visualRelationClass(relation);
}

export function graphRelationLabel(relation: string, lang: string): string {
  const entry = visualRelation(relation);
  return entry ? visualLabel(entry, lang) : relation;
}
