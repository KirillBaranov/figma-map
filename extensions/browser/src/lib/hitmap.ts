export interface HitBounds {
  x: number;
  y: number;
  width: number;
  height: number;
}

export interface HitNode {
  id: string;
  name: string;
  bounds: HitBounds;
  children?: HitNode[];
}

function containsPoint(bounds: HitBounds, point: { x: number; y: number }): boolean {
  return (
    point.x >= bounds.x &&
    point.x <= bounds.x + bounds.width &&
    point.y >= bounds.y &&
    point.y <= bounds.y + bounds.height
  );
}

// Resolves the deepest node in the subtree whose bounds contain the given
// point, in absolute Figma canvas coordinates. Among siblings that both
// contain the point, the last one wins — Figma renders later children on
// top, so this matches what the user actually sees.
export function resolveHit(tree: HitNode, point: { x: number; y: number }): HitNode | null {
  if (!containsPoint(tree.bounds, point)) {
    return null;
  }
  const children = tree.children ?? [];
  for (let i = children.length - 1; i >= 0; i--) {
    const hit = resolveHit(children[i], point);
    if (hit) {
      return hit;
    }
  }
  return tree;
}
