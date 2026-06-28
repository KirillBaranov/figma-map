// --- Serialized paint types (discriminated union) ---
type SerializedSolidPaint = {
  type: "SOLID";
  color: string;
  opacity?: number;
  // Set when this paint's color is bound to a Figma Variable — the directly
  // bound variable's "Collection/Name", not the resolved alias chain. Lets a
  // caller know e.g. this #18181b is really Color/Brand/Primary, not a
  // hardcoded value, without guessing.
  variable?: string;
  // The bound variable's designer-set WEB code identifier (e.g.
  // "--color-brand-primary"), if any — lets a caller emit var(--...) instead
  // of the literal color, with zero guessing about the CSS variable's name.
  codeSyntax?: string;
};

type SerializedGradientPaint = {
  type:
    | "GRADIENT_LINEAR"
    | "GRADIENT_RADIAL"
    | "GRADIENT_ANGULAR"
    | "GRADIENT_DIAMOND";
  gradientStops: { color: string; opacity: number; position: number }[];
  gradientTransform: Transform;
  opacity?: number;
};

type SerializedImagePaint = {
  type: "IMAGE";
  scaleMode: string;
  imageHash?: string | null;
  imageTransform?: Transform;
  opacity?: number;
};

type SerializedPaint =
  | SerializedSolidPaint
  | SerializedGradientPaint
  | SerializedImagePaint;

// --- Serialized effect types ---
type SerializedShadowEffect = {
  type: "DROP_SHADOW" | "INNER_SHADOW";
  color: string;
  opacity: number;
  offset: { x: number; y: number };
  radius: number;
  spread?: number;
  blendMode: string;
};

type SerializedBlurEffect = {
  type: "LAYER_BLUR" | "BACKGROUND_BLUR";
  radius: number;
};

type SerializedEffect = SerializedShadowEffect | SerializedBlurEffect;

// --- Serialized auto-layout ---
type SerializedAutoLayout = {
  direction: "HORIZONTAL" | "VERTICAL" | "GRID";
  gap: number;
  primaryAxisAlign: string;
  counterAxisAlign: string;
  primaryAxisSizing: string;
  counterAxisSizing: string;
  wrap?: string;
  counterAxisSpacing?: number;
  // Set only when direction is "GRID" — Figma's native CSS-grid-like
  // auto-layout (rows/columns the designer explicitly defined), not an
  // inferred structure. gridRowSizes/gridColumnSizes mirror GridTrackSize:
  // FIXED tracks carry a px value, FLEX tracks an `fr`-equivalent value, HUG
  // tracks size to content.
  gridRowSizes?: { type: "FLEX" | "FIXED" | "HUG"; value?: number }[];
  gridColumnSizes?: { type: "FLEX" | "FIXED" | "HUG"; value?: number }[];
  gridRowGap?: number;
  gridColumnGap?: number;
};

// Set only on a direct child of a GRID auto-layout frame — its explicit
// row/column placement within the parent's grid.
type SerializedGridPosition = {
  rowIndex: number;
  columnIndex: number;
  rowSpan: number;
  columnSpan: number;
};

// --- Serialized styles ---
type SerializedStyles = {
  opacity?: number;
  blendMode?: string;
  visible?: boolean;
  fills?: SerializedPaint[] | "mixed";
  strokes?: SerializedPaint[] | "mixed";
  strokeWeight?: number | "mixed";
  strokeAlign?: string;
  // Set only when per-side stroke weights differ — uniform weight stays in
  // strokeWeight above.
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
  autoLayout?: SerializedAutoLayout;
  padding?: { top: number; right: number; bottom: number; left: number };
  clipsContent?: boolean;
  rotation?: number;
  constraints?: { horizontal: string; vertical: string };
  // Auto-layout child escape hatches (only emitted for non-default values —
  // "AUTO"/"INHERIT"/0 are the common case and add noise for nothing).
  // layoutPositioning="ABSOLUTE" means this child ignores the parent's flex
  // flow entirely, even though the parent itself is auto-layout.
  layoutPositioning?: "ABSOLUTE";
  layoutGrow?: number;
  layoutAlign?: "STRETCH";
  // Direct (non-paint) property → bound variable "Collection/Name", e.g.
  // { cornerRadius: "Radius/md", itemSpacing: "Spacing/sm" }. fills/strokes
  // are excluded here — see SerializedSolidPaint.variable for those, which is
  // per-paint and therefore more precise than a node-level entry.
  boundVariables?: Record<string, string>;
};

type SerializedBounds = {
  x: number;
  y: number;
  width: number;
  height: number;
};

export type SerializedNode = {
  id: string;
  name: string;
  type: string;
  bounds?: SerializedBounds;
  characters?: string;
  styles?: SerializedStyles;
  children?: SerializedNode[];
  childCount?: number;
  // Component variant property values (INSTANCE nodes), e.g. { "State": "Hover", "Size": "M" }
  componentProps?: Record<string, string | boolean>;
  // Explicit variable mode overrides resolved to names, e.g. { "Color Semantic": "Dark" }
  variantModes?: Record<string, string>;
  // This node's explicit position within its parent's GRID auto-layout, if
  // the parent uses one. See SerializedAutoLayout's GRID fields.
  gridPosition?: SerializedGridPosition;
  // Prototyping reactions (click/hover → navigate/animate), if any are
  // configured on this node. Ground truth for interactive-state timing —
  // see SerializedReaction.
  reactions?: SerializedReaction[];
  // "READY_FOR_DEV" | "COMPLETED" — only settable on a node directly under a
  // page or section. A discovery filter: which frames are actually ready to
  // build, not a guess about intent.
  devStatus?: string;
  // Designer-attached links (name + url), surfaced as-is — a node-level
  // analogue of a Variable's codeSyntax: a human-given pointer to code/docs.
  devResources?: { name: string; url: string }[];
  // Designer notes/instructions on this node, free text — never auto-applied,
  // but a strong human-given hint an agent should read.
  annotations?: string[];
  // Designer-defined export presets (format/suffix/scale), in the order
  // Figma stores them. `export-assets` defaults to the first one instead of
  // guessing format/scale, when present.
  exportSettings?: SerializedExportSetting[];
};

type SerializedExportSetting = {
  format: "JPG" | "PNG" | "SVG";
  suffix?: string;
  constraintType?: "SCALE" | "WIDTH" | "HEIGHT";
  constraintValue?: number;
};

// One prototyping reaction: what triggers it, and (when the action is a NODE
// navigation with a transition) the transition's type/easing/duration —
// real, designer-set timing for hover/click states, not a guess.
type SerializedReaction = {
  trigger: string;
  transitionType?: string;
  easing?: string;
  duration?: number;
};

const isMixed = (value: unknown): value is symbol => typeof value === "symbol";

const toHex = (color: RGB): string => {
  const clamp = (value: number) =>
    Math.min(255, Math.max(0, Math.round(value * 255)));
  const [r, g, b] = [clamp(color.r), clamp(color.g), clamp(color.b)];
  return `#${[r, g, b].map((v) => v.toString(16).padStart(2, "0")).join("")}`;
};

const serializeGradientStops = (
  stops: readonly ColorStop[]
): { color: string; opacity: number; position: number }[] =>
  stops.map((stop) => ({
    color: toHex(stop.color),
    opacity: stop.color.a,
    position: stop.position,
  }));

// Memoizes figma.variables.getVariableByIdAsync/getVariableCollectionByIdAsync
// for the lifetime of a single serializeNode() tree walk. Components designers
// actually reuse (avatar groups, lists, tables) bind many sibling/descendant
// nodes to the *same* handful of variables — without this, each occurrence
// re-resolves the same variable/collection from scratch, and on a
// library-sourced variable that resolution is a real network call, so a
// component with a few dozen repeated bindings can blow well past the
// bridge's 30s timeout despite having a tiny tree.
export type VariableCache = {
  variables: Map<string, Promise<Variable | null>>;
  collections: Map<string, Promise<VariableCollection | null>>;
};

export const createVariableCache = (): VariableCache => ({
  variables: new Map(),
  collections: new Map(),
});

const getVariableCached = (
  cache: VariableCache,
  id: string
): Promise<Variable | null> => {
  let p = cache.variables.get(id);
  if (!p) {
    p = figma.variables.getVariableByIdAsync(id);
    cache.variables.set(id, p);
  }
  return p;
};

const getCollectionCached = (
  cache: VariableCache,
  id: string
): Promise<VariableCollection | null> => {
  let p = cache.collections.get(id);
  if (!p) {
    p = figma.variables.getVariableCollectionByIdAsync(id);
    cache.collections.set(id, p);
  }
  return p;
};

// Resolves a Figma variable alias chain to an RGB color value.
// Follows VARIABLE_ALIAS chains up to 8 levels deep.
// Returns null if the variable is not a COLOR type or cannot be resolved.
const resolveVariableColor = async (
  alias: VariableAlias,
  cache: VariableCache
): Promise<RGB | null> => {
  let id = alias.id;
  for (let i = 0; i < 8; i++) {
    const variable = await getVariableCached(cache, id);
    if (!variable || variable.resolvedType !== "COLOR") return null;
    const collection = await getCollectionCached(
      cache,
      variable.variableCollectionId
    );
    const modeId =
      collection?.defaultModeId ?? Object.keys(variable.valuesByMode)[0];
    if (!modeId) return null;
    const value = variable.valuesByMode[modeId];
    if (
      value !== null &&
      typeof value === "object" &&
      "type" in value &&
      (value as VariableAlias).type === "VARIABLE_ALIAS"
    ) {
      id = (value as VariableAlias).id;
      continue;
    }
    if (value !== null && typeof value === "object" && "r" in value) {
      return value as RGB;
    }
    return null;
  }
  return null;
};

// Resolves the *directly bound* variable (not the resolved alias chain) to a
// human-readable "Collection/Name" label — e.g. a paint bound to
// Button/Bg-Primary which itself aliases Color/Brand/Primary reports
// "Button/Bg-Primary", since that's the variable the designer actually
// attached here. Ground truth about the binding itself, not the value chain.
type VariableLabel = {
  label: string;
  // The designer-set WEB code identifier for this variable (e.g.
  // "--color-brand-primary"), if any — ground truth for what to call this
  // in code, no guessing needed. Undefined when the designer never set one.
  codeSyntax?: string;
};

const resolveVariableLabel = async (
  id: string,
  cache: VariableCache
): Promise<VariableLabel | undefined> => {
  const variable = await getVariableCached(cache, id);
  if (!variable) return undefined;
  const collection = await getCollectionCached(
    cache,
    variable.variableCollectionId
  );
  const label = collection ? `${collection.name}/${variable.name}` : variable.name;
  return { label, codeSyntax: variable.codeSyntax?.WEB };
};

const serializePaints = async (
  paints: readonly Paint[] | symbol | undefined,
  cache: VariableCache
): Promise<SerializedPaint[] | "mixed"> => {
  if (isMixed(paints)) return "mixed";
  if (!paints || !Array.isArray(paints)) return [];

  const result: SerializedPaint[] = [];
  for (const paint of paints) {
    if (paint.visible === false) continue;
    switch (paint.type) {
      case "SOLID": {
        let color = paint.color;
        // When the color is bound to a variable, resolve the variable's value
        // for the current mode. paint.color may hold the pre-binding default
        // rather than the resolved value in some Figma API contexts.
        const colorAlias = (paint as SolidPaint).boundVariables?.color;
        let variableLabel: VariableLabel | undefined;
        if (colorAlias) {
          const resolved = await resolveVariableColor(colorAlias, cache);
          if (resolved) color = resolved;
          variableLabel = await resolveVariableLabel(colorAlias.id, cache);
        }
        result.push({
          type: "SOLID",
          color: toHex(color),
          opacity: paint.opacity,
          variable: variableLabel?.label,
          codeSyntax: variableLabel?.codeSyntax,
        });
        break;
      }
      case "GRADIENT_LINEAR":
      case "GRADIENT_RADIAL":
      case "GRADIENT_ANGULAR":
      case "GRADIENT_DIAMOND":
        result.push({
          type: paint.type,
          gradientStops: serializeGradientStops(
            (paint as GradientPaint).gradientStops
          ),
          gradientTransform: (paint as GradientPaint).gradientTransform,
          opacity: paint.opacity,
        });
        break;
      case "IMAGE":
        result.push({
          type: "IMAGE",
          scaleMode: (paint as ImagePaint).scaleMode,
          imageHash: (paint as ImagePaint).imageHash,
          imageTransform: (paint as ImagePaint).imageTransform,
          opacity: paint.opacity,
        });
        break;
      default:
        break;
    }
  }
  return result;
};

const serializeEffects = (effects: readonly Effect[]): SerializedEffect[] =>
  effects
    .filter((effect) => effect.visible !== false)
    .flatMap((effect): SerializedEffect[] => {
      switch (effect.type) {
        case "DROP_SHADOW":
        case "INNER_SHADOW":
          return [
            {
              type: effect.type,
              color: toHex(effect.color),
              opacity: effect.color.a,
              offset: effect.offset,
              radius: effect.radius,
              spread: effect.spread,
              blendMode: effect.blendMode,
            },
          ];
        case "LAYER_BLUR":
        case "BACKGROUND_BLUR":
          return [{ type: effect.type, radius: effect.radius }];
        default:
          return [];
      }
    });

const serializeLineHeight = (lineHeight: LineHeight | symbol) => {
  if (isMixed(lineHeight)) return "mixed";
  if ("value" in lineHeight) {
    return { value: lineHeight.value, unit: lineHeight.unit };
  }
  return { unit: lineHeight.unit };
};

const serializeLetterSpacing = (letterSpacing: LetterSpacing | symbol) => {
  if (isMixed(letterSpacing)) return "mixed";
  return { value: letterSpacing.value, unit: letterSpacing.unit };
};

const getBounds = (node: SceneNode): SerializedBounds | undefined => {
  if ("x" in node && "y" in node && "width" in node && "height" in node) {
    return {
      x: node.x,
      y: node.y,
      width: node.width,
      height: node.height,
    };
  }
  return undefined;
};

const serializeText = async (node: TextNode, base: SerializedNode) => {
  let fontFamily: string | undefined;
  let fontStyle: string | undefined;
  if (typeof node.fontName === "symbol") {
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
      fontSize: isMixed(node.fontSize) ? "mixed" : node.fontSize,
      fontFamily,
      fontStyle,
      fontWeight: isMixed(node.fontWeight) ? "mixed" : node.fontWeight,
      textDecoration: isMixed(node.textDecoration)
        ? "mixed"
        : node.textDecoration,
      textCase: isMixed(node.textCase) ? "mixed" : node.textCase,
      lineHeight: serializeLineHeight(node.lineHeight),
      letterSpacing: serializeLetterSpacing(node.letterSpacing),
      textAlignHorizontal: isMixed(node.textAlignHorizontal)
        ? "mixed"
        : node.textAlignHorizontal,
      textAlignVertical: isMixed(node.textAlignVertical)
        ? "mixed"
        : node.textAlignVertical,
      textAutoResize: node.textAutoResize,
    },
  };
};

const serializeStyles = async (
  node: SceneNode,
  cache: VariableCache
): Promise<SerializedStyles> => {
  const styles: SerializedStyles = {};

  if ("opacity" in node) {
    styles.opacity = node.opacity as number;
  }
  if ("blendMode" in node) {
    styles.blendMode = node.blendMode as string;
  }
  if ("visible" in node) {
    styles.visible = node.visible;
  }

  if ("fills" in node) {
    styles.fills = await serializePaints(node.fills, cache);
  }
  if ("strokes" in node) {
    styles.strokes = await serializePaints(node.strokes, cache);
  }
  if ("strokeWeight" in node) {
    styles.strokeWeight = isMixed(node.strokeWeight)
      ? "mixed"
      : (node.strokeWeight as number);
  }
  if ("strokeAlign" in node) {
    styles.strokeAlign = node.strokeAlign as string;
  }
  if ("strokeTopWeight" in node) {
    const top = node.strokeTopWeight as number;
    const right = node.strokeRightWeight as number;
    const bottom = node.strokeBottomWeight as number;
    const left = node.strokeLeftWeight as number;
    if (top !== right || right !== bottom || bottom !== left) {
      styles.strokeWeights = { top, right, bottom, left };
    }
  }
  if ("dashPattern" in node) {
    const pattern = node.dashPattern as readonly number[];
    if (pattern.length > 0) {
      styles.dashPattern = [...pattern];
    }
  }

  if ("effects" in node) {
    const effects = node.effects as readonly Effect[];
    if (effects.length > 0) {
      styles.effects = serializeEffects(effects);
    }
  }

  if ("cornerRadius" in node) {
    styles.cornerRadius = isMixed(node.cornerRadius)
      ? "mixed"
      : (node.cornerRadius as number);
  }
  if ("topLeftRadius" in node) {
    const tl = node.topLeftRadius as number;
    const tr = node.topRightRadius as number;
    const br = node.bottomRightRadius as number;
    const bl = node.bottomLeftRadius as number;
    if (tl !== tr || tr !== br || br !== bl) {
      styles.cornerRadii = {
        topLeft: tl,
        topRight: tr,
        bottomRight: br,
        bottomLeft: bl,
      };
    }
  }
  if ("cornerSmoothing" in node) {
    const smoothing = node.cornerSmoothing as number;
    if (smoothing > 0) {
      styles.cornerSmoothing = smoothing;
    }
  }

  if ("layoutMode" in node) {
    const mode = node.layoutMode as string;
    if (mode !== "NONE") {
      styles.autoLayout = {
        direction: mode as "HORIZONTAL" | "VERTICAL" | "GRID",
        gap: (node as FrameNode).itemSpacing,
        primaryAxisAlign: (node as FrameNode).primaryAxisAlignItems as string,
        counterAxisAlign: (node as FrameNode).counterAxisAlignItems as string,
        primaryAxisSizing: (node as FrameNode).primaryAxisSizingMode as string,
        counterAxisSizing: (node as FrameNode).counterAxisSizingMode as string,
        wrap: "layoutWrap" in node ? (node.layoutWrap as string) : undefined,
        counterAxisSpacing:
          "counterAxisSpacing" in node
            ? (node.counterAxisSpacing as number)
            : undefined,
      };
      // GRID is Figma's native CSS-grid-like auto-layout — ground truth from
      // the designer's explicit row/column setup, not an inferred structure.
      if (mode === "GRID" && "gridRowSizes" in node) {
        const grid = node as unknown as GridLayoutMixin;
        styles.autoLayout.gridRowSizes = grid.gridRowSizes.map((t) => ({
          type: t.type,
          value: t.value,
        }));
        styles.autoLayout.gridColumnSizes = grid.gridColumnSizes.map((t) => ({
          type: t.type,
          value: t.value,
        }));
        styles.autoLayout.gridRowGap = grid.gridRowGap;
        styles.autoLayout.gridColumnGap = grid.gridColumnGap;
      }
    }
  }

  if ("paddingLeft" in node) {
    const top = node.paddingTop as number;
    const right = node.paddingRight as number;
    const bottom = node.paddingBottom as number;
    const left = node.paddingLeft as number;
    if (top > 0 || right > 0 || bottom > 0 || left > 0) {
      styles.padding = { top, right, bottom, left };
    }
  }

  if ("clipsContent" in node) {
    styles.clipsContent = node.clipsContent as boolean;
  }
  if ("rotation" in node) {
    const rotation = node.rotation as number;
    if (rotation !== 0) {
      styles.rotation = rotation;
    }
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

  const boundVariables = await resolveBoundVariables(node, cache);
  if (boundVariables) {
    styles.boundVariables = boundVariables;
  }

  return styles;
};

// Resolves node.boundVariables to { figmaFieldName: "Collection/Name" } for
// every directly-bound single-value field (cornerRadii, padding, itemSpacing,
// strokeWeight(s), opacity, gridRowGap/ColumnGap, width/height, ...).
// fills/strokes/effects/layoutGrids/componentProperties are excluded: those
// are per-paint/per-style-slot arrays or maps, not a single VariableAlias —
// fills/strokes are already covered per-paint by SerializedSolidPaint.variable.
const resolveBoundVariables = async (
  node: SceneNode,
  cache: VariableCache
): Promise<Record<string, string> | undefined> => {
  if (!("boundVariables" in node) || !node.boundVariables) return undefined;
  const excluded = new Set([
    "fills",
    "strokes",
    "effects",
    "layoutGrids",
    "componentProperties",
  ]);
  const result: Record<string, string> = {};
  for (const [field, binding] of Object.entries(node.boundVariables)) {
    if (excluded.has(field)) continue;
    if (!binding || Array.isArray(binding) || typeof binding !== "object") continue;
    const alias = binding as VariableAlias;
    if (alias.type !== "VARIABLE_ALIAS") continue;
    const resolved = await resolveVariableLabel(alias.id, cache);
    if (resolved) result[field] = resolved.label;
  }
  return Object.keys(result).length > 0 ? result : undefined;
};

// Resolves explicitVariableModes (collectionId → modeId) to human-readable names.
// Returns e.g. { "Color Semantic": "Dark", "Spacing": "Compact" }
// Exported for find_nodes (code.ts), which resolves this only for nodes that
// already passed a cheap sync filter, instead of for every node in the tree.
export const resolveVariantModes = async (
  node: SceneNode,
  cache: VariableCache
): Promise<Record<string, string> | undefined> => {
  if (!("explicitVariableModes" in node)) return undefined;
  const raw = node.explicitVariableModes as Record<string, string>;
  if (!raw || Object.keys(raw).length === 0) return undefined;
  const result: Record<string, string> = {};
  for (const [collectionId, modeId] of Object.entries(raw)) {
    const collection = await getCollectionCached(cache, collectionId);
    if (!collection) continue;
    const mode = collection.modes.find((m) => m.modeId === modeId);
    if (mode) result[collection.name] = mode.name;
  }
  return Object.keys(result).length > 0 ? result : undefined;
};

// Extracts component property values from an INSTANCE node.
// Returns e.g. { "State": "Hover", "Size": "M", "hasIcon": true }
// Exported for find_nodes (code.ts) — see resolveVariantModes above.
export const resolveComponentProps = (
  node: SceneNode
): Record<string, string | boolean> | undefined => {
  if (node.type !== "INSTANCE") return undefined;
  let props: InstanceNode["componentProperties"];
  try {
    // Throws for instances of a component set Figma flags as having
    // "existing errors" — that's a property of the set, not something this
    // plugin can fix, so skip props for this node rather than aborting the
    // whole subtree serialization.
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
};

// Extracts a node's explicit row/column placement within its parent's GRID
// auto-layout. GridChildrenMixin's fields exist on the type for any
// auto-layout-capable node regardless of the parent's actual layoutMode, so
// this checks the parent is actually a GRID frame, not just property
// presence (an `"x" in node` check alone would wrongly fire for every child
// of a FRAME-typed parent).
const resolveGridPosition = (
  node: SceneNode
): SerializedGridPosition | undefined => {
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
};

// Extracts trigger + transition timing from a node's prototyping reactions.
// Only NODE-navigation actions carry a transition; other action types
// (URL, SET_VARIABLE, ...) have no animation to report.
const resolveReactions = (node: SceneNode): SerializedReaction[] | undefined => {
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
};

// Extracts annotation text (label or labelMarkdown) from a node, if any.
const resolveAnnotations = (node: SceneNode): string[] | undefined => {
  if (!("annotations" in node) || node.annotations.length === 0) return undefined;
  const texts = node.annotations
    .map((a) => a.label ?? a.labelMarkdown)
    .filter((t): t is string => !!t);
  return texts.length > 0 ? texts : undefined;
};

// Extracts the node's designer-defined export presets, if any.
const resolveExportSettings = (
  node: SceneNode
): SerializedExportSetting[] | undefined => {
  if (!("exportSettings" in node) || node.exportSettings.length === 0) {
    return undefined;
  }
  return node.exportSettings.map((s) => ({
    format: s.format as "JPG" | "PNG" | "SVG",
    suffix: "suffix" in s ? s.suffix : undefined,
    constraintType:
      "constraint" in s && s.constraint ? s.constraint.type : undefined,
    constraintValue:
      "constraint" in s && s.constraint ? s.constraint.value : undefined,
  }));
};

// Fetches designer-attached dev resource links on a node, if any.
const resolveDevResources = async (
  node: SceneNode
): Promise<{ name: string; url: string }[] | undefined> => {
  // getDevResourcesAsync hits Figma's own related_links REST endpoint — a
  // real network call against the user's (or their org's) Figma account,
  // not local document data. Dev Resources are a Dev Mode concept, so only
  // spend that network call when the plugin is actually running in Dev
  // Mode — otherwise every find_nodes/get_selection/get_document/get_node
  // call would burn a network round-trip per node for a field nobody in
  // Design mode asked for, and risk hitting Figma's own rate limits.
  if (figma.editorType !== "dev") return undefined;
  if (!("getDevResourcesAsync" in node)) return undefined;
  let resources: Awaited<ReturnType<typeof node.getDevResourcesAsync>>;
  try {
    // Can still reject (e.g. 403 on an unsaved/duplicated file) for reasons
    // that have nothing to do with the node itself — this is optional
    // enrichment, so a failure here shouldn't take down the rest of an
    // otherwise-successful response.
    resources = await node.getDevResourcesAsync();
  } catch {
    return undefined;
  }
  if (resources.length === 0) return undefined;
  return resources.map((r) => ({ name: r.name, url: r.url }));
};

// Resolves an INSTANCE's main-component family name: the COMPONENT_SET name
// for a variant (e.g. "Button"), or the main component's own name when it
// isn't part of a set. Deliberately NOT wired into serializeNode's generic
// base fields — that would resolve it for every INSTANCE in a whole-tree
// fetch (get_node/get_document/get_selection), even nested ones nobody asked
// about, repeating the exact "pay for every node" mistake this file's other
// caches/gates exist to avoid. Used only by the dedicated
// get_main_component_name RPC (code.ts), called once per node actually being
// matched (map/plan's tier-1 name match), never as part of a bulk tree walk.
export const resolveMainComponentName = async (
  node: SceneNode
): Promise<string | undefined> => {
  if (node.type !== "INSTANCE") return undefined;
  try {
    const main = await node.getMainComponentAsync();
    if (!main) return undefined;
    const parent = main.parent;
    return parent && parent.type === "COMPONENT_SET" ? parent.name : main.name;
  } catch {
    // A main component living in an unavailable/unpublished library can
    // reject — this is an optional matching hint, not load-bearing data.
    return undefined;
  }
};

// maxDepth bounds recursion: 0/undefined means unlimited (the original
// behavior every other call site relies on). depth is the current node's
// distance from the call's root (0 at the root). Past maxDepth, children are
// dropped and reported as childCount instead of being walked at all — this
// is a depth *limit*, not depth-then-discard, so a huge subtree past the
// limit costs nothing (get_design_context used to fully serialize every
// descendant before truncating the JSON, which is why deep sections could
// time out regardless of the requested depth).
export const serializeNode = async (
  node: SceneNode,
  maxDepth?: number,
  depth = 0,
  // Shared across the whole tree walk (created once at the root call, then
  // threaded through every recursive call) — see VariableCache above for why.
  cache: VariableCache = createVariableCache()
): Promise<SerializedNode> => {
  const base: SerializedNode = {
    id: node.id,
    name: node.name,
    type: node.type,
    bounds: getBounds(node),
    styles: await serializeStyles(node, cache),
    componentProps: resolveComponentProps(node),
    variantModes: await resolveVariantModes(node, cache),
    gridPosition: resolveGridPosition(node),
    reactions: resolveReactions(node),
    devStatus: "devStatus" in node ? node.devStatus?.type : undefined,
    devResources: await resolveDevResources(node),
    annotations: resolveAnnotations(node),
    exportSettings: resolveExportSettings(node),
  };

  if (node.type === "TEXT") {
    return serializeText(node, base);
  }

  if ("children" in node) {
    const visibleChildren = node.children.filter((child) => child.visible !== false);
    if (maxDepth && depth >= maxDepth) {
      return { ...base, childCount: visibleChildren.length };
    }
    const children = await Promise.all(
      visibleChildren.map((child) =>
        serializeNode(child, maxDepth, depth + 1, cache)
      )
    );
    return { ...base, children };
  }

  return base;
};
