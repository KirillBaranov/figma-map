import { useEffect, useState } from "react";

export function useToast(timeoutMs = 4000) {
  const [message, setMessage] = useState<string | null>(null);

  useEffect(() => {
    if (!message) return;
    const id = setTimeout(() => setMessage(null), timeoutMs);
    return () => clearTimeout(id);
  }, [message, timeoutMs]);

  return { message, show: setMessage };
}

export function Toast({ message }: { message: string | null }) {
  if (!message) return null;
  return <div className="fm-toast">{message}</div>;
}
