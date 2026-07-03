import { useCallback, useEffect, useRef, useState } from "react";

export interface Pos {
  x: number;
  y: number;
}

interface DragOrigin {
  pointerX: number;
  pointerY: number;
  x: number;
  y: number;
}

export interface Draggable {
  dragging: boolean;
  /** Wire to onMouseDown on whatever should move. `current` is the position
   *  to drag from — pass it explicitly when it isn't the hook's own `pos`
   *  state (e.g. reading a DOMRect on the first drag of an element whose
   *  position hasn't been set yet). */
  startDrag: (e: React.MouseEvent, current?: Pos) => void;
}

// Pointer-drag mechanics (grab at a point, track mousemove deltas, release
// on mouseup) shared by the overlay image and the panel's title bar — both
// need this, but neither owns the position: that's persisted/reset by
// useCompareState, so this hook takes the setter rather than owning state
// itself. Previously duplicated as two near-identical effect/callback sets.
export function useDraggable(setPos: (pos: Pos) => void, fallback: Pos): Draggable {
  const [dragging, setDragging] = useState(false);
  const origin = useRef<DragOrigin | null>(null);

  const onDrag = useCallback(
    (e: MouseEvent) => {
      if (!origin.current) return;
      const { pointerX, pointerY, x, y } = origin.current;
      setPos({ x: x + (e.clientX - pointerX), y: y + (e.clientY - pointerY) });
    },
    [setPos]
  );

  const stopDrag = useCallback(() => {
    origin.current = null;
    setDragging(false);
    window.removeEventListener("mousemove", onDrag);
    window.removeEventListener("mouseup", stopDrag);
  }, [onDrag]);

  const startDrag = useCallback(
    (e: React.MouseEvent, current?: Pos) => {
      e.preventDefault();
      const base = current ?? fallback;
      origin.current = { pointerX: e.clientX, pointerY: e.clientY, x: base.x, y: base.y };
      setDragging(true);
      window.addEventListener("mousemove", onDrag);
      window.addEventListener("mouseup", stopDrag);
    },
    [fallback, onDrag, stopDrag]
  );

  useEffect(() => stopDrag, [stopDrag]);

  return { dragging, startDrag };
}
