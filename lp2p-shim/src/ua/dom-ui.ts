// A minimal in-page stand-in for browser chrome: permission prompt, device
// picker and PIN dialogs, drawn in a shadow root so page styles don't leak
// in. A real browser draws these outside the page, where the page cannot
// fake or click them; the shim cannot, which is one more reason it offers no
// security.

import { peerLabel, type PeerInfo, type PeerPicker, type PermissionRequest, type PINPresentation, type PINPrompt, type UserAgentUI } from "./ui.ts";

const STYLE = `
:host { all: initial; }
.backdrop { position: fixed; inset: 0; background: rgb(0 0 0 / .25); display: flex;
  align-items: flex-start; justify-content: center; z-index: 2147483647;
  font: 14px/1.4 system-ui, -apple-system, "Segoe UI", sans-serif; }
.dialog { margin-top: 12px; background: #fff; color: #1f1f1f; border-radius: 10px; padding: 16px;
  width: min(360px, calc(100vw - 32px)); box-shadow: 0 8px 28px rgb(0 0 0 / .3); box-sizing: border-box; }
.badge { font-size: 11px; color: #8a4b00; background: #fff2d6; border-radius: 4px; padding: 1px 6px;
  display: inline-block; margin-bottom: 8px; }
h2 { font-size: 15px; margin: 0 0 8px; font-weight: 600; word-break: break-word; }
p { margin: 0 0 12px; color: #444; }
ul { list-style: none; margin: 0 0 12px; padding: 0; max-height: 220px; overflow: auto;
  border: 1px solid #ddd; border-radius: 6px; }
li button { all: unset; display: block; width: 100%; box-sizing: border-box; padding: 8px 10px; cursor: pointer; }
li button:hover, li button:focus-visible { background: #eef3fd; }
li + li { border-top: 1px solid #eee; }
.empty { padding: 10px; color: #777; }
.pin { font: 600 28px/1.2 ui-monospace, monospace; letter-spacing: 2px; text-align: center; margin: 8px 0 12px; }
input { width: 100%; box-sizing: border-box; font: 18px ui-monospace, monospace; padding: 6px 8px;
  margin-bottom: 12px; border: 1px solid #bbb; border-radius: 6px; }
.row { display: flex; gap: 8px; justify-content: flex-end; }
.row button { font: inherit; padding: 6px 14px; border-radius: 6px; border: 1px solid #bbb; background: #fff; cursor: pointer; }
.row button.primary { background: #1a73e8; border-color: #1a73e8; color: #fff; }
`;

type Build = (dialog: HTMLElement, done: () => void) => void;

export class DomUI implements UserAgentUI {
  private readonly root: () => HTMLElement;

  constructor(root: () => HTMLElement = () => document.body ?? document.documentElement) {
    this.root = root;
  }

  requestPermission(req: PermissionRequest, signal: AbortSignal): Promise<boolean> {
    return new Promise((resolve) => {
      this.show(signal, () => resolve(false), (d, done) => {
        const what =
          req.permission === "advertise"
            ? `make this device discoverable as “${req.nickname || "Unnamed device"}”`
            : `connect with ${req.peer ? peerLabel(req.peer) : "a device"}`;
        d.append(el("h2", `${req.origin} wants to ${what}`));
        d.append(el("p", req.permission === "advertise"
          ? "Other devices nearby will be able to ask to connect."
          : "The device will be able to exchange data with this page."));
        d.append(buttons([
          ["Block", false, () => { done(); resolve(false); }],
          ["Allow", true, () => { done(); resolve(true); }],
        ]));
      });
    });
  }

  pickPeer(picker: PeerPicker, signal: AbortSignal): Promise<PeerInfo | null> {
    return new Promise((resolve) => {
      this.show(signal, () => resolve(null), (d, done) => {
        d.append(el("h2", `${picker.origin} wants to connect to a device`));
        const list = el("ul");
        const render = (): void => {
          list.replaceChildren();
          if (picker.peers.length === 0) list.append(el("li", "Looking for devices…", "empty"));
          for (const p of picker.peers) {
            const b = el("button", peerLabel(p));
            b.addEventListener("click", () => { picker.onchange = null; done(); resolve(p); });
            const li = el("li");
            li.append(b);
            list.append(li);
          }
        };
        picker.onchange = render;
        render();
        d.append(list);
        d.append(buttons([["Cancel", false, () => { picker.onchange = null; done(); resolve(null); }]]));
      });
    });
  }

  presentPIN(p: PINPresentation, signal: AbortSignal): void {
    this.show(signal, () => {}, (d) => {
      d.append(el("h2", `Pair with ${peerLabel(p.peer)}`));
      d.append(el("p", "Enter this PIN on the other device:"));
      d.append(el("div", p.pin, "pin"));
    });
  }

  enterPIN(p: PINPrompt, signal: AbortSignal): Promise<string | null> {
    return new Promise((resolve) => {
      this.show(signal, () => resolve(null), (d, done) => {
        d.append(el("h2", `Pair with ${peerLabel(p.peer)}`));
        d.append(el("p", "Enter the PIN shown on the other device:"));
        const input = el("input") as HTMLInputElement;
        input.inputMode = "numeric";
        input.autocomplete = "off";
        input.placeholder = "123-456-789";
        const ok = (): void => { done(); resolve(input.value); };
        input.addEventListener("keydown", (e) => e.key === "Enter" && ok());
        d.append(input);
        d.append(buttons([
          ["Cancel", false, () => { done(); resolve(null); }],
          ["Pair", true, ok],
        ]));
        queueMicrotask(() => input.focus());
      });
    });
  }

  private show(signal: AbortSignal, onAbort: () => void, build: Build): void {
    if (signal.aborted) {
      onAbort();
      return;
    }
    const host = document.createElement("lp2p-shim-dialog");
    const shadow = host.attachShadow({ mode: "open" });
    const style = document.createElement("style");
    style.textContent = STYLE;
    const backdrop = el("div", undefined, "backdrop");
    const dialog = el("div", undefined, "dialog");
    dialog.setAttribute("role", "dialog");
    dialog.setAttribute("aria-modal", "true");
    dialog.append(el("span", "LP2P shim · simulated browser UI", "badge"));
    backdrop.append(dialog);
    shadow.append(style, backdrop);
    let removed = false;
    const done = (): void => {
      if (removed) return;
      removed = true;
      signal.removeEventListener("abort", aborted);
      host.remove();
    };
    const aborted = (): void => {
      if (removed) return;
      done();
      onAbort();
    };
    signal.addEventListener("abort", aborted, { once: true });
    build(dialog, done);
    this.root().append(host);
  }
}

function el(tag: string, text?: string, cls?: string): HTMLElement {
  const e = document.createElement(tag);
  if (text !== undefined) e.textContent = text;
  if (cls) e.className = cls;
  return e;
}

function buttons(spec: [label: string, primary: boolean, onclick: () => void][]): HTMLElement {
  const row = el("div", undefined, "row");
  for (const [label, primary, onclick] of spec) {
    const b = el("button", label, primary ? "primary" : undefined);
    b.addEventListener("click", onclick);
    row.append(b);
  }
  return row;
}
