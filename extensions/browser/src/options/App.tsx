import { useEffect, useState } from "react";
import { Button, CrosshairIcon, TextField } from "../kit";
import { DEFAULT_BRIDGE_URL, getOptions, setOptions, type ExtensionOptions } from "../lib/options";

export function App() {
  const [values, setValues] = useState<ExtensionOptions>({
    bridgeUrl: DEFAULT_BRIDGE_URL,
    enabledHosts: [],
    barPos: null
  });
  const [saved, setSaved] = useState(false);

  useEffect(() => {
    getOptions().then(setValues);
  }, []);

  function update<K extends keyof ExtensionOptions>(key: K, value: string) {
    setValues((v) => ({ ...v, [key]: value }));
  }

  async function save() {
    await setOptions(values);
    setSaved(true);
    setTimeout(() => setSaved(false), 1500);
  }

  return (
    <div className="fm-reset fm-options">
      <div className="fm-options-brand">
        <div className="fm-options-logo">
          <CrosshairIcon size={18} />
        </div>
        <div>
          <h1>figma-map Capture</h1>
          <div className="fm-options-subtitle">Settings</div>
        </div>
      </div>
      <div className="fm-panel fm-options-panel">
        <TextField
          label="Bridge URL"
          placeholder={DEFAULT_BRIDGE_URL}
          value={values.bridgeUrl}
          onChange={(e) => update("bridgeUrl", e.target.value)}
          hint="The figma-map bridge server's base URL (figma-map.yaml's &quot;bridge&quot; by default)."
        />
        <TextField
          label="Linked Figma node id (optional)"
          placeholder="123:456"
          value={values.figmaNodeId ?? ""}
          onChange={(e) => update("figmaNodeId", e.target.value)}
          hint="If set, every capture is linked to this node so the agent can diff against it directly."
        />
        <TextField
          label="Figma file key (optional)"
          value={values.fileKey ?? ""}
          onChange={(e) => update("fileKey", e.target.value)}
          hint="Only needed if more than one Figma file is connected to the bridge."
        />
        <div className="fm-options-footer">
          <Button onClick={save}>Save</Button>
          {saved && <span className="fm-options-saved">Saved</span>}
        </div>
      </div>
    </div>
  );
}
