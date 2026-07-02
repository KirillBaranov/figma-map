import { createRoot } from "react-dom/client";
import { App } from "./App";
import "../kit/tokens.css";
import "./popup.css";

createRoot(document.getElementById("root")!).render(<App />);
