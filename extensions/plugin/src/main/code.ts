import {
  serializeNode,
  resolveComponentProps,
  resolveVariantModes,
  resolveMainComponentName,
  createVariableCache,
} from "./serializer";
import pkg from "../../package.json";

const PLUGIN_VERSION: string = pkg.version;

type RequestType =
  | "get_document"
  | "get_selection"
  | "get_node"
  | "find_nodes"
  | "get_main_component_name"
  | "get_styles"
  | "get_metadata"
  | "get_design_context"
  | "get_variable_defs"
  | "get_screenshot"
  | "set_node_visibility"
  | "set_text_content"
  | "set_text_properties"
  | "set_node_properties"
  | "set_solid_fill"
  | "set_gradient_fill"
  | "set_effects"
  | "set_stroke_properties"
  | "set_auto_layout"
  | "create_frame"
  | "create_text"
  | "create_shape"
  | "create_image"
  | "duplicate_nodes"
  | "reparent_nodes"
  | "group_nodes"
  | "ungroup_node"
  | "set_selection"
  | "scroll_and_zoom_into_view"
  | "delete_nodes";

type ServerRequestParams = Record<string, unknown> & {
  format?: "PNG" | "SVG" | "JPG" | "PDF";
  scale?: number;
  /**
   * When true, export using the node's absolute bounds (matching Figma REST
   * image export's `use_absolute_bounds`) — clips raster exports to the
   * node's logical bounds instead of its rendered bounds including
   * overflow/effects.
   */
  clip?: boolean;
  depth?: number;
  /** get_node: skip styles/variables/dev-resources — only id/name/type/bounds/children, for structure-only consumers like hit-testing. */
  lean?: boolean;
  /** find_nodes: case-insensitive substring match against node name. */
  query?: string;
  /** find_nodes: case-insensitive substring match against TEXT characters. */
  textQuery?: string;
  /** find_nodes: exact (case-insensitive) Figma node type, e.g. "FRAME". */
  nodeType?: string;
  /** find_nodes: case-insensitive substring match against a resolved variant mode name. */
  mode?: string;
  /** find_nodes: restrict the search to this node's subtree (default: current page). */
  withinNodeId?: string;
  /** find_nodes: cap recursion depth relative to the search root (0 = unlimited). */
  maxDepth?: number;
  /** find_nodes: stop once this many matches are found (default 50). */
  maxResults?: number;
};

type ServerRequest = {
  type: RequestType;
  requestId: string;
  nodeIds?: string[];
  params?: ServerRequestParams;
};

type PluginResponse = {
  type: RequestType;
  requestId: string;
  data?: unknown;
  error?: string;
};

// --- File identity ---

let fallbackFileKey: string | null = null;

function randomFallbackFileKey(): string {
  const random = Math.random().toString(36).slice(2, 10);
  return `unsaved-${Date.now().toString(36)}-${random}`;
}

function currentFileKey(): string {
  // figma.fileKey is available for saved files; unsaved files (and files
  // with duplicate names) get a stable, session-scoped fallback instead.
  try {
    if (typeof figma.fileKey === "string" && figma.fileKey) {
      return figma.fileKey;
    }
  } catch {
    // fileKey may not be available in all contexts
  }
  if (!fallbackFileKey) {
    fallbackFileKey = randomFallbackFileKey();
    console.warn(
      `[figma-bridge] figma.fileKey unavailable for "${figma.root.name}". ` +
        `Using session fallback key "${fallbackFileKey}". ` +
        `If you encounter this in a built plugin, please report at ` +
        `https://github.com/KirillBaranov/figma-map/issues with steps to reproduce.`
    );
  }
  return fallbackFileKey;
}

function pushStatus(): void {
  const selection = figma.currentPage.selection;
  figma.ui.postMessage({
    type: "plugin-status",
    payload: {
      fileName: figma.root.name,
      fileKey: currentFileKey(),
      selectionCount: selection.length,
      selectedNodeIds: selection.map((node) => node.id),
      selectedNodeNames: selection.map((node) => node.name),
      version: PLUGIN_VERSION,
    },
  });
}

// --- Node lookup helpers ---

function isSceneNode(node: BaseNode | null): node is SceneNode {
  return node !== null && node.type !== "DOCUMENT" && node.type !== "PAGE";
}

function isTextNode(node: BaseNode | null): node is TextNode {
  return node !== null && node.type === "TEXT";
}

function supportsChildren(node: BaseNode): node is BaseNode & ChildrenMixin {
  return "appendChild" in node;
}

async function requireSceneNode(nodeId: string): Promise<SceneNode> {
  const node = await figma.getNodeByIdAsync(nodeId);
  if (!isSceneNode(node)) {
    throw new Error(`Node not found: ${nodeId}`);
  }
  return node;
}

async function requireTextNode(nodeId: string): Promise<TextNode> {
  const node = await figma.getNodeByIdAsync(nodeId);
  if (!isTextNode(node)) {
    throw new Error(`Text node not found: ${nodeId}`);
  }
  return node;
}

async function requireContainerNode(parentId: string): Promise<BaseNode & ChildrenMixin> {
  const parent = await figma.getNodeByIdAsync(parentId);
  if (!parent || parent.type === "DOCUMENT" || !supportsChildren(parent)) {
    throw new Error(`Parent does not support children: ${parentId}`);
  }
  return parent;
}

// --- Color / paint helpers ---

function parseHexColor(hex: string): RGB {
  const stripped = hex.trim().replace(/^#/, "");
  if (stripped.length !== 3 && stripped.length !== 6) {
    throw new Error(`Invalid hex color: ${hex}`);
  }
  const expanded =
    stripped.length === 3
      ? stripped
          .split("")
          .map((c) => c + c)
          .join("")
      : stripped;
  if (!/^[0-9a-fA-F]{6}$/.test(expanded)) {
    throw new Error(`Invalid hex color: ${hex}`);
  }
  return {
    r: parseInt(expanded.slice(0, 2), 16) / 255,
    g: parseInt(expanded.slice(2, 4), 16) / 255,
    b: parseInt(expanded.slice(4, 6), 16) / 255,
  };
}

function applySolidFill(
  node: SceneNode,
  hex: string,
  opacity?: number,
  target: "fill" | "stroke" = "fill"
): void {
  const paint: SolidPaint = { type: "SOLID", color: parseHexColor(hex), opacity: opacity ?? 1 };

  if (target === "stroke") {
    if (!("strokes" in node)) {
      throw new Error(`Node does not support strokes: ${node.id}`);
    }
    (node as GeometryMixin & { strokes: ReadonlyArray<Paint> }).strokes = [paint];
    return;
  }

  if (!("fills" in node)) {
    throw new Error(`Node does not support fills: ${node.id}`);
  }
  (node as GeometryMixin & { fills: ReadonlyArray<Paint> }).fills = [paint];
}

function applyTextFill(node: TextNode, hex: string, opacity?: number): void {
  node.fills = [{ type: "SOLID", color: parseHexColor(hex), opacity: opacity ?? 1 }];
}

type GradientStopInput = { position: number; hex: string; opacity?: number };
type GradientPaintType =
  | "GRADIENT_LINEAR"
  | "GRADIENT_RADIAL"
  | "GRADIENT_ANGULAR"
  | "GRADIENT_DIAMOND";

function buildGradientPaint(
  paintType: GradientPaintType,
  stops: GradientStopInput[],
  transform: Transform | undefined,
  opacity: number | undefined
): GradientPaint {
  const gradientStops = stops.map((stop) => {
    const rgb = parseHexColor(stop.hex);
    return { position: stop.position, color: { r: rgb.r, g: rgb.g, b: rgb.b, a: stop.opacity ?? 1 } };
  });
  // Figma-default identity transform: horizontal, left-to-right.
  const gradientTransform: Transform = transform ?? [
    [1, 0, 0],
    [0, 1, 0],
  ];
  return { type: paintType, gradientStops, gradientTransform, opacity: opacity ?? 1 };
}

// --- Text helpers ---

async function loadFontsForNode(node: TextNode): Promise<void> {
  const fonts = new Map<string, FontName>();

  if (node.characters.length > 0) {
    for (const font of node.getRangeAllFontNames(0, node.characters.length)) {
      fonts.set(`${font.family}::${font.style}`, font);
    }
  } else if (typeof node.fontName !== "symbol") {
    fonts.set(`${node.fontName.family}::${node.fontName.style}`, node.fontName);
  } else {
    throw new Error(`Cannot determine font for empty mixed-font text node: ${node.id}`);
  }

  await Promise.all([...fonts.values()].map((font) => figma.loadFontAsync(font)));
}

async function loadFont(family: string, style: string): Promise<FontName> {
  const font: FontName = { family, style };
  await figma.loadFontAsync(font);
  return font;
}

// --- Geometry helpers ---

function setPosition(node: SceneNode, x: unknown, y: unknown): void {
  if ("x" in node && typeof x === "number") node.x = x;
  if ("y" in node && typeof y === "number") node.y = y;
}

function setSizeIfSupported(node: SceneNode, width: unknown, height: unknown): void {
  if (typeof width !== "number" && typeof height !== "number") return;
  if (!("resize" in node) || typeof node.resize !== "function") {
    throw new Error(`Node does not support resizing: ${node.id}`);
  }
  node.resize(typeof width === "number" ? width : node.width, typeof height === "number" ? height : node.height);
}

async function reparentIfRequested(node: SceneNode, parentId: unknown): Promise<void> {
  if (typeof parentId !== "string") return;
  const parent = await requireContainerNode(parentId);
  parent.appendChild(node);
}

function decodeBase64Image(base64: string): Uint8Array {
  try {
    return figma.base64Decode(base64);
  } catch {
    throw new Error("Invalid base64 image payload");
  }
}

// --- Editor-mode guard ---

const EDIT_REQUEST_TYPES = new Set<RequestType>([
  "set_node_visibility",
  "set_text_content",
  "set_text_properties",
  "set_node_properties",
  "set_solid_fill",
  "set_gradient_fill",
  "set_effects",
  "set_stroke_properties",
  "set_auto_layout",
  "create_frame",
  "create_text",
  "create_shape",
  "create_image",
  "duplicate_nodes",
  "reparent_nodes",
  "group_nodes",
  "ungroup_node",
  "delete_nodes",
]);

function requireDesignEditor(toolName: RequestType): void {
  // Dev Mode is read-only — every figma.create*/setter throws there with a
  // confusing error, so reject up front with a clear one instead.
  if (figma.editorType === "dev") {
    throw new Error(
      `${toolName} requires the plugin to be opened in Figma's design editor (Dev Mode is read-only). Switch to the design editor and re-run.`
    );
  }
}

// --- Per-request-type handlers. Each returns the `data` payload only; the
// dispatcher below wraps it in the {type, requestId, data|error} envelope
// and is the single place errors are caught. ---

async function handleGetDocument(request: ServerRequest) {
  const depth = typeof request.params?.depth === "number" ? request.params.depth : undefined;
  return serializeNode(figma.currentPage as unknown as SceneNode, depth);
}

async function handleGetSelection(request: ServerRequest) {
  const depth = typeof request.params?.depth === "number" ? request.params.depth : undefined;
  return Promise.all(figma.currentPage.selection.map((node) => serializeNode(node, depth)));
}

async function handleGetNode(request: ServerRequest) {
  const nodeId = request.nodeIds?.[0];
  if (!nodeId) throw new Error("nodeIds is required for get_node");

  const node = await figma.getNodeByIdAsync(nodeId);
  if (!node || node.type === "DOCUMENT") throw new Error(`Node not found: ${nodeId}`);

  const depth = typeof request.params?.depth === "number" ? request.params.depth : undefined;
  const lean = request.params?.lean === true;
  return serializeNode(node as SceneNode, depth, 0, undefined, lean);
}

async function handleFindNodes(request: ServerRequest) {
  const params = request.params ?? {};
  const query = typeof params.query === "string" ? params.query.toLowerCase() : "";
  const textQuery = typeof params.textQuery === "string" ? params.textQuery.toLowerCase() : "";
  const nodeType = typeof params.nodeType === "string" ? params.nodeType.toUpperCase() : "";
  const mode = typeof params.mode === "string" ? params.mode.toLowerCase() : "";
  const maxDepth = typeof params.maxDepth === "number" ? params.maxDepth : 0;
  const maxResults =
    typeof params.maxResults === "number" && params.maxResults > 0 ? params.maxResults : 50;

  let root: BaseNode & ChildrenMixin;
  if (typeof params.withinNodeId === "string") {
    const found = await figma.getNodeByIdAsync(params.withinNodeId);
    if (!found || !supportsChildren(found)) {
      throw new Error(`Node not found or has no children: ${params.withinNodeId}`);
    }
    root = found;
  } else {
    root = figma.currentPage as unknown as BaseNode & ChildrenMixin;
  }

  // Cheap, synchronous predicate — no await, no style/variable resolution.
  // This is what lets find_nodes search a whole document without paying the
  // per-node async cost that times out get_document.
  const matchesCheap = (node: SceneNode): boolean => {
    if (query && !node.name.toLowerCase().includes(query)) return false;
    if (nodeType && node.type.toUpperCase() !== nodeType) return false;
    if (textQuery) {
      const chars = "characters" in node ? node.characters : "";
      if (!chars.toLowerCase().includes(textQuery)) return false;
    }
    return true;
  };

  type Match = { node: SceneNode; path: string; variantModes?: Record<string, string> };
  const matches: Match[] = [];
  // Shared for the whole walk + enrichment below — repeated matches (e.g.
  // several instances of the same component) shouldn't each re-resolve the
  // same variable/collection from scratch.
  const variableCache = createVariableCache();

  // Depth-first, document order, with early-exit once maxResults is hit.
  const visit = async (node: BaseNode & ChildrenMixin, path: string, depth: number): Promise<void> => {
    for (const child of node.children) {
      if (matches.length >= maxResults) return;
      if (child.visible === false) continue;
      const childPath = path ? `${path} › ${child.name}` : child.name;

      if (matchesCheap(child)) {
        let variantModes: Record<string, string> | undefined;
        let modeOk = true;
        if (mode) {
          // Resolving variant modes is async — only pay for it on nodes
          // that already passed the cheap filter AND actually have
          // explicit modes set.
          const hasOwnModes =
            "explicitVariableModes" in child &&
            Object.keys(child.explicitVariableModes as Record<string, string>).length > 0;
          if (hasOwnModes) {
            variantModes = await resolveVariantModes(child, variableCache);
            modeOk = Object.values(variantModes ?? {}).some((v) => v.toLowerCase().includes(mode));
          } else {
            modeOk = false;
          }
        }
        if (modeOk) matches.push({ node: child, path, variantModes });
      }

      if (matches.length >= maxResults) return;
      if (supportsChildren(child) && (maxDepth === 0 || depth < maxDepth)) {
        await visit(child, childPath, depth + 1);
      }
    }
  };

  await visit(root, "", 0);

  const resultMatches = await Promise.all(
    matches.map(async ({ node, path, variantModes }) => ({
      id: node.id,
      name: node.name,
      type: node.type,
      path,
      characters: "characters" in node ? node.characters : undefined,
      componentProps: resolveComponentProps(node),
      variantModes: variantModes ?? (await resolveVariantModes(node, variableCache)),
      devStatus: "devStatus" in node ? node.devStatus?.type : undefined,
    }))
  );

  return { matches: resultMatches };
}

async function handleGetMainComponentName(request: ServerRequest) {
  const nodeId = request.nodeIds?.[0];
  if (!nodeId) throw new Error("nodeIds is required for get_main_component_name");

  const node = await figma.getNodeByIdAsync(nodeId);
  if (!node || node.type === "DOCUMENT") throw new Error(`Node not found: ${nodeId}`);

  const name = await resolveMainComponentName(node as SceneNode);
  return { name: name ?? null };
}

async function handleGetStyles() {
  const [paintStyles, textStyles, effectStyles, gridStyles] = await Promise.all([
    figma.getLocalPaintStylesAsync(),
    figma.getLocalTextStylesAsync(),
    figma.getLocalEffectStylesAsync(),
    figma.getLocalGridStylesAsync(),
  ]);
  return {
    paints: paintStyles.map((style) => ({ id: style.id, name: style.name, paints: style.paints })),
    text: textStyles.map((style) => ({
      id: style.id,
      name: style.name,
      fontSize: style.fontSize,
      fontName: style.fontName,
      textDecoration: style.textDecoration,
      lineHeight: style.lineHeight,
      letterSpacing: style.letterSpacing,
    })),
    effects: effectStyles.map((style) => ({ id: style.id, name: style.name, effects: style.effects })),
    grids: gridStyles.map((style) => ({ id: style.id, name: style.name, layoutGrids: style.layoutGrids })),
  };
}

function handleGetMetadata() {
  return {
    fileName: figma.root.name,
    currentPageId: figma.currentPage.id,
    currentPageName: figma.currentPage.name,
    pageCount: figma.root.children.length,
    pages: figma.root.children.map((page) => ({ id: page.id, name: page.name })),
  };
}

async function handleGetDesignContext(request: ServerRequest) {
  const depth = typeof request.params?.depth === "number" ? request.params.depth : 2;
  const selection = figma.currentPage.selection;
  const contextNodes =
    selection.length > 0
      ? await Promise.all(selection.map((node) => serializeNode(node, depth)))
      : [await serializeNode(figma.currentPage as unknown as SceneNode, depth)];

  return {
    fileName: figma.root.name,
    currentPage: { id: figma.currentPage.id, name: figma.currentPage.name },
    selectionCount: selection.length,
    context: contextNodes,
  };
}

function serializeVariableValue(value: VariableValue): unknown {
  if (typeof value === "object" && value !== null) {
    if ("type" in value && value.type === "VARIABLE_ALIAS") {
      return { type: "VARIABLE_ALIAS", id: value.id };
    }
    if ("r" in value && "g" in value && "b" in value) {
      const color = value as RGBA;
      return { type: "COLOR", r: color.r, g: color.g, b: color.b, a: "a" in color ? color.a : 1 };
    }
  }
  return value;
}

function buildVariableCollectionEntry(collection: VariableCollection, variables: Variable[]) {
  return {
    id: collection.id,
    name: collection.name,
    modes: collection.modes.map((mode) => ({ modeId: mode.modeId, name: mode.name })),
    variables: variables.map((variable) => ({
      id: variable.id,
      name: variable.name,
      resolvedType: variable.resolvedType,
      valuesByMode: Object.fromEntries(
        Object.entries(variable.valuesByMode).map(([modeId, value]) => [modeId, serializeVariableValue(value)])
      ),
      // codeSyntax: designer-set per-platform code identifiers, e.g.
      // { WEB: "--color-brand-primary" } — the single most direct signal
      // for "what to call this in code" when populated. scopes: which
      // property types this variable is intended for (e.g. CORNER_RADIUS).
      codeSyntax: variable.codeSyntax,
      scopes: variable.scopes,
    })),
  };
}

async function handleGetVariableDefs() {
  const localCollections = await figma.variables.getLocalVariableCollectionsAsync();
  const seenCollectionIds = new Set(localCollections.map((c) => c.id));
  const localData = await Promise.all(
    localCollections.map(async (collection) => {
      const variables = (
        await Promise.all(collection.variableIds.map((id) => figma.variables.getVariableByIdAsync(id)))
      ).filter((v): v is Variable => v !== null);
      return buildVariableCollectionEntry(collection, variables);
    })
  );

  // Node-bound variables resolve fine even from a published team library,
  // but getLocalVariableCollectionsAsync only sees collections local to this
  // file — a file whose tokens are all library-sourced would otherwise
  // report an empty catalog despite tokens clearly using them. Library
  // variables have to be imported by key to read their values.
  let libraryData: Array<ReturnType<typeof buildVariableCollectionEntry>> = [];
  try {
    const libraryCollections = await figma.teamLibrary.getAvailableLibraryVariableCollectionsAsync();
    const entries = await Promise.all(
      libraryCollections.map(async (libCollection) => {
        const libVars = await figma.teamLibrary.getVariablesInLibraryCollectionAsync(libCollection.key);
        const imported = (
          await Promise.all(libVars.map((lv) => figma.variables.importVariableByKeyAsync(lv.key)))
        ).filter((v): v is Variable => v !== null);
        if (imported.length === 0) return null;

        const collection = await figma.variables.getVariableCollectionByIdAsync(imported[0].variableCollectionId);
        if (!collection || seenCollectionIds.has(collection.id)) return null;
        seenCollectionIds.add(collection.id);
        return buildVariableCollectionEntry(collection, imported);
      })
    );
    libraryData = entries.filter((e): e is NonNullable<typeof e> => e !== null);
  } catch {
    // No team libraries enabled / no access — local-only is still useful,
    // so don't fail the whole request over this.
  }

  return { collections: [...localData, ...libraryData] };
}

// Topmost ancestor frame that owns explicitVariableModes (the frame that
// defines the theme for this subtree), or null if the node itself already
// has explicit modes and needs no ancestor context.
function findThemeRootFrame(node: SceneNode): FrameNode | null {
  const hasOwnModes =
    "explicitVariableModes" in node &&
    Object.keys(node.explicitVariableModes as Record<string, string>).length > 0;
  if (hasOwnModes) return null;

  let result: FrameNode | null = null;
  let cursor: BaseNode | null = node.parent;
  while (cursor && cursor.type !== "PAGE" && cursor.type !== "DOCUMENT") {
    if (
      (cursor.type === "FRAME" || cursor.type === "COMPONENT" || cursor.type === "COMPONENT_SET") &&
      "explicitVariableModes" in cursor &&
      Object.keys(cursor.explicitVariableModes as Record<string, string>).length > 0
    ) {
      result = cursor as FrameNode;
    }
    cursor = cursor.parent;
  }
  return result;
}

type ExportFormat = "PNG" | "SVG" | "JPG" | "PDF";

async function exportOneNode(node: SceneNode, format: ExportFormat, scale: number, clip: boolean) {
  const commonSettings = clip ? { contentsOnly: true, useAbsoluteBounds: true } : {};
  const settings: ExportSettings =
    format === "SVG"
      ? { format: "SVG", ...commonSettings }
      : format === "PDF"
        ? { format: "PDF", ...commonSettings }
        : format === "JPG"
          ? { format: "JPG", constraint: { type: "SCALE", value: scale }, ...commonSettings }
          : { format: "PNG", constraint: { type: "SCALE", value: scale }, ...commonSettings };

  // exportAsync ignores a node's own explicitVariableModes and renders using
  // page-level modes instead. Propagate the node's modes (and its
  // ancestors') to the page before exporting, then also apply the same mode
  // NAMES to every other collection with a matching mode (e.g. "Light" in
  // Color Semantic → "Light" in foundation/base) so derived fills render
  // with the correct theme.
  const page = figma.currentPage;
  const savedModes: Array<{ collection: VariableCollection; modeId: string | null }> = [];

  const ancestorModes: Record<string, string> = {};
  {
    const chain: Array<Record<string, string>> = [];
    let cursor: BaseNode | null = node;
    while (cursor && cursor.type !== "PAGE" && cursor.type !== "DOCUMENT") {
      if ("explicitVariableModes" in cursor) {
        chain.unshift(cursor.explicitVariableModes as Record<string, string>);
      }
      cursor = cursor.parent;
    }
    // Root-to-node order so a closer ancestor's mode wins over a farther one.
    for (const modes of chain) Object.assign(ancestorModes, modes);
  }

  if (Object.keys(ancestorModes).length > 0) {
    try {
      const wantedModeNames = new Set<string>();
      for (const [collectionId, modeId] of Object.entries(ancestorModes)) {
        const collection = await figma.variables.getVariableCollectionByIdAsync(collectionId);
        if (!collection) continue;
        const pageModes = page.explicitVariableModes as Record<string, string>;
        savedModes.push({ collection, modeId: pageModes[collectionId] ?? null });
        page.setExplicitVariableModeForCollection(collection, modeId);
        const mode = collection.modes.find((m) => m.modeId === modeId);
        if (mode) wantedModeNames.add(mode.name);
      }

      if (wantedModeNames.size > 0) {
        const allCollections = await figma.variables.getLocalVariableCollectionsAsync();
        for (const collection of allCollections) {
          if (ancestorModes[collection.id]) continue;
          const match = collection.modes.find((m) => wantedModeNames.has(m.name));
          if (!match) continue;
          const pageModes = page.explicitVariableModes as Record<string, string>;
          savedModes.push({ collection, modeId: pageModes[collection.id] ?? null });
          page.setExplicitVariableModeForCollection(collection, match.modeId);
        }
      }
    } catch (err) {
      // Read-only access (viewer permission) means none of the mode-set
      // calls above actually took effect — nothing to restore, so drop the
      // records and render with whatever mode is currently active instead
      // of aborting the export.
      const message = err instanceof Error ? err.message : String(err);
      if (!message.includes("read-only")) throw err;
      savedModes.length = 0;
    }
  }

  // If the node has no background fill, export the nearest ancestor that
  // does and report crop coordinates so the caller can slice the right region.
  const backgroundAncestor = findThemeRootFrame(node);
  const exportTarget: SceneNode = backgroundAncestor ?? node;

  let cropX: number | undefined;
  let cropY: number | undefined;
  let cropW: number | undefined;
  let cropH: number | undefined;
  if (backgroundAncestor) {
    const targetBox = node.absoluteBoundingBox!;
    const ancestorBox = backgroundAncestor.absoluteBoundingBox!;
    cropX = Math.round((targetBox.x - ancestorBox.x) * scale);
    cropY = Math.round((targetBox.y - ancestorBox.y) * scale);
    cropW = Math.round(targetBox.width * scale);
    cropH = Math.round(targetBox.height * scale);
  }

  let bytes: Uint8Array;
  try {
    bytes = await exportTarget.exportAsync(settings);
  } finally {
    for (const { collection, modeId } of savedModes) {
      if (modeId === null) {
        page.clearExplicitVariableModeForCollection(collection);
      } else {
        page.setExplicitVariableModeForCollection(collection, modeId);
      }
    }
  }

  return {
    nodeId: node.id,
    nodeName: node.name,
    format,
    base64: figma.base64Encode(bytes),
    width: cropW !== undefined ? cropW / scale : exportTarget.width,
    height: cropH !== undefined ? cropH / scale : exportTarget.height,
    cropX,
    cropY,
    cropW,
    cropH,
  };
}

async function handleGetScreenshot(request: ServerRequest) {
  const format: ExportFormat =
    request.params?.format === "SVG" ||
    request.params?.format === "PDF" ||
    request.params?.format === "JPG" ||
    request.params?.format === "PNG"
      ? request.params.format
      : "PNG";
  const scale = typeof request.params?.scale === "number" ? request.params.scale : 2;
  const clip = request.params?.clip === true;

  let targetNodes: SceneNode[];
  if (request.nodeIds && request.nodeIds.length > 0) {
    const nodes = await Promise.all(request.nodeIds.map((id) => figma.getNodeByIdAsync(id)));
    targetNodes = nodes.filter(
      (node): node is SceneNode => node !== null && node.type !== "DOCUMENT" && node.type !== "PAGE"
    );
  } else {
    targetNodes = [...figma.currentPage.selection];
  }

  if (targetNodes.length === 0) {
    throw new Error("No nodes to export. Select nodes or provide nodeIds.");
  }

  // Sequential (not Promise.all): each export temporarily swaps page-level
  // variable modes, and concurrent swaps would interfere with each other.
  const exports: Awaited<ReturnType<typeof exportOneNode>>[] = [];
  for (const node of targetNodes) {
    exports.push(await exportOneNode(node, format, scale, clip));
  }

  return { exports };
}

async function handleSetNodeVisibility(request: ServerRequest) {
  const rawItems = request.params?.items;
  if (!Array.isArray(rawItems) || rawItems.length === 0) {
    throw new Error("items is required for set_node_visibility");
  }
  const items = rawItems as Array<{ nodeId: string; visible: boolean }>;
  const results: Array<
    { nodeId: string; previousVisible: boolean; visible: boolean } | { nodeId: string; error: string }
  > = [];
  for (const { nodeId, visible } of items) {
    const node = await figma.getNodeByIdAsync(nodeId);
    if (!node || node.type === "DOCUMENT" || node.type === "PAGE") {
      results.push({ nodeId, error: `Node not found: ${nodeId}` });
      continue;
    }
    const sceneNode = node as SceneNode;
    const previousVisible = sceneNode.visible;
    sceneNode.visible = visible;
    results.push({ nodeId, previousVisible, visible });
  }
  return { results };
}

async function handleSetTextContent(request: ServerRequest) {
  const nodeId = request.nodeIds?.[0];
  const text = request.params?.text;
  if (!nodeId) throw new Error("nodeIds is required for set_text_content");
  if (typeof text !== "string") throw new Error("text is required for set_text_content");

  const node = await requireTextNode(nodeId);
  await loadFontsForNode(node);

  const previousCharacters = node.characters;
  node.characters = text;

  return { nodeId: node.id, nodeName: node.name, previousCharacters, characters: node.characters };
}

async function handleSetTextProperties(request: ServerRequest) {
  const nodeId = request.nodeIds?.[0];
  if (!nodeId) throw new Error("nodeIds is required for set_text_properties");

  const node = await requireTextNode(nodeId);
  const params = request.params ?? {};
  const applied: Record<string, unknown> = {};

  await loadFontsForNode(node);

  if (typeof params.fontFamily === "string" || typeof params.fontStyle === "string") {
    const currentFontName = typeof node.fontName === "symbol" ? null : node.fontName;
    const nextFamily = typeof params.fontFamily === "string" ? params.fontFamily : currentFontName?.family;
    const nextStyle = typeof params.fontStyle === "string" ? params.fontStyle : currentFontName?.style;

    if (!nextFamily || !nextStyle) {
      throw new Error("fontFamily and fontStyle must resolve to a concrete font for set_text_properties");
    }

    node.fontName = await loadFont(nextFamily, nextStyle);
    applied.fontName = node.fontName;
  }

  if (typeof params.fontSize === "number") {
    node.fontSize = params.fontSize;
    applied.fontSize = node.fontSize;
  }

  if (
    params.textAlignHorizontal === "LEFT" ||
    params.textAlignHorizontal === "CENTER" ||
    params.textAlignHorizontal === "RIGHT" ||
    params.textAlignHorizontal === "JUSTIFIED"
  ) {
    node.textAlignHorizontal = params.textAlignHorizontal;
    applied.textAlignHorizontal = node.textAlignHorizontal;
  }

  if (
    params.textAlignVertical === "TOP" ||
    params.textAlignVertical === "CENTER" ||
    params.textAlignVertical === "BOTTOM"
  ) {
    node.textAlignVertical = params.textAlignVertical;
    applied.textAlignVertical = node.textAlignVertical;
  }

  if (
    params.textAutoResize === "NONE" ||
    params.textAutoResize === "WIDTH_AND_HEIGHT" ||
    params.textAutoResize === "HEIGHT" ||
    params.textAutoResize === "TRUNCATE"
  ) {
    node.textAutoResize = params.textAutoResize;
    applied.textAutoResize = node.textAutoResize;
  }

  if (typeof params.lineHeightPx === "number") {
    node.lineHeight = { unit: "PIXELS", value: params.lineHeightPx };
    applied.lineHeight = node.lineHeight;
  }

  if (typeof params.letterSpacingPx === "number") {
    node.letterSpacing = { unit: "PIXELS", value: params.letterSpacingPx };
    applied.letterSpacing = node.letterSpacing;
  }

  if (typeof params.fillHex === "string") {
    const fillOpacity = typeof params.fillOpacity === "number" ? params.fillOpacity : undefined;
    applyTextFill(node, params.fillHex, fillOpacity);
    applied.fillHex = params.fillHex;
    applied.fillOpacity = fillOpacity ?? 1;
  }

  if (typeof params.x === "number" || typeof params.y === "number") {
    setPosition(node, params.x, params.y);
    applied.x = node.x;
    applied.y = node.y;
  }

  setSizeIfSupported(node, params.width, params.height);
  if (typeof params.width === "number" || typeof params.height === "number") {
    applied.width = node.width;
    applied.height = node.height;
  }

  return { nodeId: node.id, nodeName: node.name, applied };
}

async function handleSetNodeProperties(request: ServerRequest) {
  const nodeId = request.nodeIds?.[0];
  if (!nodeId) throw new Error("nodeIds is required for set_node_properties");

  const node = await requireSceneNode(nodeId);
  const params = request.params ?? {};
  const applied: Record<string, unknown> = {};

  if (Object.keys(params).length === 0) {
    throw new Error("At least one property is required for set_node_properties");
  }

  if (typeof params.name === "string") {
    node.name = params.name;
    applied.name = node.name;
  }

  if (typeof params.visible === "boolean") {
    node.visible = params.visible;
    applied.visible = node.visible;
  }

  if (typeof params.x === "number" || typeof params.y === "number") {
    if (!("x" in node) || !("y" in node)) {
      throw new Error(`Node does not support x/y positioning: ${node.id}`);
    }
    setPosition(node, params.x, params.y);
    applied.x = node.x;
    applied.y = node.y;
  }

  if (typeof params.width === "number" || typeof params.height === "number") {
    setSizeIfSupported(node, params.width, params.height);
    applied.width = node.width;
    applied.height = node.height;
  }

  if (typeof params.rotation === "number") {
    if (!("rotation" in node)) {
      throw new Error(`Node does not support rotation: ${node.id}`);
    }
    node.rotation = params.rotation;
    applied.rotation = node.rotation;
  }

  if (typeof params.opacity === "number") {
    if (!("opacity" in node)) {
      throw new Error(`Node does not support opacity: ${node.id}`);
    }
    node.opacity = params.opacity;
    applied.opacity = node.opacity;
  }

  if (typeof params.cornerRadius === "number") {
    if (!("cornerRadius" in node)) {
      throw new Error(`Node does not support cornerRadius: ${node.id}`);
    }
    node.cornerRadius = params.cornerRadius;
    applied.cornerRadius = node.cornerRadius;
  }

  return { nodeId: node.id, nodeName: node.name, applied };
}

async function handleSetSolidFill(request: ServerRequest) {
  const nodeId = request.nodeIds?.[0];
  if (!nodeId) throw new Error("nodeIds is required for set_solid_fill");

  const node = await requireSceneNode(nodeId);
  const params = request.params ?? {};
  if (typeof params.hex !== "string") throw new Error("hex is required");

  const target = params.target === "stroke" ? "stroke" : "fill";
  const opacity = typeof params.opacity === "number" ? params.opacity : undefined;
  applySolidFill(node, params.hex, opacity, target);

  return { nodeId: node.id, nodeName: node.name, applied: { target, hex: params.hex, opacity: opacity ?? 1 } };
}

async function handleSetGradientFill(request: ServerRequest) {
  const nodeId = request.nodeIds?.[0];
  if (!nodeId) throw new Error("nodeIds is required for set_gradient_fill");

  const node = await requireSceneNode(nodeId);
  const params = request.params ?? {};

  const target = params.target === "stroke" ? "stroke" : "fill";
  if (target === "fill" && !("fills" in node)) throw new Error(`Node does not support fills: ${node.id}`);
  if (target === "stroke" && !("strokes" in node)) throw new Error(`Node does not support strokes: ${node.id}`);

  const gradientKind = typeof params.gradientType === "string" ? params.gradientType : "LINEAR";
  const paintType = `GRADIENT_${gradientKind}` as GradientPaintType;
  if (
    paintType !== "GRADIENT_LINEAR" &&
    paintType !== "GRADIENT_RADIAL" &&
    paintType !== "GRADIENT_ANGULAR" &&
    paintType !== "GRADIENT_DIAMOND"
  ) {
    throw new Error(`Unsupported gradient type: ${gradientKind}`);
  }

  if (!Array.isArray(params.gradientStops) || params.gradientStops.length < 2) {
    throw new Error("gradientStops must have at least 2 entries");
  }
  const stops = params.gradientStops as GradientStopInput[];

  const transform =
    Array.isArray(params.gradientTransform) && params.gradientTransform.length === 2
      ? (params.gradientTransform as Transform)
      : undefined;
  const opacity = typeof params.opacity === "number" ? params.opacity : undefined;

  const paint = buildGradientPaint(paintType, stops, transform, opacity);
  if (target === "fill") {
    (node as GeometryMixin & { fills: ReadonlyArray<Paint> }).fills = [paint];
  } else {
    (node as GeometryMixin & { strokes: ReadonlyArray<Paint> }).strokes = [paint];
  }

  return {
    nodeId: node.id,
    nodeName: node.name,
    applied: { target, gradientType: paintType, stops: paint.gradientStops.length },
  };
}

function buildEffect(raw: Record<string, unknown>, index: number): Effect {
  const type = raw.type;
  if (type === "DROP_SHADOW" || type === "INNER_SHADOW") {
    if (typeof raw.color !== "string") throw new Error(`effects[${index}].color must be a hex string`);
    const offset = raw.offset as { x?: unknown; y?: unknown } | undefined;
    if (!offset || typeof offset.x !== "number" || typeof offset.y !== "number") {
      throw new Error(`effects[${index}].offset must be {x,y} numbers`);
    }
    if (typeof raw.radius !== "number") throw new Error(`effects[${index}].radius must be a number`);

    const rgb = parseHexColor(raw.color);
    const alpha = typeof raw.opacity === "number" ? raw.opacity : 1;
    return {
      type,
      color: { r: rgb.r, g: rgb.g, b: rgb.b, a: alpha },
      offset: { x: offset.x, y: offset.y },
      radius: raw.radius,
      spread: typeof raw.spread === "number" ? raw.spread : 0,
      visible: raw.visible === undefined ? true : Boolean(raw.visible),
      blendMode: typeof raw.blendMode === "string" ? (raw.blendMode as BlendMode) : "NORMAL",
    };
  }
  if (type === "LAYER_BLUR" || type === "BACKGROUND_BLUR") {
    if (typeof raw.radius !== "number") throw new Error(`effects[${index}].radius must be a number`);
    return { type, radius: raw.radius, visible: raw.visible === undefined ? true : Boolean(raw.visible) } as Effect;
  }
  throw new Error(`Unsupported effect type at effects[${index}]: ${String(type)}`);
}

async function handleSetEffects(request: ServerRequest) {
  const nodeId = request.nodeIds?.[0];
  if (!nodeId) throw new Error("nodeIds is required for set_effects");

  const node = await requireSceneNode(nodeId);
  if (!("effects" in node)) throw new Error(`Node does not support effects: ${node.id}`);

  const params = request.params ?? {};
  if (!Array.isArray(params.effects)) throw new Error("effects must be an array (pass [] to clear)");

  const built = (params.effects as Array<Record<string, unknown>>).map(buildEffect);
  (node as BlendMixin & { effects: ReadonlyArray<Effect> }).effects = built;

  return { nodeId: node.id, nodeName: node.name, applied: { count: built.length } };
}

async function handleSetStrokeProperties(request: ServerRequest) {
  const nodeId = request.nodeIds?.[0];
  if (!nodeId) throw new Error("nodeIds is required for set_stroke_properties");

  const node = await requireSceneNode(nodeId);
  const params = request.params ?? {};
  const applied: Record<string, unknown> = {};

  if (typeof params.strokeWeight === "number") {
    if (!("strokeWeight" in node)) throw new Error(`Node does not support strokeWeight: ${node.id}`);
    (node as MinimalStrokesMixin).strokeWeight = params.strokeWeight;
    applied.strokeWeight = params.strokeWeight;
  }

  if (params.strokeAlign === "INSIDE" || params.strokeAlign === "OUTSIDE" || params.strokeAlign === "CENTER") {
    if (!("strokeAlign" in node)) throw new Error(`Node does not support strokeAlign: ${node.id}`);
    (node as MinimalStrokesMixin).strokeAlign = params.strokeAlign;
    applied.strokeAlign = params.strokeAlign;
  }

  if (Array.isArray(params.dashPattern)) {
    if (!("dashPattern" in node)) throw new Error(`Node does not support dashPattern: ${node.id}`);
    const pattern = (params.dashPattern as unknown[]).map((n, i) => {
      if (typeof n !== "number" || n < 0) throw new Error(`dashPattern[${i}] must be a non-negative number`);
      return n;
    });
    (node as MinimalStrokesMixin).dashPattern = pattern;
    applied.dashPattern = pattern;
  }

  if (typeof params.strokeCap === "string") {
    if (!("strokeCap" in node)) throw new Error(`Node does not support strokeCap: ${node.id}`);
    (node as SceneNode & { strokeCap: StrokeCap }).strokeCap = params.strokeCap as StrokeCap;
    applied.strokeCap = params.strokeCap;
  }

  if (typeof params.strokeJoin === "string") {
    if (!("strokeJoin" in node)) throw new Error(`Node does not support strokeJoin: ${node.id}`);
    (node as SceneNode & { strokeJoin: StrokeJoin }).strokeJoin = params.strokeJoin as StrokeJoin;
    applied.strokeJoin = params.strokeJoin;
  }

  return { nodeId: node.id, nodeName: node.name, applied };
}

async function handleSetAutoLayout(request: ServerRequest) {
  const nodeId = request.nodeIds?.[0];
  if (!nodeId) throw new Error("nodeIds is required for set_auto_layout");

  const node = await requireSceneNode(nodeId);
  if (!("layoutMode" in node)) throw new Error(`Node does not support auto-layout: ${node.id}`);

  const frame = node as FrameNode;
  const params = request.params ?? {};
  const applied: Record<string, unknown> = {};

  if (params.layoutMode === "NONE" || params.layoutMode === "HORIZONTAL" || params.layoutMode === "VERTICAL") {
    frame.layoutMode = params.layoutMode;
    applied.layoutMode = params.layoutMode;
  }

  if (typeof params.itemSpacing === "number") {
    frame.itemSpacing = params.itemSpacing;
    applied.itemSpacing = params.itemSpacing;
  }
  if (typeof params.counterAxisSpacing === "number") {
    (frame as FrameNode & { counterAxisSpacing: number }).counterAxisSpacing = params.counterAxisSpacing;
    applied.counterAxisSpacing = params.counterAxisSpacing;
  }

  if (typeof params.paddingTop === "number") {
    frame.paddingTop = params.paddingTop;
    applied.paddingTop = params.paddingTop;
  }
  if (typeof params.paddingRight === "number") {
    frame.paddingRight = params.paddingRight;
    applied.paddingRight = params.paddingRight;
  }
  if (typeof params.paddingBottom === "number") {
    frame.paddingBottom = params.paddingBottom;
    applied.paddingBottom = params.paddingBottom;
  }
  if (typeof params.paddingLeft === "number") {
    frame.paddingLeft = params.paddingLeft;
    applied.paddingLeft = params.paddingLeft;
  }

  if (
    params.primaryAxisAlignItems === "MIN" ||
    params.primaryAxisAlignItems === "MAX" ||
    params.primaryAxisAlignItems === "CENTER" ||
    params.primaryAxisAlignItems === "SPACE_BETWEEN"
  ) {
    frame.primaryAxisAlignItems = params.primaryAxisAlignItems;
    applied.primaryAxisAlignItems = params.primaryAxisAlignItems;
  }
  if (
    params.counterAxisAlignItems === "MIN" ||
    params.counterAxisAlignItems === "MAX" ||
    params.counterAxisAlignItems === "CENTER" ||
    params.counterAxisAlignItems === "BASELINE"
  ) {
    frame.counterAxisAlignItems = params.counterAxisAlignItems;
    applied.counterAxisAlignItems = params.counterAxisAlignItems;
  }

  if (params.primaryAxisSizingMode === "FIXED" || params.primaryAxisSizingMode === "AUTO") {
    frame.primaryAxisSizingMode = params.primaryAxisSizingMode;
    applied.primaryAxisSizingMode = params.primaryAxisSizingMode;
  }
  if (params.counterAxisSizingMode === "FIXED" || params.counterAxisSizingMode === "AUTO") {
    frame.counterAxisSizingMode = params.counterAxisSizingMode;
    applied.counterAxisSizingMode = params.counterAxisSizingMode;
  }

  if (params.layoutWrap === "NO_WRAP" || params.layoutWrap === "WRAP") {
    (frame as FrameNode & { layoutWrap: "NO_WRAP" | "WRAP" }).layoutWrap = params.layoutWrap;
    applied.layoutWrap = params.layoutWrap;
  }

  return { nodeId: node.id, nodeName: node.name, applied };
}

async function handleCreateFrame(request: ServerRequest) {
  const params = request.params ?? {};
  const frame = figma.createFrame();

  if (typeof params.name === "string") frame.name = params.name;

  const width = typeof params.width === "number" ? params.width : 100;
  const height = typeof params.height === "number" ? params.height : 100;
  frame.resize(width, height);

  if (typeof params.fillHex === "string") {
    const fillOpacity = typeof params.fillOpacity === "number" ? params.fillOpacity : undefined;
    applySolidFill(frame, params.fillHex, fillOpacity);
  }

  await reparentIfRequested(frame, params.parentId);
  setPosition(frame, params.x, params.y);

  return {
    nodeId: frame.id,
    nodeName: frame.name,
    parentId: frame.parent?.id,
    x: frame.x,
    y: frame.y,
    width: frame.width,
    height: frame.height,
  };
}

async function handleCreateText(request: ServerRequest) {
  const params = request.params ?? {};
  const text = figma.createText();

  const fontFamily = typeof params.fontFamily === "string" ? params.fontFamily : "Inter";
  const fontStyle = typeof params.fontStyle === "string" ? params.fontStyle : "Regular";
  text.fontName = await loadFont(fontFamily, fontStyle);

  if (typeof params.name === "string") text.name = params.name;
  if (typeof params.characters === "string") text.characters = params.characters;
  if (typeof params.fontSize === "number") text.fontSize = params.fontSize;
  if (typeof params.fillHex === "string") {
    const fillOpacity = typeof params.fillOpacity === "number" ? params.fillOpacity : undefined;
    applyTextFill(text, params.fillHex, fillOpacity);
  }

  if (
    params.textAlignHorizontal === "LEFT" ||
    params.textAlignHorizontal === "CENTER" ||
    params.textAlignHorizontal === "RIGHT" ||
    params.textAlignHorizontal === "JUSTIFIED"
  ) {
    text.textAlignHorizontal = params.textAlignHorizontal;
  }

  if (
    params.textAutoResize === "NONE" ||
    params.textAutoResize === "WIDTH_AND_HEIGHT" ||
    params.textAutoResize === "HEIGHT" ||
    params.textAutoResize === "TRUNCATE"
  ) {
    text.textAutoResize = params.textAutoResize;
  }

  setSizeIfSupported(text, params.width, params.height);
  await reparentIfRequested(text, params.parentId);
  setPosition(text, params.x, params.y);

  return {
    nodeId: text.id,
    nodeName: text.name,
    parentId: text.parent?.id,
    characters: text.characters,
    x: text.x,
    y: text.y,
    width: text.width,
    height: text.height,
  };
}

async function handleCreateShape(request: ServerRequest) {
  const params = request.params ?? {};
  const shapeType = params.shapeType;

  let node: SceneNode;
  if (shapeType === "ELLIPSE") {
    node = figma.createEllipse();
  } else if (shapeType === "LINE") {
    node = figma.createLine();
  } else {
    node = figma.createRectangle();
  }

  if (typeof params.name === "string") node.name = params.name;

  setSizeIfSupported(node, params.width, params.height);

  if (typeof params.rotation === "number" && "rotation" in node) {
    node.rotation = params.rotation;
  }

  if (shapeType === "LINE" && typeof params.fillHex === "string") {
    throw new Error("LINE shapes do not support fillHex — use strokeHex instead");
  }
  if (typeof params.fillHex === "string") {
    const fillOpacity = typeof params.fillOpacity === "number" ? params.fillOpacity : undefined;
    applySolidFill(node, params.fillHex, fillOpacity);
  }

  if (shapeType === "LINE" && typeof params.strokeHex !== "string") {
    throw new Error("LINE shapes require strokeHex (lines have no fill, so without a stroke they are invisible)");
  }
  if (typeof params.strokeHex === "string") {
    if (!("strokes" in node)) throw new Error(`Node does not support strokes: ${node.id}`);
    const strokeOpacity = typeof params.strokeOpacity === "number" ? params.strokeOpacity : undefined;
    applySolidFill(node, params.strokeHex, strokeOpacity, "stroke");
  }

  if ("strokeWeight" in node && typeof params.strokeWeight === "number") {
    node.strokeWeight = params.strokeWeight;
  }

  if (typeof params.cornerRadius === "number" && "cornerRadius" in node) {
    node.cornerRadius = params.cornerRadius;
  }

  await reparentIfRequested(node, params.parentId);
  setPosition(node, params.x, params.y);

  return {
    nodeId: node.id,
    nodeName: node.name,
    shapeType,
    parentId: node.parent?.id,
    x: "x" in node ? node.x : undefined,
    y: "y" in node ? node.y : undefined,
    width: "width" in node ? node.width : undefined,
    height: "height" in node ? node.height : undefined,
  };
}

async function handleCreateImage(request: ServerRequest) {
  const params = request.params ?? {};
  if (typeof params.imageBase64 !== "string" || params.imageBase64.length === 0) {
    throw new Error("imageBase64 is required for create_image");
  }

  const image = figma.createImage(decodeBase64Image(params.imageBase64));
  const imageSize = await image.getSizeAsync();
  const node = figma.createRectangle();

  if (typeof params.name === "string") node.name = params.name;

  const aspectRatio = imageSize.width / imageSize.height;
  const width =
    typeof params.width === "number"
      ? params.width
      : typeof params.height === "number"
        ? params.height * aspectRatio
        : imageSize.width;
  const height =
    typeof params.height === "number"
      ? params.height
      : typeof params.width === "number"
        ? params.width / aspectRatio
        : imageSize.height;

  node.resize(width, height);
  node.fills = [{ type: "IMAGE", imageHash: image.hash, scaleMode: params.scaleMode === "FIT" ? "FIT" : "FILL" }];

  if (typeof params.cornerRadius === "number") node.cornerRadius = params.cornerRadius;

  await reparentIfRequested(node, params.parentId);
  setPosition(node, params.x, params.y);

  return {
    nodeId: node.id,
    nodeName: node.name,
    parentId: node.parent?.id,
    x: node.x,
    y: node.y,
    width: node.width,
    height: node.height,
    imageHash: image.hash,
  };
}

async function handleDuplicateNodes(request: ServerRequest) {
  if (!request.nodeIds || request.nodeIds.length === 0) {
    throw new Error("nodeIds is required for duplicate_nodes");
  }

  const duplicates = [];
  for (const nodeId of request.nodeIds) {
    const node = await requireSceneNode(nodeId);
    if (!("clone" in node) || typeof node.clone !== "function") {
      throw new Error(`Node does not support duplication: ${node.id}`);
    }
    const clone = node.clone();
    duplicates.push({ sourceNodeId: node.id, nodeId: clone.id, nodeName: clone.name, parentId: clone.parent?.id });
  }

  return { duplicatedCount: duplicates.length, duplicates };
}

async function handleReparentNodes(request: ServerRequest) {
  if (!request.nodeIds || request.nodeIds.length === 0) {
    throw new Error("nodeIds is required for reparent_nodes");
  }
  const parentId = request.params?.parentId;
  if (typeof parentId !== "string") throw new Error("parentId is required for reparent_nodes");

  const parent = await requireContainerNode(parentId);
  const moved = [];
  for (const nodeId of request.nodeIds) {
    const node = await requireSceneNode(nodeId);
    parent.appendChild(node);
    moved.push({ nodeId: node.id, nodeName: node.name, parentId: node.parent?.id });
  }

  return { movedCount: moved.length, moved };
}

async function handleGroupNodes(request: ServerRequest) {
  if (!request.nodeIds || request.nodeIds.length === 0) {
    throw new Error("nodeIds is required for group_nodes");
  }

  const nodes = await Promise.all(request.nodeIds.map((nodeId) => requireSceneNode(nodeId)));

  const explicitParentId = request.params?.parentId;
  let parent: BaseNode & ChildrenMixin;
  if (typeof explicitParentId === "string") {
    parent = await requireContainerNode(explicitParentId);
  } else {
    const parentIds = new Set(nodes.map((n) => n.parent?.id));
    if (parentIds.size !== 1 || parentIds.has(undefined)) {
      throw new Error("group_nodes requires all nodes to share a parent, or pass parentId explicitly");
    }
    const sharedParent = nodes[0].parent;
    if (!sharedParent || !supportsChildren(sharedParent)) {
      throw new Error("Shared parent does not support children");
    }
    parent = sharedParent;
  }

  const group = figma.group(nodes, parent);
  if (typeof request.params?.name === "string") group.name = request.params.name;

  return {
    nodeId: group.id,
    nodeName: group.name,
    parentId: group.parent?.id,
    childIds: group.children.map((c) => c.id),
  };
}

async function handleUngroupNode(request: ServerRequest) {
  const nodeId = request.nodeIds?.[0];
  if (!nodeId) throw new Error("nodeIds is required for ungroup_node");

  const node = await requireSceneNode(nodeId);
  if (node.type !== "GROUP" && node.type !== "FRAME") {
    throw new Error(`ungroup_node only works on GROUP or FRAME nodes, got ${node.type}`);
  }

  const parentId = node.parent?.id;
  const orphans = figma.ungroup(node as GroupNode | FrameNode);

  return { parentId, orphanIds: orphans.map((o) => o.id) };
}

async function handleSetSelection(request: ServerRequest) {
  const nodes: SceneNode[] = [];
  for (const id of request.nodeIds ?? []) {
    nodes.push(await requireSceneNode(id));
  }
  figma.currentPage.selection = nodes;
  return { selectedCount: nodes.length, selectedIds: nodes.map((n) => n.id) };
}

async function handleScrollAndZoomIntoView(request: ServerRequest) {
  if (!request.nodeIds || request.nodeIds.length === 0) {
    throw new Error("nodeIds is required for scroll_and_zoom_into_view");
  }
  const nodes = await Promise.all(request.nodeIds.map((nodeId) => requireSceneNode(nodeId)));
  figma.viewport.scrollAndZoomIntoView(nodes);
  return { framedCount: nodes.length, framedIds: nodes.map((n) => n.id) };
}

async function handleDeleteNodes(request: ServerRequest) {
  if (request.params?.confirm !== true) throw new Error("delete_nodes requires confirm: true");
  if (!request.nodeIds || request.nodeIds.length === 0) throw new Error("nodeIds is required for delete_nodes");

  const nodes = await Promise.all(request.nodeIds.map((nodeId) => requireSceneNode(nodeId)));
  const deletions = nodes.map((node) => ({ nodeId: node.id, nodeName: node.name, parentId: node.parent?.id }));

  for (const node of nodes) node.remove();

  return { deletedCount: deletions.length, deletions };
}

const HANDLERS: { [K in RequestType]: (request: ServerRequest) => Promise<unknown> } = {
  get_document: handleGetDocument,
  get_selection: handleGetSelection,
  get_node: handleGetNode,
  find_nodes: handleFindNodes,
  get_main_component_name: handleGetMainComponentName,
  get_styles: handleGetStyles,
  get_metadata: async () => handleGetMetadata(),
  get_design_context: handleGetDesignContext,
  get_variable_defs: handleGetVariableDefs,
  get_screenshot: handleGetScreenshot,
  set_node_visibility: handleSetNodeVisibility,
  set_text_content: handleSetTextContent,
  set_text_properties: handleSetTextProperties,
  set_node_properties: handleSetNodeProperties,
  set_solid_fill: handleSetSolidFill,
  set_gradient_fill: handleSetGradientFill,
  set_effects: handleSetEffects,
  set_stroke_properties: handleSetStrokeProperties,
  set_auto_layout: handleSetAutoLayout,
  create_frame: handleCreateFrame,
  create_text: handleCreateText,
  create_shape: handleCreateShape,
  create_image: handleCreateImage,
  duplicate_nodes: handleDuplicateNodes,
  reparent_nodes: handleReparentNodes,
  group_nodes: handleGroupNodes,
  ungroup_node: handleUngroupNode,
  set_selection: handleSetSelection,
  scroll_and_zoom_into_view: handleScrollAndZoomIntoView,
  delete_nodes: handleDeleteNodes,
};

async function handleRequest(request: ServerRequest): Promise<PluginResponse> {
  try {
    if (EDIT_REQUEST_TYPES.has(request.type)) {
      requireDesignEditor(request.type);
    }
    const handler = HANDLERS[request.type];
    if (!handler) throw new Error(`Unknown request type: ${request.type}`);
    const data = await handler(request);
    return { type: request.type, requestId: request.requestId, data };
  } catch (error) {
    return { type: request.type, requestId: request.requestId, error: error instanceof Error ? error.message : String(error) };
  }
}

figma.showUI(__html__, { width: 328, height: 380, themeColors: true });
pushStatus();

figma.on("selectionchange", () => {
  pushStatus();
});

figma.ui.onmessage = async (message) => {
  if (message.type === "ui-ready") {
    pushStatus();
    return;
  }

  if (message.type === "server-request") {
    const response = await handleRequest(message.payload as ServerRequest);
    try {
      figma.ui.postMessage(response);
    } catch (err) {
      figma.ui.postMessage({
        type: response.type,
        requestId: response.requestId,
        error: err instanceof Error ? err.message : String(err),
      });
    }
  }
};
