import { type PeerInfo, type PeerPicker, type PermissionRequest, type PINPresentation, type PINPrompt, type UserAgentUI } from "./ui.js";
export declare class DomUI implements UserAgentUI {
    private readonly root;
    constructor(root?: () => HTMLElement);
    requestPermission(req: PermissionRequest, signal: AbortSignal): Promise<boolean>;
    pickPeer(picker: PeerPicker, signal: AbortSignal): Promise<PeerInfo | null>;
    presentPIN(p: PINPresentation, signal: AbortSignal): void;
    enterPIN(p: PINPrompt, signal: AbortSignal): Promise<string | null>;
    private show;
}
