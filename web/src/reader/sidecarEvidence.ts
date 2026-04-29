export type RelationLike = {
  relation: string;
  artifact_id?: string;
  slug?: string;
};

export function isEvidenceRelation(relation: string | undefined | null): boolean {
  return (relation ?? "").trim().toLowerCase() === "evidence";
}

export function splitEvidenceEdges<T extends RelationLike>(
  relates: readonly T[],
  relatedBy: readonly T[],
): {
  regularRelates: T[];
  regularRelatedBy: T[];
  evidenceRelates: T[];
  evidenceRelatedBy: T[];
} {
  const regularRelates: T[] = [];
  const regularRelatedBy: T[] = [];
  const evidenceRelates: T[] = [];
  const evidenceRelatedBy: T[] = [];

  for (const edge of relates) {
    (isEvidenceRelation(edge.relation) ? evidenceRelates : regularRelates).push(edge);
  }
  for (const edge of relatedBy) {
    (isEvidenceRelation(edge.relation) ? evidenceRelatedBy : regularRelatedBy).push(edge);
  }
  return { regularRelates, regularRelatedBy, evidenceRelates, evidenceRelatedBy };
}
