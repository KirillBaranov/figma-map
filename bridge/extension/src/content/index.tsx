// Content script entry: renders a persistent floating button on every page
// so a human can pick a DOM region and ship it to figma-map's bridge as a
// "flagged issue" for an agent to pick up. Captures ground truth only (bbox,
// selector, screenshot) — no diffing or matching happens here (ADR-0001).
// UI renders into an isolated shadow root so it never collides with the
// host page's CSS/JS.
import { createRoot } from "react-dom/client";
import { createRef } from "react";
import { App, type AppHandle } from "./App";
import { cssSelector } from "./selector";
import { sendExtensionMessage } from "../protocol";
import kitCss from "../kit/tokens.css?raw";
import overlayCss from "./overlay.css?raw";

const appRef = createRef<AppHandle>();
let selecting = false;
let hovered: HTMLElement | null = null;
let revertCursor: (() => void) | null = null;
let highlightEl: HTMLDivElement | null = null;

function mount(): void {
  const host = document.createElement("div");
  host.id = "figma-map-host";
  document.documentElement.appendChild(host);
  const shadow = host.attachShadow({ mode: "open" });
  const style = document.createElement("style");
  style.textContent = kitCss + "\n" + overlayCss;
  shadow.appendChild(style);

  highlightEl = document.createElement("div");
  highlightEl.className = "fm-highlight";
  shadow.appendChild(highlightEl);

  const mountPoint = document.createElement("div");
  shadow.appendChild(mountPoint);
  createRoot(mountPoint).render(
    <App ref={appRef} onToggleSelect={toggleSelectMode} onStopSelect={stopSelectMode} onOpenSettings={openSettings} />
  );
}

function positionHighlight(el: HTMLElement): void {
  if (!highlightEl) return;
  const rect = el.getBoundingClientRect();
  highlightEl.style.left = `${rect.left}px`;
  highlightEl.style.top = `${rect.top}px`;
  highlightEl.style.width = `${rect.width}px`;
  highlightEl.style.height = `${rect.height}px`;
  highlightEl.classList.add("fm-highlight-visible");
}

function hideHighlight(): void {
  highlightEl?.classList.remove("fm-highlight-visible");
}

function clearHover(): void {
  revertCursor?.();
  revertCursor = null;
  hovered = null;
  hideHighlight();
}

function onHover(e: MouseEvent): void {
  const target = e.target as HTMLElement;
  if (target === hovered) return;
  revertCursor?.();
  hovered = target;
  const prevCursor = target.style.cursor;
  target.style.cursor = "crosshair";
  revertCursor = () => {
    target.style.cursor = prevCursor;
  };
  positionHighlight(target);
}

function onScrollOrResize(): void {
  if (hovered) positionHighlight(hovered);
}

function onKeyDown(e: KeyboardEvent): void {
  if (e.key === "Escape") stopSelectMode();
}

function onClick(e: MouseEvent): void {
  // Ignore clicks on our own widget (it lives in a separate shadow host, so
  // e.target here is always document.body via composedPath in that case).
  if (e.composedPath().some((n) => n instanceof HTMLElement && n.id === "figma-map-host")) {
    return;
  }
  e.preventDefault();
  e.stopPropagation();
  const el = e.target as HTMLElement;
  const rect = el.getBoundingClientRect();
  const selector = cssSelector(el);
  stopSelectMode();

  appRef.current?.showPanel({
    selector,
    x: Math.min(e.clientX, window.innerWidth - 280),
    y: Math.min(e.clientY, window.innerHeight - 220),
    onSend: (note) => sendCapture(rect, selector, note)
  });
}

function startSelectMode(): void {
  if (selecting) return;
  selecting = true;
  appRef.current?.setSelecting(true);
  document.addEventListener("mouseover", onHover, true);
  document.addEventListener("click", onClick, true);
  document.addEventListener("keydown", onKeyDown, true);
  window.addEventListener("scroll", onScrollOrResize, true);
  window.addEventListener("resize", onScrollOrResize);
}

function stopSelectMode(): void {
  selecting = false;
  appRef.current?.setSelecting(false);
  document.removeEventListener("mouseover", onHover, true);
  document.removeEventListener("click", onClick, true);
  document.removeEventListener("keydown", onKeyDown, true);
  window.removeEventListener("scroll", onScrollOrResize, true);
  window.removeEventListener("resize", onScrollOrResize);
  clearHover();
}

function toggleSelectMode(): void {
  if (selecting) {
    stopSelectMode();
  } else {
    startSelectMode();
  }
}

function openSettings(): void {
  sendExtensionMessage({ type: "FIGMA_MAP_OPEN_OPTIONS" });
}

function sendCapture(rect: DOMRect, selector: string, note: string): void {
  sendExtensionMessage({
    type: "FIGMA_MAP_CAPTURE",
    bbox: { x: rect.left, y: rect.top, width: rect.width, height: rect.height },
    dpr: window.devicePixelRatio || 1,
    selector,
    tabUrl: location.href,
    note
  }).then((response) => {
    appRef.current?.showToast(
      response?.ok ? "Sent to figma-map ✓" : `figma-map: ${response?.error || "failed to send"}`
    );
  });
}

chrome.runtime.onMessage.addListener((msg, _sender, sendResponse) => {
  if (msg.type === "FIGMA_MAP_TOGGLE_SELECT") {
    toggleSelectMode();
    sendResponse({ selecting });
  }
});

mount();
