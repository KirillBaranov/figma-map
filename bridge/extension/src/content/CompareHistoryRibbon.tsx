import { useEffect, useState } from "react";
import { CloseIcon, PinIcon, Tooltip } from "../kit";
import {
  deleteCompareHistory,
  listCompareHistory,
  pinCompareHistory,
  type CompareHistoryEntryData
} from "../lib/compareSession";

interface CompareHistoryRibbonProps {
  onSelect: (entry: CompareHistoryEntryData) => void;
}

// Past compare sessions (pushed only when a new reference image loads — see
// content/hooks/useFigmaCompare.ts) as a row of thumbnails above the fetch
// form / active comparison. Fetches once on mount, same "no polling loop"
// pattern as IssuesWindow — a history entry only changes when this window's
// own actions (fetch/paste/drop/pin/delete) cause it to.
export function CompareHistoryRibbon({ onSelect }: CompareHistoryRibbonProps) {
  const [entries, setEntries] = useState<CompareHistoryEntryData[]>([]);

  useEffect(() => {
    listCompareHistory()
      .then(setEntries)
      .catch(() => {});
  }, []);

  async function onPinToggle(entry: CompareHistoryEntryData) {
    try {
      const updated = await pinCompareHistory(entry.id, !entry.pinned);
      setEntries((prev) => prev.map((e) => (e.id === updated.id ? updated : e)));
    } catch {
      // best-effort — a failed pin toggle just leaves the entry as-is
    }
  }

  async function onDelete(id: string) {
    try {
      await deleteCompareHistory(id);
      setEntries((prev) => prev.filter((e) => e.id !== id));
    } catch {
      // best-effort — a failed delete just leaves the entry in the ribbon
    }
  }

  if (entries.length === 0) return null;

  return (
    <div className="fm-history-ribbon">
      {entries.map((entry) => (
        <div className="fm-history-thumb-wrap" key={entry.id}>
          <img
            className="fm-history-thumb"
            src={entry.image}
            alt=""
            onClick={() => onSelect(entry)}
          />
          <Tooltip label={entry.pinned ? "Unpin" : "Pin"}>
            <button
              type="button"
              className={`fm-history-pin ${entry.pinned ? "fm-history-pin-active" : ""}`}
              onClick={() => onPinToggle(entry)}
              aria-label={entry.pinned ? "Unpin" : "Pin"}
            >
              <PinIcon fill={entry.pinned ? "currentColor" : "none"} />
            </button>
          </Tooltip>
          <Tooltip label="Remove">
            <button
              type="button"
              className="fm-history-remove"
              onClick={() => onDelete(entry.id)}
              aria-label="Remove"
            >
              <CloseIcon />
            </button>
          </Tooltip>
        </div>
      ))}
    </div>
  );
}
