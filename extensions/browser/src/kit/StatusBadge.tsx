type Status = "connected" | "disconnected" | "pending";

const LABEL: Record<Status, string> = {
  connected: "Bridge connected",
  disconnected: "Bridge unreachable",
  pending: "Checking bridge…"
};

export function StatusBadge({ status, detail }: { status: Status; detail?: string }) {
  return (
    <div className={`fm-status fm-status-${status}`}>
      <span className="fm-dot" />
      <span>{detail ?? LABEL[status]}</span>
    </div>
  );
}
