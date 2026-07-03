// --- Wire types the bridge server / extension / Go client all agree on.
// Field names here are the actual JSON contract — don't rename them without
// updating every consumer (backend/src/types.ts's RPCResponse payloads,
// internal/figma in Go, extensions/browser's background.ts). ---

type PaintSolid = {
  type: "SOLID";
  color: string;
  opacity?: number;
  // Directly-bound Figma Variable, as "Collection/Name" — the binding
  // itself, not the resolved alias chain. Lets a caller know e.g. this
  // #18181b is really Color/Brand/Primary rather than a hardcoded value.
  variable?: string;
  // Designer-set WEB code identifier for that variable (e.g.
  // "--color-brand-primary"), so a caller can emit var(--...) verbatim.
  codeSyntax?: string;
};

type PaintGradient = {
  type:
    | "GRADIENT_LINEAR"
    | "GRADIENT_RADIAL"
    | "GRADIENT_ANGULAR"
    | "GRADIENT_DIAMOND";
  gradientStops: { color: string; opacity: number; position: number }[];
  gradientTransform: Transform;
  opacity?: number;
};

type PaintImage = {
  type: "IMAGE";
  scaleMode: string;
  imageHash?: string | null;
  imageTransform?: Transform;
  opacity?: number;
};

type SerializedPaint = PaintSolid | PaintGradient | PaintImage;

type ShadowEffect = {
  type: "DROP_SHADOW" | "INNER_SHADOW";
  color: string;
  opacity: number;
  offset: { x: number; y: number };
  radius: number;
  spread?: number;
  blendMode: string;
};

type BlurEffect = {
  type: "LAYER_BLUR" | "BACKGROUND_BLUR";
  radius: number;
};

type SerializedEffect = ShadowEffect | BlurEffect;

type AutoLayoutInfo = {
  direction: "HORIZONTAL" | "VERTICAL" | "GRID";
  gap: number;
  primaryAxisAlign: string;
  counterAxisAlign: string;
  primaryAxisSizing: string;
  counterAxisSizing: string;
  wrap?: string;
  counterAxisSpacing?: number;
  // Populated only for direction "GRID" — Figma's native CSS-grid-like
  // layout, mirroring GridTrackSize: FIXED carries a px value, FLEX an
  // `fr`-equivalent, HUG sizes to content.
  gridRowSizes?: { type: "FLEX" | "FIXED" | "HUG"; value?: number }[];
  gridColumnSizes?: { type: "FLEX" | "FIXED" | "HUG"; value?: number }[];
  gridRowGap?: number;
  gridColumnGap?: number;
};

// A direct child of a GRID auto-layout frame's explicit placement in it.
type GridPlacement = {
  rowIndex: number;
  columnIndex: number;
  rowSpan: number;
  columnSpan: number;
};

type SerializedStyles = {
  opacity?: number;
  blendMode?: string;
  visible?: boolean;
  fills?: SerializedPaint[] | "mixed";
  strokes?: SerializedPaint[] | "mixed";
  strokeWeight?: number | "mixed";
  strokeAlign?: string;
  // Only set when per-side weights differ; a uniform weight stays above.
  strokeWeights?: { top: number; right: number; bottom: number; left: number };
  dashPattern?: number[];
  effects?: SerializedEffect[];
  cornerRadius?: number | "mixed";
  cornerRadii?: {
    topLeft: number;
    topRight: number;
    bottomRight: number;
    bottomLeft: number;
  };
  cornerSmoothing?: number;
  autoLayout?: AutoLayoutInfo;
  padding?: { top: number; right: number; bottom: number; left: number };
  clipsContent?: boolean;
  rotation?: number;
  constraints?: { horizontal: string; vertical: string };
  // Auto-layout child escape hatches — omitted entirely for the common/
  // default case (AUTO/INHERIT/0) so they don't add noise to every node.
  layoutPositioning?: "ABSOLUTE";
  layoutGrow?: number;
  layoutAlign?: "STRETCH";
  // Non-paint properties bound to a Variable, as { field: "Collection/Name" }.
  // fills/strokes are intentionally excluded — see PaintSolid.variable, which
  // is per-paint and therefore more precise than a single node-level entry.
  boundVariables?: Record<string, string>;
};

type NodeBounds = { x: number; y: number; width: number; height: number };

type ExportPreset = {
  format: "JPG" | "PNG" | "SVG";
  suffix?: string;
  constraintType?: "SCALE" | "WIDTH" | "HEIGHT";
  constraintValue?: number;
};

// One prototyping reaction. transitionType/easing/duration are only present
// for a NODE-navigation action — real designer-set timing, not a guess.
type SerializedReaction = {
  trigger: string;
  transitionType?: string;
  easing?: string;
  duration?: number;
};

export type SerializedNode = {
  id: string;
  name: string;
  type: string;
  bounds?: NodeBounds;
  characters?: string;
  styles?: SerializedStyles;
  children?: SerializedNode[];
  childCount?: number;
  // INSTANCE component variant property values, e.g. { State: "Hover" }.
  componentProps?: Record<string, string | boolean>;
  // Explicit variable mode overrides, resolved to names.
  variantModes?: Record<string, string>;
  gridPosition?: GridPlacement;
  reactions?: SerializedReaction[];
  // "READY_FOR_DEV" | "COMPLETED" — only ever set on a page/section child.
  devStatus?: string;
  devResources?: { name: string; url: string }[];
  annotations?: string[];
  exportSettings?: ExportPreset[];
};

const isSymbol = (value: unknown): value is symbol => typeof value === "symbol";

function hexFromRgb(color: RGB): string {
  const channel = (v: number) => Math.min(255, Math.max(0, Math.round(v * 255)));
  return (
    "#" +
    [channel(color.r), channel(color.g), channel(color.b)]
      .map((n) => n.toString(16).padStart(2, "0"))
      .join("")
  );
}

function serializeStops(
  stops: readonly ColorStop[]
): { color: string; opacity: number; position: number }[] {
  return stops.map((stop) => ({
    color: hexFromRgb(stop.color),
    opacity: stop.color.a,
    position: stop.position,
  }));
}

// Caches figma.variables.getVariableByIdAsync/getVariableCollectionByIdAsync
// results for one serializeNode() tree walk. Reused components (avatar
// groups, tables, lists) bind many sibling/descendant nodes to the same
// handful of variables; without caching, a library-sourced variable's real
// network-bound resolution repeats per occurrence and a component with a
// few dozen bindings can blow past the bridge's 30s RPC timeout.
export type VariableCache = {
  variables: Map<string, Promise<Variable | null>>;
  collections: Map<string, Promise<VariableCollection | null>>;
};

export function createVariableCache(): VariableCache {
  return { variables: new Map(), collections: new Map() };
}

function cachedVariable(cache: VariableCache, id: string): Promise<Variable | null> {
  const existing = cache.variables.get(id);
  if (existing) return existing;
  const pending = figma.variables.getVariableByIdAsync(id);
  cache.variables.set(id, pending);
  return pending;
}

function cachedCollection(
  cache: VariableCache,
  id: string
): Promise<VariableCollection | null> {
  const existing = cache.collections.get(id);
  if (existing) return existing;
  const pending = figma.variables.getVariableCollectionByIdAsync(id);
  cache.collections.set(id, pending);
  return pending;
}

// Follows a VARIABLE_ALIAS chain (up to 8 hops) to a concrete RGB value.
// Returns null if the chain doesn't resolve to a COLOR variable.
async function followColorAlias(
  alias: VariableAlias,
  cache: VariableCache
): Promise<RGB | null> {
  let currentId = alias.id;
  const MAX_HOPS = 8;
  for (let hop = 0; hop < MAX_HOPS; hop++) {
    const variable = await cachedVariable(cache, currentId);
    if (!variable || variable.resolvedType !== "COLOR") return null;

    const collection = await cachedCollection(cache, variable.variableCollectionId);
    const modeId = collection?.defaultModeId ?? Object.keys(variable.valuesByMode)[0];
    if (!modeId) return null;

    const value = variable.valuesByMode[modeId];
    if (value !== null && typeof value === "object" && "type" in value) {
      const asAlias = value as VariableAlias;
      if (asAlias.type === "VARIABLE_ALIAS") {
        currentId = asAlias.id;
        continue;
      }
    }
    if (value !== null && typeof value === "object" && "r" in value) {
      return value as RGB;
    }
    return null;
  }
  return null;
}

type VariableLabel = {
  label: string;
  // Designer-set WEB code identifier, if any (undefined if never set).
  codeSyntax?: string;
};

// Resolves the *directly bound* variable (not the value it aliases) to a
// "Collection/Name" label — ground truth about the binding itself, distinct
// from whatever value it ultimately resolves to.
async function labelForVariable(
  id: string,
  cache: VariableCache
): Promise<VariableLabel | undefined> {
  const variable = await cachedVariable(cache, id);
  if (!variable) return undefined;
  const collection = await cachedCollection(cache, variable.variableCollectionId);
  return {
    label: collection ? `${collection.name}/${variable.name}` : variable.name,
    codeSyntax: variable.codeSyntax?.WEB,
  };
}

async function serializePaint(
  paint: Paint,
  cache: VariableCache
): Promise<SerializedPaint | undefined> {
  switch (paint.type) {
    case "SOLID": {
      const colorAlias = (paint as SolidPaint).boundVariables?.color;
      let resolvedColor = paint.color;
      let variableLabel: VariableLabel | undefined;
      if (colorAlias) {
        // paint.color can hold the pre-binding default rather than the
        // value actually resolved for the current mode, so re-resolve it.
        const aliasedColor = await followColorAlias(colorAlias, cache);
        if (aliasedColor) resolvedColor = aliasedColor;
        variableLabel = await labelForVariable(colorAlias.id, cache);
      }
      return {
        type: "SOLID",
        color: hexFromRgb(resolvedColor),
        opacity: paint.opacity,
        variable: variableLabel?.label,
        codeSyntax: variableLabel?.codeSyntax,
      };
    }
    case "GRADIENT_LINEAR":
    case "GRADIENT_RADIAL":
    case "GRADIENT_ANGULAR":
    case "GRADIENT_DIAMOND":
      return {
        type: paint.type,
        gradientStops: serializeStops((paint as GradientPaint).gradientStops),
        gradientTransform: (paint as GradientPaint).gradientTransform,
        opacity: paint.opacity,
      };
    case "IMAGE":
      return {
        type: "IMAGE",
        scaleMode: (paint as ImagePaint).scaleMode,
        imageHash: (paint as ImagePaint).imageHash,
        imageTransform: (paint as ImagePaint).imageTransform,
        opacity: paint.opacity,
      };
    default:
      return undefined;
  }
}

async function serializePaintList(
  paints: readonly Paint[] | symbol | undefined,
  cache: VariableCache
): Promise<SerializedPaint[] | "mixed"> {
  if (isSymbol(paints)) return "mixed";
  if (!paints || !Array.isArray(paints)) return [];

  const visible = paints.filter((p) => p.visible !== false);
  const serialized = await Promise.all(visible.map((p) => serializePaint(p, cache)));
  return serialized.filter((p): p is SerializedPaint => p !== undefined);
}

function serializeEffectList(effects: readonly Effect[]): SerializedEffect[] {
  const out: SerializedEffect[] = [];
  for (const effect of effects) {
    if (effect.visible === false) continue;
    if (effect.type === "DROP_SHADOW" || effect.type === "INNER_SHADOW") {
      out.push({
        type: effect.type,
        color: hexFromRgb(effect.color),
        opacity: effect.color.a,
        offset: effect.offset,
        radius: effect.radius,
        spread: effect.spread,
        blendMode: effect.blendMode,
      });
    } else if (effect.type === "LAYER_BLUR" || effect.type === "BACKGROUND_BLUR") {
      out.push({ type: effect.type, radius: effect.radius });
    }
  }
  return out;
}

function serializeLineHeight(lineHeight: LineHeight | symbol) {
  if (isSymbol(lineHeight)) return "mixed";
  return "value" in lineHeight
    ? { value: lineHeight.value, unit: lineHeight.unit }
    : { unit: lineHeight.unit };
}

function serializeLetterSpacing(letterSpacing: LetterSpacing | symbol) {
  if (isSymbol(letterSpacing)) return "mixed";
  return { value: letterSpacing.value, unit: letterSpacing.unit };
}

function nodeBounds(node: SceneNode): NodeBounds | undefined {
  if (!("x" in node && "y" in node && "width" in node && "height" in node)) {
    return undefined;
  }
  return { x: node.x, y: node.y, width: node.width, height: node.height };
}

async function withTextFields(node: TextNode, base: SerializedNode) {
  let fontFamily: string | undefined;
  let fontStyle: string | undefined;
  if (isSymbol(node.fontName)) {
    fontFamily = "mixed";
    fontStyle = "mixed";
  } else if (node.fontName) {
    fontFamily = node.fontName.family;
    fontStyle = node.fontName.style;
  }
  return {
    ...base,
    characters: node.characters,
    styles: {
      ...base.styles,
      fontSize: isSymbol(node.fontSize) ? "mixed" : node.fontSize,
      fontFamily,
      fontStyle,
      fontWeight: isSymbol(node.fontWeight) ? "mixed" : node.fontWeight,
      textDecoration: isSymbol(node.textDecoration) ? "mixed" : node.textDecoration,
      textCase: isSymbol(node.textCase) ? "mixed" : node.textCase,
      lineHeight: serializeLineHeight(node.lineHeight),
      letterSpacing: serializeLetterSpacing(node.letterSpacing),
      textAlignHorizontal: isSymbol(node.textAlignHorizontal)
        ? "mixed"
        : node.textAlignHorizontal,
      textAlignVertical: isSymbol(node.textAlignVertical)
        ? "mixed"
        : node.textAlignVertical,
      textAutoResize: node.textAutoResize,
    },
  };
}

function withGridLayoutFields(node: SceneNode, layout: AutoLayoutInfo): void {
  if (!("gridRowSizes" in node)) return;
  const grid = node as unknown as GridLayoutMixin;
  layout.gridRowSizes = grid.gridRowSizes.map((t) => ({ type: t.type, value: t.value }));
  layout.gridColumnSizes = grid.gridColumnSizes.map((t) => ({
    type: t.type,
    value: t.value,
  }));
  layout.gridRowGap = grid.gridRowGap;
  layout.gridColumnGap = grid.gridColumnGap;
}

function autoLayoutFor(node: SceneNode): AutoLayoutInfo | undefined {
  if (!("layoutMode" in node)) return undefined;
  const mode = node.layoutMode as string;
  if (mode === "NONE") return undefined;

  const frame = node as FrameNode;
  const layout: AutoLayoutInfo = {
    direction: mode as "HORIZONTAL" | "VERTICAL" | "GRID",
    gap: frame.itemSpacing,
    primaryAxisAlign: frame.primaryAxisAlignItems as string,
    counterAxisAlign: frame.counterAxisAlignItems as string,
    primaryAxisSizing: frame.primaryAxisSizingMode as string,
    counterAxisSizing: frame.counterAxisSizingMode as string,
    wrap: "layoutWrap" in node ? (node.layoutWrap as string) : undefined,
    counterAxisSpacing:
      "counterAxisSpacing" in node ? (node.counterAxisSpacing as number) : undefined,
  };

  // GRID is Figma's native CSS-grid-like layout — the designer's explicit
  // row/column setup, ground truth rather than an inferred structure.
  if (mode === "GRID") withGridLayoutFields(node, layout);

  return layout;
}

function uniformOrPerSide(top: number, right: number, bottom: number, left: number) {
  return top === right && right === bottom && bottom === left
    ? undefined
    : { top, right, bottom, left };
}

async function baseStylesFor(node: SceneNode, cache: VariableCache): Promise<SerializedStyles> {
  const styles: SerializedStyles = {};

  if ("opacity" in node) styles.opacity = node.opacity as number;
  if ("blendMode" in node) styles.blendMode = node.blendMode as string;
  if ("visible" in node) styles.visible = node.visible;

  if ("fills" in node) styles.fills = await serializePaintList(node.fills, cache);
  if ("strokes" in node) styles.strokes = await serializePaintList(node.strokes, cache);
  if ("strokeWeight" in node) {
    styles.strokeWeight = isSymbol(node.strokeWeight)
      ? "mixed"
      : (node.strokeWeight as number);
  }
  if ("strokeAlign" in node) styles.strokeAlign = node.strokeAlign as string;
  if ("strokeTopWeight" in node) {
    styles.strokeWeights = uniformOrPerSide(
      node.strokeTopWeight as number,
      node.strokeRightWeight as number,
      node.strokeBottomWeight as number,
      node.strokeLeftWeight as number
    );
  }
  if ("dashPattern" in node) {
    const pattern = node.dashPattern as readonly number[];
    if (pattern.length > 0) styles.dashPattern = [...pattern];
  }

  if ("effects" in node) {
    const effects = node.effects as readonly Effect[];
    if (effects.length > 0) styles.effects = serializeEffectList(effects);
  }

  if ("cornerRadius" in node) {
    styles.cornerRadius = isSymbol(node.cornerRadius)
      ? "mixed"
      : (node.cornerRadius as number);
  }
  if ("topLeftRadius" in node) {
    const perSide = uniformOrPerSide(
      node.topLeftRadius as number,
      node.topRightRadius as number,
      node.bottomRightRadius as number,
      node.bottomLeftRadius as number
    );
    if (perSide) {
      styles.cornerRadii = {
        topLeft: perSide.top,
        topRight: perSide.right,
        bottomRight: perSide.bottom,
        bottomLeft: perSide.left,
      };
    }
  }
  if ("cornerSmoothing" in node) {
    const smoothing = node.cornerSmoothing as number;
    if (smoothing > 0) styles.cornerSmoothing = smoothing;
  }

  const autoLayout = autoLayoutFor(node);
  if (autoLayout) styles.autoLayout = autoLayout;

  if ("paddingLeft" in node) {
    const top = node.paddingTop as number;
    const right = node.paddingRight as number;
    const bottom = node.paddingBottom as number;
    const left = node.paddingLeft as number;
    if (top > 0 || right > 0 || bottom > 0 || left > 0) {
      styles.padding = { top, right, bottom, left };
    }
  }

  if ("clipsContent" in node) styles.clipsContent = node.clipsContent as boolean;
  if ("rotation" in node) {
    const rotation = node.rotation as number;
    if (rotation !== 0) styles.rotation = rotation;
  }
  if ("constraints" in node) {
    const c = node.constraints as Constraints;
    styles.constraints = { horizontal: c.horizontal, vertical: c.vertical };
  }
  if ("layoutPositioning" in node && node.layoutPositioning === "ABSOLUTE") {
    styles.layoutPositioning = "ABSOLUTE";
  }
  if ("layoutGrow" in node && node.layoutGrow > 0) {
    styles.layoutGrow = node.layoutGrow;
  }
  if ("layoutAlign" in node && node.layoutAlign === "STRETCH") {
    styles.layoutAlign = "STRETCH";
  }

  const boundVariables = await boundVariableLabels(node, cache);
  if (boundVariables) styles.boundVariables = boundVariables;

  return styles;
}

// Non-paint/effect/layoutGrid/componentProperty bindings, resolved to
// { field: "Collection/Name" }. Those excluded fields are arrays or maps of
// bindings rather than a single VariableAlias, and fills/strokes are already
// covered per-paint by PaintSolid.variable — more precise than a single
// node-level entry would be.
const NON_SCALAR_BOUND_FIELDS = new Set([
  "fills",
  "strokes",
  "effects",
  "layoutGrids",
  "componentProperties",
]);

async function boundVariableLabels(
  node: SceneNode,
  cache: VariableCache
): Promise<Record<string, string> | undefined> {
  if (!("boundVariables" in node) || !node.boundVariables) return undefined;

  const result: Record<string, string> = {};
  for (const [field, binding] of Object.entries(node.boundVariables)) {
    if (NON_SCALAR_BOUND_FIELDS.has(field)) continue;
    if (!binding || Array.isArray(binding) || typeof binding !== "object") continue;
    const alias = binding as VariableAlias;
    if (alias.type !== "VARIABLE_ALIAS") continue;
    const resolved = await labelForVariable(alias.id, cache);
    if (resolved) result[field] = resolved.label;
  }
  return Object.keys(result).length > 0 ? result : undefined;
}

// Resolves explicitVariableModes (collectionId → modeId) to human-readable
// names, e.g. { "Color Semantic": "Dark" }. Exported so find_nodes (code.ts)
// can resolve this only for nodes that already passed a cheap sync filter.
export async function resolveVariantModes(
  node: SceneNode,
  cache: VariableCache
): Promise<Record<string, string> | undefined> {
  if (!("explicitVariableModes" in node)) return undefined;
  const raw = node.explicitVariableModes as Record<string, string>;
  if (!raw || Object.keys(raw).length === 0) return undefined;

  const result: Record<string, string> = {};
  for (const [collectionId, modeId] of Object.entries(raw)) {
    const collection = await cachedCollection(cache, collectionId);
    const mode = collection?.modes.find((m) => m.modeId === modeId);
    if (mode) result[collection!.name] = mode.name;
  }
  return Object.keys(result).length > 0 ? result : undefined;
}

// Component property values on an INSTANCE, e.g. { State: "Hover", hasIcon: true }.
// Exported for find_nodes (code.ts) — see resolveVariantModes above.
export function resolveComponentProps(
  node: SceneNode
): Record<string, string | boolean> | undefined {
  if (node.type !== "INSTANCE") return undefined;

  let props: InstanceNode["componentProperties"];
  try {
    // Throws for instances of a component set Figma flags as having
    // "existing errors" — a property of the set, not fixable per-node, so
    // skip props for this node instead of aborting the whole subtree walk.
    props = (node as InstanceNode).componentProperties;
  } catch {
    return undefined;
  }
  if (!props) return undefined;

  const result: Record<string, string | boolean> = {};
  for (const [key, prop] of Object.entries(props)) {
    if (prop.type === "VARIANT" || prop.type === "TEXT") {
      result[key] = prop.value as string;
    } else if (prop.type === "BOOLEAN") {
      result[key] = prop.value as boolean;
    }
  }
  return Object.keys(result).length > 0 ? result : undefined;
}

// A node's explicit row/column placement in its parent's GRID auto-layout.
// GridChildrenMixin fields exist on the type for any auto-layout-capable
// node regardless of the parent's actual layoutMode, so this checks the
// parent really is a GRID frame rather than relying on property presence
// alone (an `"x" in node` style check would misfire for FRAME children).
function gridPositionOf(node: SceneNode): GridPlacement | undefined {
  const parent = node.parent;
  if (!parent || !("layoutMode" in parent) || parent.layoutMode !== "GRID") {
    return undefined;
  }
  if (!("gridRowAnchorIndex" in node)) return undefined;
  const grid = node as unknown as GridChildrenMixin;
  return {
    rowIndex: grid.gridRowAnchorIndex,
    columnIndex: grid.gridColumnAnchorIndex,
    rowSpan: grid.gridRowSpan,
    columnSpan: grid.gridColumnSpan,
  };
}

// Trigger + transition timing from a node's prototyping reactions. Only a
// NODE-navigation action carries a transition; other action types (URL,
// SET_VARIABLE, ...) have nothing to animate.
function reactionsOf(node: SceneNode): SerializedReaction[] | undefined {
  if (!("reactions" in node) || node.reactions.length === 0) return undefined;

  const result: SerializedReaction[] = [];
  for (const reaction of node.reactions) {
    if (!reaction.trigger) continue;
    const actions = reaction.actions ?? (reaction.action ? [reaction.action] : []);
    const nodeAction = actions.find(
      (a): a is Action & { type: "NODE" } => a.type === "NODE"
    );
    result.push({
      trigger: reaction.trigger.type,
      transitionType: nodeAction?.transition?.type,
      easing: nodeAction?.transition?.easing.type,
      duration: nodeAction?.transition?.duration,
    });
  }
  return result.length > 0 ? result : undefined;
}

function annotationsOf(node: SceneNode): string[] | undefined {
  if (!("annotations" in node) || node.annotations.length === 0) return undefined;
  const texts = node.annotations
    .map((a) => a.label ?? a.labelMarkdown)
    .filter((t): t is string => !!t);
  return texts.length > 0 ? texts : undefined;
}

function exportSettingsOf(node: SceneNode): ExportPreset[] | undefined {
  if (!("exportSettings" in node) || node.exportSettings.length === 0) return undefined;
  return node.exportSettings.map((s) => ({
    format: s.format as "JPG" | "PNG" | "SVG",
    suffix: "suffix" in s ? s.suffix : undefined,
    constraintType: "constraint" in s && s.constraint ? s.constraint.type : undefined,
    constraintValue: "constraint" in s && s.constraint ? s.constraint.value : undefined,
  }));
}

// Designer-attached dev resource links, fetched lazily.
async function devResourcesOf(
  node: SceneNode
): Promise<{ name: string; url: string }[] | undefined> {
  // getDevResourcesAsync hits Figma's related_links REST endpoint — a real
  // network call, not local document data. Dev Resources are a Dev Mode
  // concept, so this is only worth paying for in Dev Mode: otherwise every
  // find_nodes/get_selection/get_document/get_node call would spend a
  // network round-trip per node on a field nobody in Design mode asked for.
  if (figma.editorType !== "dev") return undefined;
  if (!("getDevResourcesAsync" in node)) return undefined;

  let resources: Awaited<ReturnType<typeof node.getDevResourcesAsync>>;
  try {
    // Can reject for reasons unrelated to the node (e.g. 403 on an unsaved/
    // duplicated file) — optional enrichment, so don't fail the rest of an
    // otherwise-successful response over it.
    resources = await node.getDevResourcesAsync();
  } catch {
    return undefined;
  }
  return resources.length > 0 ? resources.map((r) => ({ name: r.name, url: r.url })) : undefined;
}

// An INSTANCE's main-component family name: the COMPONENT_SET name for a
// variant (e.g. "Button"), or the main component's own name otherwise.
// Deliberately not wired into the generic per-node walk below — that would
// resolve it for every INSTANCE in a whole-tree fetch, even nested ones
// nobody asked about. Used only by the dedicated get_main_component_name
// RPC (code.ts), called once per node actually being matched.
export async function resolveMainComponentName(
  node: SceneNode
): Promise<string | undefined> {
  if (node.type !== "INSTANCE") return undefined;
  try {
    const main = await node.getMainComponentAsync();
    if (!main) return undefined;
    const parent = main.parent;
    return parent?.type === "COMPONENT_SET" ? parent.name : main.name;
  } catch {
    // A main component from an unavailable/unpublished library can reject —
    // this is an optional matching hint, not load-bearing data.
    return undefined;
  }
}

function leanBase(node: SceneNode): SerializedNode {
  return { id: node.id, name: node.name, type: node.type, bounds: nodeBounds(node) };
}

async function fullBase(node: SceneNode, cache: VariableCache): Promise<SerializedNode> {
  return {
    id: node.id,
    name: node.name,
    type: node.type,
    bounds: nodeBounds(node),
    styles: await baseStylesFor(node, cache),
    componentProps: resolveComponentProps(node),
    variantModes: await resolveVariantModes(node, cache),
    gridPosition: gridPositionOf(node),
    reactions: reactionsOf(node),
    devStatus: "devStatus" in node ? node.devStatus?.type : undefined,
    devResources: await devResourcesOf(node),
    annotations: annotationsOf(node),
    exportSettings: exportSettingsOf(node),
  };
}

// Serializes a node (and, recursively, its visible children) into the wire
// shape every consumer (Go, MCP tools, the extension) expects.
//
// maxDepth bounds recursion — 0/undefined means unlimited, matching every
// existing call site. depth is the current node's distance from the call's
// root. Once depth reaches maxDepth, children are reported as childCount
// instead of walked at all: a depth limit, not depth-then-discard, so a huge
// subtree past the limit costs nothing to serialize.
export async function serializeNode(
  node: SceneNode,
  maxDepth?: number,
  depth = 0,
  // Shared for one whole-tree walk — created once at the root call and
  // threaded through recursion. See VariableCache above for why.
  cache: VariableCache = createVariableCache(),
  // Skips every field a structure-only consumer (e.g. the extension's
  // click-to-node hit-map) never reads: no style/variable resolution, no
  // dev-resources network call, no text field serialization. Still returns
  // id/name/type/bounds/children at any depth, for a fraction of the cost.
  lean = false
): Promise<SerializedNode> {
  const base = lean ? leanBase(node) : await fullBase(node, cache);

  if (node.type === "TEXT" && !lean) {
    return withTextFields(node, base);
  }

  if ("children" in node) {
    const visibleChildren = node.children.filter((c) => c.visible !== false);
    if (maxDepth && depth >= maxDepth) {
      return { ...base, childCount: visibleChildren.length };
    }
    const children = await Promise.all(
      visibleChildren.map((child) => serializeNode(child, maxDepth, depth + 1, cache, lean))
    );
    return { ...base, children };
  }

  return base;
}
