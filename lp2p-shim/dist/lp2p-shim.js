/*! lp2p-shim 0.1.0 - experimental Local Peer-to-Peer API polyfill. NOT SECURE: no real authentication or encryption. https://github.com/backkem/go-lp2p/tree/main/lp2p-shim */
"use strict";
(() => {
  var __defProp = Object.defineProperty;
  var __export = (target, all) => {
    for (var name in all)
      __defProp(target, name, { get: all[name], enumerable: true });
  };

  // src/index.ts
  var index_exports = {};
  __export(index_exports, {
    AutoUI: () => AutoUI,
    BaseConnection: () => BaseConnection,
    BaseStream: () => BaseStream,
    BroadcastChannelSignaler: () => BroadcastChannelSignaler,
    ControlChannel: () => ControlChannel,
    DataChannelEndpoint: () => DataChannelEndpoint,
    DomUI: () => DomUI,
    LP2PConnection: () => LP2PConnection,
    LP2PConnectionEvent: () => LP2PConnectionEvent,
    LP2PDataChannel: () => LP2PDataChannel,
    LP2PDataChannelEvent: () => LP2PDataChannelEvent,
    LP2PReceiver: () => LP2PReceiver,
    LP2PRequest: () => LP2PRequest,
    ManualSignaler: () => ManualSignaler,
    PortSignaler: () => PortSignaler,
    PostMessageTransport: () => PostMessageTransport,
    ProtocolError: () => ProtocolError,
    Rendezvous: () => Rendezvous,
    Session: () => Session,
    UserAgent: () => UserAgent,
    WebRTCTransport: () => WebRTCTransport,
    configure: () => configure,
    decodeCBOR: () => decodeCBOR,
    decodeVarint: () => decodeVarint,
    encodeCBOR: () => encodeCBOR,
    encodeVarint: () => encodeVarint,
    getUserAgent: () => getUserAgent,
    install: () => install,
    interfaces: () => interfaces,
    messages: () => messages_exports,
    peerLabel: () => peerLabel,
    version: () => version
  });

  // src/transport/base.ts
  var Inbox = class {
    items = [];
    fn = null;
    get handler() {
      return this.fn;
    }
    set handler(fn) {
      this.fn = fn;
      while (this.fn && this.items.length > 0) {
        this.fn(this.items.shift());
      }
    }
    get empty() {
      return this.items.length === 0;
    }
    push(item) {
      if (this.fn && this.items.length === 0) this.fn(item);
      else this.items.push(item);
    }
    clear() {
      this.items = [];
    }
  };
  var BaseStream = class {
    inbox = new Inbox();
    closeHandler = null;
    closeFired = false;
    isClosed = false;
    id;
    constructor(id) {
      this.id = id;
    }
    get closed() {
      return this.isClosed;
    }
    get onmessage() {
      return this.inbox.handler;
    }
    set onmessage(fn) {
      this.inbox.handler = fn;
      this.maybeFireClose();
    }
    get onclose() {
      return this.closeHandler;
    }
    set onclose(fn) {
      this.closeHandler = fn;
      this.maybeFireClose();
    }
    close() {
      if (this.isClosed) return;
      this.isClosed = true;
      this.doClose();
      this.maybeFireClose();
    }
    /** For implementations: deliver a received message. */
    deliver(message) {
      if (!this.isClosed) this.inbox.push(message);
    }
    /**
     * For implementations: the peer closed the stream or the connection went
     * away. Public so connections can close their streams; not for API users.
     */
    remoteClosed() {
      if (this.isClosed) return;
      this.isClosed = true;
      this.maybeFireClose();
    }
    maybeFireClose() {
      if (!this.isClosed || this.closeFired || !this.closeHandler) return;
      if (!this.inbox.empty) return;
      this.closeFired = true;
      this.closeHandler();
    }
  };
  var live = /* @__PURE__ */ new Set();
  var pageHideHooked = false;
  function closeOnPageHide(c) {
    live.add(c);
    if (pageHideHooked) return;
    pageHideHooked = true;
    globalThis.addEventListener?.("pagehide", () => {
      for (const conn of [...live]) conn.close("page closed");
    });
  }
  var BaseConnection = class {
    incoming = new Inbox();
    closeHandler = null;
    closeReason = null;
    closeFired = false;
    streams = /* @__PURE__ */ new Map();
    remoteId;
    constructor(remoteId) {
      this.remoteId = remoteId;
      closeOnPageHide(this);
    }
    get closed() {
      return this.closeReason !== null;
    }
    get onstream() {
      return this.incoming.handler;
    }
    set onstream(fn) {
      this.incoming.handler = fn;
    }
    get onclose() {
      return this.closeHandler;
    }
    set onclose(fn) {
      this.closeHandler = fn;
      this.maybeFireClose();
    }
    close(reason = "closed locally") {
      if (this.closed) return;
      this.closeReason = reason;
      this.doClose(reason);
      this.teardown();
    }
    /** For implementations: a stream the peer opened. */
    accept(stream) {
      this.streams.set(stream.id, stream);
      this.incoming.push(stream);
    }
    /** For implementations: the peer or the medium closed the connection. */
    remoteClosed(reason) {
      if (this.closed) return;
      this.closeReason = reason;
      this.teardown();
    }
    teardown() {
      live.delete(this);
      this.incoming.clear();
      for (const s of this.streams.values()) {
        if (s instanceof BaseStream) s.remoteClosed();
        else s.close();
      }
      this.streams.clear();
      this.maybeFireClose();
    }
    maybeFireClose() {
      if (this.closeReason === null || this.closeFired || !this.closeHandler) return;
      this.closeFired = true;
      this.closeHandler(this.closeReason);
    }
  };
  function randomId(bytes2 = 8) {
    const b = new Uint8Array(bytes2);
    crypto.getRandomValues(b);
    return Array.from(b, (x) => x.toString(16).padStart(2, "0")).join("");
  }

  // src/transport/rendezvous.ts
  var Rendezvous = class {
    ads = /* @__PURE__ */ new Map();
    handlers = /* @__PURE__ */ new Map();
    discoverers = /* @__PURE__ */ new Set();
    /**
     * Recent announcements, replayed to new discoveries: on a manual medium the
     * user may paste a peer's code before opening the picker.
     */
    recent = /* @__PURE__ */ new Map();
    queryInterval;
    ttl;
    onPageHide = () => {
      for (const id of this.ads.keys()) this.signaler.post({ t: "bye", from: id });
    };
    signaler;
    constructor(signaler, options = {}) {
      this.signaler = signaler;
      this.queryInterval = options.queryInterval ?? 1e3;
      this.ttl = options.ttl ?? 3500;
      signaler.onmessage = (m) => this.receive(m);
      globalThis.addEventListener?.("pagehide", this.onPageHide);
    }
    /** Advertises ad until closed; addressed messages for ad.id go to onmessage. */
    advertise(ad, onmessage) {
      this.ads.set(ad.id, ad);
      const reg = this.register(ad.id, onmessage);
      this.announce(ad);
      return {
        close: () => {
          if (!this.ads.delete(ad.id)) return;
          reg.close();
          this.signaler.post({ t: "bye", from: ad.id });
        }
      };
    }
    /** Receives messages addressed to id until closed. */
    register(id, onmessage) {
      this.handlers.set(id, onmessage);
      return { close: () => void (this.handlers.get(id) === onmessage && this.handlers.delete(id)) };
    }
    send(message, transfer) {
      this.signaler.post(message, transfer);
    }
    discover(onevent) {
      const peers = /* @__PURE__ */ new Map();
      const onmessage = (m) => {
        if (m.t === "announce") {
          const peer = { id: m.from, nickname: typeof m["nickname"] === "string" ? m["nickname"] : "" };
          const prev = peers.get(peer.id);
          peers.set(peer.id, { peer, seen: Date.now() });
          if (!prev) onevent({ kind: "added", peer });
          else if (prev.peer.nickname !== peer.nickname) onevent({ kind: "updated", peer });
        } else if (m.t === "bye") {
          const prev = peers.get(m.from);
          if (prev) {
            peers.delete(m.from);
            onevent({ kind: "removed", peer: prev.peer });
          }
        }
      };
      this.discoverers.add(onmessage);
      const fresh = Date.now() - this.ttl;
      for (const r of this.recent.values()) {
        if (!this.signaler.live || r.seen >= fresh) onmessage(r.message);
      }
      const query = () => this.signaler.post({ t: "query", from: "" });
      query();
      let timer;
      if (this.signaler.live) {
        timer = setInterval(() => {
          const cutoff = Date.now() - this.ttl;
          for (const [id, p] of peers) {
            if (p.seen < cutoff) {
              peers.delete(id);
              onevent({ kind: "removed", peer: p.peer });
            }
          }
          query();
        }, this.queryInterval);
      }
      return {
        close: () => {
          this.discoverers.delete(onmessage);
          if (timer !== void 0) clearInterval(timer);
        }
      };
    }
    close() {
      for (const id of this.ads.keys()) this.signaler.post({ t: "bye", from: id });
      this.ads.clear();
      this.handlers.clear();
      this.discoverers.clear();
      globalThis.removeEventListener?.("pagehide", this.onPageHide);
      this.signaler.close();
    }
    announce(ad) {
      this.signaler.post({ t: "announce", from: ad.id, nickname: ad.nickname });
    }
    receive(m) {
      if (m.to !== void 0) {
        this.handlers.get(m.to)?.(m);
        return;
      }
      switch (m.t) {
        case "query":
          for (const ad of this.ads.values()) this.announce(ad);
          break;
        case "announce":
        case "bye":
          if (m.t === "announce") this.recent.set(m.from, { message: m, seen: Date.now() });
          else this.recent.delete(m.from);
          for (const d of this.discoverers) d(m);
          break;
      }
    }
  };

  // src/transport/signaling.ts
  function isSignalMessage(v) {
    return typeof v === "object" && v !== null && typeof v.t === "string" && typeof v.from === "string";
  }
  var BroadcastChannelSignaler = class {
    canTransfer = false;
    live = true;
    onmessage = null;
    channel;
    name;
    constructor(name = "lp2p-shim") {
      this.name = name;
      this.channel = new BroadcastChannel(name);
      this.channel.onmessage = (e) => {
        if (isSignalMessage(e.data)) this.onmessage?.(e.data);
      };
    }
    post(message) {
      this.channel.postMessage(message);
    }
    close() {
      this.channel.close();
    }
  };
  var PortSignaler = class {
    canTransfer = true;
    live = true;
    onmessage = null;
    listener = (e) => {
      const d = e.data;
      if (typeof d === "object" && d !== null && "lp2pSignal" in d && isSignalMessage(d.lp2pSignal)) {
        const msg = d.lp2pSignal;
        if (e.ports.length > 0) msg["port"] = e.ports[0];
        this.onmessage?.(msg);
      }
    };
    port;
    constructor(port) {
      this.port = port;
      port.addEventListener("message", this.listener);
      port.start?.();
    }
    post(message, transfer = []) {
      const { port, ...rest } = message;
      const ports = port ? [port] : [];
      this.port.postMessage({ lp2pSignal: rest }, [...ports, ...transfer.filter((t) => t !== port)]);
    }
    close() {
      this.port.removeEventListener("message", this.listener);
    }
  };
  var ManualSignaler = class _ManualSignaler {
    canTransfer = false;
    live = false;
    onmessage = null;
    shown = /* @__PURE__ */ new Set();
    /** show is called with each code the user must copy to the other device. */
    show;
    constructor(show) {
      this.show = show;
    }
    post(message) {
      if (message.t === "query" || message.t === "bye") return;
      const code = _ManualSignaler.encode(message);
      if (this.shown.has(code)) return;
      this.shown.add(code);
      this.show(code, message);
    }
    /** Feeds a code copied from the other device. Throws on garbage. */
    paste(code) {
      this.onmessage?.(_ManualSignaler.decode(code));
    }
    close() {
    }
    static encode(message) {
      const bytes2 = new TextEncoder().encode(JSON.stringify(message));
      let bin = "";
      for (const b of bytes2) bin += String.fromCharCode(b);
      return "LP2P:" + btoa(bin);
    }
    static decode(code) {
      const trimmed = code.trim().replace(/^LP2P:/, "").replace(/\s+/g, "");
      const bin = atob(trimmed);
      const bytes2 = Uint8Array.from(bin, (c) => c.charCodeAt(0));
      const msg = JSON.parse(new TextDecoder().decode(bytes2));
      if (!isSignalMessage(msg)) throw new TypeError("not an LP2P signaling code");
      return msg;
    }
  };

  // src/transport/postmessage.ts
  var PortStream = class extends BaseStream {
    conn;
    constructor(id, conn) {
      super(id);
      this.conn = conn;
    }
    send(message) {
      if (this.closed) throw new DOMException("stream is closed", "InvalidStateError");
      this.conn.post({ k: "msg", s: this.id, d: message });
    }
    doClose() {
      this.conn.post({ k: "close", s: this.id });
      this.conn.forget(this.id);
    }
    receive(d) {
      this.deliver(d);
    }
  };
  var PortConnection = class extends BaseConnection {
    nextStreamId;
    port;
    constructor(remoteId, port, dialer) {
      super(remoteId);
      this.port = port;
      this.nextStreamId = dialer ? 0 : 1;
      port.onmessage = (e) => this.onFrame(e.data);
    }
    openStream() {
      if (this.closed) throw new DOMException("connection is closed", "InvalidStateError");
      const s = new PortStream(this.nextStreamId, this);
      this.nextStreamId += 2;
      this.streams.set(s.id, s);
      this.post({ k: "open", s: s.id });
      return s;
    }
    post(frame) {
      if (!this.closed) this.port.postMessage(frame);
    }
    forget(id) {
      this.streams.delete(id);
    }
    doClose(reason) {
      this.port.postMessage({ k: "bye", reason });
      this.port.close();
    }
    onFrame(f) {
      switch (f.k) {
        case "open":
          this.accept(new PortStream(f.s, this));
          break;
        case "msg": {
          const s = this.streams.get(f.s);
          if (s instanceof PortStream) s.receive(f.d);
          break;
        }
        case "close": {
          const s = this.streams.get(f.s);
          this.streams.delete(f.s);
          if (s instanceof PortStream) s.remoteClosed();
          break;
        }
        case "bye":
          this.remoteClosed(`closed by peer: ${f.reason}`);
          this.port.close();
          break;
      }
    }
  };
  var PostMessageTransport = class {
    name = "postmessage";
    rendezvous;
    canTransfer;
    connectTimeout;
    constructor(options = {}) {
      const signaler = options.signaler ?? new BroadcastChannelSignaler(options.channelName ?? "lp2p-shim");
      this.canTransfer = signaler.canTransfer;
      this.rendezvous = new Rendezvous(signaler, options);
      this.connectTimeout = options.connectTimeout ?? 5e3;
    }
    listen(ad, onconnection) {
      return this.rendezvous.advertise(ad, (m) => {
        if (m.t !== "connect" || typeof m["conn"] !== "string") return;
        const port = m["port"] ?? new BroadcastChannel(connChannelName(m["conn"]));
        port.postMessage({ k: "accept" });
        onconnection(new PortConnection(m.from, port, false));
      });
    }
    discover(onevent) {
      return this.rendezvous.discover(onevent);
    }
    connect(peerId, local, options = {}) {
      const conn = randomId();
      let port;
      const msg = { t: "connect", from: local.id, to: peerId, conn, nickname: local.nickname };
      const transfer = [];
      if (this.canTransfer) {
        const mc = new MessageChannel();
        port = mc.port1;
        msg["port"] = mc.port2;
        transfer.push(mc.port2);
      } else {
        port = new BroadcastChannel(connChannelName(conn));
      }
      return new Promise((resolve, reject) => {
        const fail = (err) => {
          clearTimeout(timer);
          options.signal?.removeEventListener("abort", onAbort);
          port.onmessage = null;
          port.close();
          reject(err);
        };
        const onAbort = () => fail(options.signal?.reason);
        const timer = setTimeout(
          () => fail(new DOMException(`peer ${peerId} did not answer`, "NetworkError")),
          this.connectTimeout
        );
        options.signal?.addEventListener("abort", onAbort);
        port.onmessage = (e) => {
          if (e.data.k !== "accept") return;
          clearTimeout(timer);
          options.signal?.removeEventListener("abort", onAbort);
          resolve(new PortConnection(peerId, port, true));
        };
        this.rendezvous.send(msg, transfer);
      });
    }
    /** Stops all advertising and discovery and releases the signaler. */
    close() {
      this.rendezvous.close();
    }
  };
  function connChannelName(conn) {
    return `lp2p-shim/conn/${conn}`;
  }

  // src/ua/ui.ts
  function peerLabel(p) {
    if (!p.nickname) return "Unnamed device";
    return p.verified ? p.nickname : `${p.nickname} (unverified)`;
  }
  var AutoUI = class {
    options;
    constructor(options = {}) {
      this.options = options;
    }
    async requestPermission(req) {
      const a = this.options.allow ?? true;
      const ok = typeof a === "function" ? await a(req) : a;
      this.options.log?.(`${req.origin} asks for ${req.permission}${req.peer ? ` with ${peerLabel(req.peer)}` : ""}: ${ok ? "allowed" : "denied"}`);
      return ok;
    }
    pickPeer(picker, signal) {
      const pick = this.options.pick ?? ((peers) => peers[0]);
      return new Promise((resolve) => {
        const check = () => {
          const p = pick(picker.peers);
          if (!p) return;
          picker.onchange = null;
          this.options.log?.(`picked ${peerLabel(p)}`);
          resolve(p);
        };
        signal.addEventListener("abort", () => resolve(null), { once: true });
        picker.onchange = check;
        check();
      });
    }
    presentPIN(p) {
      this.options.log?.(`shows PIN ${p.pin} to pair with ${peerLabel(p.peer)}`);
      this.options.onPIN?.(p);
    }
    async enterPIN(p, signal) {
      const pin = this.options.enterPIN ? await this.options.enterPIN(p, signal) : null;
      this.options.log?.(pin === null ? "cancels PIN entry" : `enters PIN ${pin}`);
      return pin;
    }
  };

  // src/ua/dom-ui.ts
  var STYLE = `
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
  var DomUI = class {
    root;
    constructor(root = () => document.body ?? document.documentElement) {
      this.root = root;
    }
    requestPermission(req, signal) {
      return new Promise((resolve) => {
        this.show(signal, () => resolve(false), (d, done) => {
          const what = req.permission === "advertise" ? `make this device discoverable as “${req.nickname || "Unnamed device"}”` : `connect with ${req.peer ? peerLabel(req.peer) : "a device"}`;
          d.append(el("h2", `${req.origin} wants to ${what}`));
          d.append(el("p", req.permission === "advertise" ? "Other devices nearby will be able to ask to connect." : "The device will be able to exchange data with this page."));
          d.append(buttons([
            ["Block", false, () => {
              done();
              resolve(false);
            }],
            ["Allow", true, () => {
              done();
              resolve(true);
            }]
          ]));
        });
      });
    }
    pickPeer(picker, signal) {
      return new Promise((resolve) => {
        this.show(signal, () => resolve(null), (d, done) => {
          d.append(el("h2", `${picker.origin} wants to connect to a device`));
          const list2 = el("ul");
          const render = () => {
            list2.replaceChildren();
            if (picker.peers.length === 0) list2.append(el("li", "Looking for devices…", "empty"));
            for (const p of picker.peers) {
              const b = el("button", peerLabel(p));
              b.addEventListener("click", () => {
                picker.onchange = null;
                done();
                resolve(p);
              });
              const li = el("li");
              li.append(b);
              list2.append(li);
            }
          };
          picker.onchange = render;
          render();
          d.append(list2);
          d.append(buttons([["Cancel", false, () => {
            picker.onchange = null;
            done();
            resolve(null);
          }]]));
        });
      });
    }
    presentPIN(p, signal) {
      this.show(signal, () => {
      }, (d) => {
        d.append(el("h2", `Pair with ${peerLabel(p.peer)}`));
        d.append(el("p", "Enter this PIN on the other device:"));
        d.append(el("div", p.pin, "pin"));
      });
    }
    enterPIN(p, signal) {
      return new Promise((resolve) => {
        this.show(signal, () => resolve(null), (d, done) => {
          d.append(el("h2", `Pair with ${peerLabel(p.peer)}`));
          d.append(el("p", "Enter the PIN shown on the other device:"));
          const input = el("input");
          input.inputMode = "numeric";
          input.autocomplete = "off";
          input.placeholder = "123-456-789";
          const ok = () => {
            done();
            resolve(input.value);
          };
          input.addEventListener("keydown", (e) => e.key === "Enter" && ok());
          d.append(input);
          d.append(buttons([
            ["Cancel", false, () => {
              done();
              resolve(null);
            }],
            ["Pair", true, ok]
          ]));
          queueMicrotask(() => input.focus());
        });
      });
    }
    show(signal, onAbort, build) {
      if (signal.aborted) {
        onAbort();
        return;
      }
      const host = document.createElement("lp2p-shim-dialog");
      const shadow = host.attachShadow({ mode: "open" });
      const style = document.createElement("style");
      style.textContent = STYLE;
      const backdrop = el("div", void 0, "backdrop");
      const dialog = el("div", void 0, "dialog");
      dialog.setAttribute("role", "dialog");
      dialog.setAttribute("aria-modal", "true");
      dialog.append(el("span", "LP2P shim · simulated browser UI", "badge"));
      backdrop.append(dialog);
      shadow.append(style, backdrop);
      let removed = false;
      const done = () => {
        if (removed) return;
        removed = true;
        signal.removeEventListener("abort", aborted);
        host.remove();
      };
      const aborted = () => {
        if (removed) return;
        done();
        onAbort();
      };
      signal.addEventListener("abort", aborted, { once: true });
      build(dialog, done);
      this.root().append(host);
    }
  };
  function el(tag, text2, cls) {
    const e = document.createElement(tag);
    if (text2 !== void 0) e.textContent = text2;
    if (cls) e.className = cls;
    return e;
  }
  function buttons(spec) {
    const row = el("div", void 0, "row");
    for (const [label, primary, onclick] of spec) {
      const b = el("button", label, primary ? "primary" : void 0);
      b.addEventListener("click", onclick);
      row.append(b);
    }
    return row;
  }

  // src/protocol/messages.ts
  var messages_exports = {};
  __export(messages_exports, {
    AgentCapability: () => AgentCapability,
    AuthResult: () => AuthResult,
    DataEncoding: () => DataEncoding,
    PskStatus: () => PskStatus,
    Result: () => Result,
    TypeKey: () => TypeKey,
    decodeMessage: () => decodeMessage,
    encodeMessage: () => encodeMessage
  });

  // src/codec/cbor.ts
  var INT_KEY = /^(0|[1-9][0-9]*)$/;
  var textEncoder = new TextEncoder();
  var textDecoder = new TextDecoder("utf-8", { fatal: true });
  var Writer = class {
    buf = new Uint8Array(64);
    len = 0;
    reserve(n) {
      if (this.len + n <= this.buf.length) return;
      let size = this.buf.length * 2;
      while (size < this.len + n) size *= 2;
      const next = new Uint8Array(size);
      next.set(this.buf.subarray(0, this.len));
      this.buf = next;
    }
    byte(b) {
      this.reserve(1);
      this.buf[this.len++] = b;
    }
    bytes(b) {
      this.reserve(b.length);
      this.buf.set(b, this.len);
      this.len += b.length;
    }
    head(major, n) {
      const m = major << 5;
      if (n < 24) {
        this.byte(m | n);
      } else if (n < 256) {
        this.byte(m | 24);
        this.byte(n);
      } else if (n < 65536) {
        this.byte(m | 25);
        this.byte(n >> 8);
        this.byte(n & 255);
      } else if (n < 4294967296) {
        this.byte(m | 26);
        const b = new Uint8Array(4);
        new DataView(b.buffer).setUint32(0, n);
        this.bytes(b);
      } else {
        this.byte(m | 27);
        const b = new Uint8Array(8);
        const view = new DataView(b.buffer);
        view.setUint32(0, Math.floor(n / 2 ** 32));
        view.setUint32(4, n >>> 0);
        this.bytes(b);
      }
    }
    result() {
      return this.buf.slice(0, this.len);
    }
  };
  function encodeInto(w, v) {
    switch (typeof v) {
      case "number":
        if (Number.isSafeInteger(v)) {
          if (v >= 0) w.head(0, v);
          else w.head(1, -1 - v);
        } else {
          w.byte(251);
          const b = new Uint8Array(8);
          new DataView(b.buffer).setFloat64(0, v);
          w.bytes(b);
        }
        return;
      case "string": {
        const b = textEncoder.encode(v);
        w.head(3, b.length);
        w.bytes(b);
        return;
      }
      case "boolean":
        w.byte(v ? 245 : 244);
        return;
      case "undefined":
        w.byte(247);
        return;
      case "object":
        if (v === null) {
          w.byte(246);
        } else if (v instanceof Uint8Array) {
          w.head(2, v.length);
          w.bytes(v);
        } else if (Array.isArray(v)) {
          w.head(4, v.length);
          for (const item of v) encodeInto(w, item);
        } else {
          const entries = Object.entries(v).filter(([, val]) => val !== void 0);
          w.head(5, entries.length);
          for (const [key, val] of entries) {
            if (INT_KEY.test(key) && Number.isSafeInteger(Number(key))) w.head(0, Number(key));
            else encodeInto(w, key);
            encodeInto(w, val);
          }
        }
        return;
      default:
        throw new TypeError(`cbor: cannot encode ${typeof v}`);
    }
  }
  function encodeCBOR(v) {
    const w = new Writer();
    encodeInto(w, v);
    return w.result();
  }
  var MAX_DEPTH = 64;
  var Reader = class {
    pos = 0;
    view;
    buf;
    constructor(buf) {
      this.buf = buf;
      this.view = new DataView(buf.buffer, buf.byteOffset, buf.byteLength);
    }
    need(n) {
      if (this.pos + n > this.buf.length) throw new RangeError("cbor: unexpected end of input");
    }
    u8() {
      this.need(1);
      return this.buf[this.pos++];
    }
    arg(info) {
      if (info < 24) return info;
      switch (info) {
        case 24:
          return this.u8();
        case 25: {
          this.need(2);
          const v = this.view.getUint16(this.pos);
          this.pos += 2;
          return v;
        }
        case 26: {
          this.need(4);
          const v = this.view.getUint32(this.pos);
          this.pos += 4;
          return v;
        }
        case 27: {
          this.need(8);
          const v = this.view.getUint32(this.pos) * 2 ** 32 + this.view.getUint32(this.pos + 4);
          this.pos += 8;
          if (!Number.isSafeInteger(v)) throw new RangeError("cbor: integer exceeds 2^53");
          return v;
        }
        default:
          throw new RangeError(`cbor: unsupported additional info ${info} (indefinite lengths are not supported)`);
      }
    }
    value(depth = 0) {
      if (depth > MAX_DEPTH) throw new RangeError("cbor: nesting too deep");
      const ib = this.u8();
      const major = ib >> 5;
      const info = ib & 31;
      switch (major) {
        case 0:
          return this.arg(info);
        case 1:
          return -1 - this.arg(info);
        case 2: {
          const n = this.arg(info);
          this.need(n);
          const b = this.buf.slice(this.pos, this.pos + n);
          this.pos += n;
          return b;
        }
        case 3: {
          const n = this.arg(info);
          this.need(n);
          const s = textDecoder.decode(this.buf.subarray(this.pos, this.pos + n));
          this.pos += n;
          return s;
        }
        case 4: {
          const n = this.arg(info);
          const out = [];
          for (let i = 0; i < n; i++) out.push(this.value(depth + 1));
          return out;
        }
        case 5: {
          const n = this.arg(info);
          const out = {};
          for (let i = 0; i < n; i++) {
            const key = this.value(depth + 1);
            if (typeof key !== "number" && typeof key !== "string") {
              throw new TypeError("cbor: only integer and text map keys are supported");
            }
            Object.defineProperty(out, String(key), {
              value: this.value(depth + 1),
              enumerable: true,
              writable: true,
              configurable: true
            });
          }
          return out;
        }
        case 6:
          this.arg(info);
          return this.value(depth + 1);
        default:
          switch (info) {
            case 20:
              return false;
            case 21:
              return true;
            case 22:
              return null;
            case 23:
              return void 0;
            case 25: {
              this.need(2);
              const v = decodeFloat16(this.view.getUint16(this.pos));
              this.pos += 2;
              return v;
            }
            case 26: {
              this.need(4);
              const v = this.view.getFloat32(this.pos);
              this.pos += 4;
              return v;
            }
            case 27: {
              this.need(8);
              const v = this.view.getFloat64(this.pos);
              this.pos += 8;
              return v;
            }
            default:
              throw new RangeError(`cbor: unsupported simple value ${info}`);
          }
      }
    }
  };
  function decodeFloat16(h) {
    const sign = h & 32768 ? -1 : 1;
    const exp = h >> 10 & 31;
    const frac = h & 1023;
    if (exp === 0) return sign * 2 ** -14 * (frac / 1024);
    if (exp === 31) return frac ? NaN : sign * Infinity;
    return sign * 2 ** (exp - 15) * (1 + frac / 1024);
  }
  function decodeCBORPrefix(buf, offset = 0) {
    const r = new Reader(buf);
    r.pos = offset;
    const v = r.value();
    return [v, r.pos];
  }
  function decodeCBOR(buf) {
    const [v, end] = decodeCBORPrefix(buf);
    if (end !== buf.length) throw new RangeError("cbor: trailing bytes");
    return v;
  }

  // src/codec/varint.ts
  var MAX_VARINT = 2 ** 53 - 1;
  function encodeVarint(n) {
    if (!Number.isSafeInteger(n) || n < 0 || n > MAX_VARINT) {
      throw new RangeError(`varint out of range: ${n}`);
    }
    if (n < 64) return Uint8Array.of(n);
    if (n < 16384) return Uint8Array.of(64 | n >> 8, n & 255);
    if (n < 1073741824) {
      return Uint8Array.of(128 | n >>> 24, n >>> 16 & 255, n >>> 8 & 255, n & 255);
    }
    const out = new Uint8Array(8);
    const view = new DataView(out.buffer);
    view.setUint32(0, Math.floor(n / 2 ** 32));
    view.setUint32(4, n >>> 0);
    out[0] = (out[0] ?? 0) | 192;
    return out;
  }
  function decodeVarint(buf, offset = 0) {
    const first = buf[offset];
    if (first === void 0) throw new RangeError("varint: unexpected end of input");
    const length = 1 << (first >> 6);
    if (offset + length > buf.length) throw new RangeError("varint: unexpected end of input");
    let value = first & 63;
    for (let i = 1; i < length; i++) {
      value = value * 256 + buf[offset + i];
    }
    if (value > MAX_VARINT) throw new RangeError("varint: value exceeds 2^53");
    return [value, length];
  }

  // src/protocol/messages.ts
  var TypeKey = {
    DataFrame: 24,
    AgentInfoRequest: 10,
    AgentInfoResponse: 11,
    AuthCapabilities: 1001,
    AuthSpake2Confirmation: 1003,
    AuthStatus: 1004,
    AuthSpake2Handshake: 1005,
    DataChannelOpenRequest: 1101,
    DataChannelOpenResponse: 1102
  };
  var AgentCapability = {
    DataChannels: 1100,
    QuicTransport: 1200
  };
  var DataEncoding = {
    Blob: 0,
    String: 1,
    ArrayBuffer: 2
  };
  var Result = {
    Success: 1,
    Timeout: 100,
    TransientError: 101,
    PermanentError: 102,
    Terminating: 103,
    UnknownError: 199
  };
  var PskStatus = {
    NeedsPresentation: 0,
    Shown: 1,
    Input: 2
  };
  var AuthResult = {
    Authenticated: 0,
    UnknownError: 1,
    Timeout: 2,
    SecretUnknown: 3,
    ValidationTookTooLong: 4,
    ProofInvalid: 5
  };
  function toCBOR(m) {
    switch (m.type) {
      case TypeKey.DataFrame:
        return { 0: m.encodingId, 4: m.payload };
      case TypeKey.AgentInfoRequest:
        return { 0: m.requestId };
      case TypeKey.AgentInfoResponse: {
        const a = m.agentInfo;
        return {
          0: m.requestId,
          1: { 0: a.displayName, 1: a.modelName, 2: a.capabilities, 3: a.stateToken, 4: a.locales }
        };
      }
      case TypeKey.AuthCapabilities:
        return { 0: m.pskEaseOfInput, 1: m.pskInputMethods, 2: m.pskMinBitsOfEntropy };
      case TypeKey.AuthSpake2Handshake:
        return {
          0: m.initiationToken === void 0 ? {} : { 0: m.initiationToken },
          1: m.pskStatus,
          2: m.publicValue
        };
      case TypeKey.AuthSpake2Confirmation:
        return { 0: m.confirmationValue };
      case TypeKey.AuthStatus:
        return { 0: m.result };
      case TypeKey.DataChannelOpenRequest:
        return { 0: m.requestId, 1: m.channelId, 2: m.label, 3: m.protocol };
      case TypeKey.DataChannelOpenResponse:
        return { 0: m.requestId, 1: m.result };
    }
  }
  var DecodeError = class extends Error {
    name = "DecodeError";
  };
  function uint(map, key, what) {
    const v = map[key];
    if (typeof v !== "number" || !Number.isSafeInteger(v) || v < 0) throw new DecodeError(`${what}: field ${key} must be uint`);
    return v;
  }
  function text(map, key, what) {
    const v = map[key];
    if (typeof v !== "string") throw new DecodeError(`${what}: field ${key} must be text`);
    return v;
  }
  function bytes(map, key, what) {
    const v = map[key];
    if (!(v instanceof Uint8Array)) throw new DecodeError(`${what}: field ${key} must be bytes`);
    return v;
  }
  function asMap(v, what) {
    if (typeof v !== "object" || v === null || v instanceof Uint8Array || Array.isArray(v)) {
      throw new DecodeError(`${what}: expected a map`);
    }
    return v;
  }
  function list(map, key, what, check) {
    const v = map[key];
    if (!Array.isArray(v) || !v.every(check)) throw new DecodeError(`${what}: field ${key} has the wrong type`);
    return v;
  }
  var isUint = (v) => typeof v === "number" && Number.isSafeInteger(v) && v >= 0;
  var isText = (v) => typeof v === "string";
  function fromCBOR(type, value) {
    const m = asMap(value, `type ${type}`);
    switch (type) {
      case TypeKey.DataFrame:
        return { type, encodingId: uint(m, 0, "data-frame"), payload: bytes(m, 4, "data-frame") };
      case TypeKey.AgentInfoRequest:
        return { type, requestId: uint(m, 0, "agent-info-request") };
      case TypeKey.AgentInfoResponse: {
        const what = "agent-info";
        const a = asMap(m[1], what);
        return {
          type,
          requestId: uint(m, 0, "agent-info-response"),
          agentInfo: {
            displayName: text(a, 0, what),
            modelName: text(a, 1, what),
            capabilities: list(a, 2, what, isUint),
            stateToken: text(a, 3, what),
            locales: list(a, 4, what, isText)
          }
        };
      }
      case TypeKey.AuthCapabilities:
        return {
          type,
          pskEaseOfInput: uint(m, 0, "auth-capabilities"),
          pskInputMethods: list(m, 1, "auth-capabilities", isUint),
          pskMinBitsOfEntropy: uint(m, 2, "auth-capabilities")
        };
      case TypeKey.AuthSpake2Handshake: {
        const token = asMap(m[0], "auth-initiation-token")[0];
        return {
          type,
          ...typeof token === "string" ? { initiationToken: token } : {},
          pskStatus: uint(m, 1, "auth-spake2-handshake"),
          publicValue: bytes(m, 2, "auth-spake2-handshake")
        };
      }
      case TypeKey.AuthSpake2Confirmation:
        return { type, confirmationValue: bytes(m, 0, "auth-spake2-confirmation") };
      case TypeKey.AuthStatus:
        return { type, result: uint(m, 0, "auth-status") };
      case TypeKey.DataChannelOpenRequest:
        return {
          type,
          requestId: uint(m, 0, "data-channel-open-request"),
          channelId: uint(m, 1, "data-channel-open-request"),
          label: text(m, 2, "data-channel-open-request"),
          protocol: text(m, 3, "data-channel-open-request")
        };
      case TypeKey.DataChannelOpenResponse:
        return {
          type,
          requestId: uint(m, 0, "data-channel-open-response"),
          result: uint(m, 1, "data-channel-open-response")
        };
      default:
        throw new DecodeError(`unknown type key ${type}`);
    }
  }
  function encodeMessage(m) {
    const key = encodeVarint(m.type);
    const body = encodeCBOR(toCBOR(m));
    const out = new Uint8Array(key.length + body.length);
    out.set(key);
    out.set(body, key.length);
    return out;
  }
  function decodeMessage(buf) {
    const [type, n] = decodeVarint(buf);
    const [value, end] = decodeCBORPrefix(buf, n);
    if (end !== buf.length) throw new DecodeError("trailing bytes after message");
    return fromCBOR(type, value);
  }

  // src/protocol/auth.ts
  var TAG = new TextEncoder().encode("lp2p-shim toy pairing v1");
  var AuthError = class extends Error {
    name = "AuthError";
    result;
    constructor(message, result) {
      super(message);
      this.result = result;
    }
  };
  function generatePIN() {
    const v = new Uint32Array(1);
    crypto.getRandomValues(v);
    const digits = String(v[0] % 1e9).padStart(9, "0");
    return `${digits.slice(0, 3)}-${digits.slice(3, 6)}-${digits.slice(6)}`;
  }
  function normalizePIN(pin) {
    return pin.replace(/[^0-9]/g, "");
  }
  function nonce() {
    return crypto.getRandomValues(new Uint8Array(32));
  }
  async function proof(pin, presenterNonce, consumerNonce) {
    const p = new TextEncoder().encode(normalizePIN(pin));
    const input = new Uint8Array(TAG.length + p.length + presenterNonce.length + consumerNonce.length);
    input.set(TAG);
    input.set(p, TAG.length);
    input.set(presenterNonce, TAG.length + p.length);
    input.set(consumerNonce, TAG.length + p.length + presenterNonce.length);
    return new Uint8Array(await crypto.subtle.digest("SHA-512", input));
  }
  function equal(a, b) {
    return a.length === b.length && a.every((x, i) => x === b[i]);
  }
  function skipAuthentication(control) {
    control.send({ type: TypeKey.AuthStatus, result: AuthResult.Authenticated });
  }
  async function presentPIN(control, showPIN, signal) {
    const pin = generatePIN();
    const nR = nonce();
    control.send({ type: TypeKey.AuthCapabilities, pskEaseOfInput: 50, pskInputMethods: [0], pskMinBitsOfEntropy: 20 });
    control.send({ type: TypeKey.AuthSpake2Handshake, pskStatus: PskStatus.Shown, publicValue: nR });
    showPIN(pin);
    const hs = await control.next([TypeKey.AuthSpake2Handshake], signal);
    const conf = await control.next([TypeKey.AuthSpake2Confirmation], signal);
    const ok = equal(conf.confirmationValue, await proof(pin, nR, hs.publicValue));
    const result = ok ? AuthResult.Authenticated : AuthResult.ProofInvalid;
    control.send({ type: TypeKey.AuthStatus, result });
    if (!ok) throw new AuthError("the peer entered a wrong PIN", result);
  }
  async function awaitAuthentication(control, enterPIN, signal) {
    for (; ; ) {
      const m = await control.next([TypeKey.AuthSpake2Handshake, TypeKey.AuthStatus], signal);
      if (m.type === TypeKey.AuthStatus) {
        if (m.result === AuthResult.Authenticated) return;
        throw new AuthError(m.result === AuthResult.ProofInvalid ? "wrong PIN" : `authentication failed (${m.result})`, m.result);
      }
      if (m.pskStatus !== PskStatus.Shown) continue;
      const pin = await enterPIN();
      if (pin === null) throw new AuthError("the user canceled pairing", AuthResult.UnknownError);
      const nQ = nonce();
      control.send({ type: TypeKey.AuthSpake2Handshake, pskStatus: PskStatus.Input, publicValue: nQ });
      control.send({ type: TypeKey.AuthSpake2Confirmation, confirmationValue: await proof(pin, m.publicValue, nQ) });
    }
  }

  // src/protocol/session.ts
  var ProtocolError = class extends Error {
    name = "ProtocolError";
  };
  var ControlChannel = class {
    queue = [];
    waiter = null;
    closedWith = null;
    info = null;
    unanswered = [];
    stream;
    constructor(stream) {
      this.stream = stream;
      stream.onmessage = (b) => this.feed(b);
      stream.onclose = () => this.fail(new ProtocolError("control stream closed"));
    }
    send(m) {
      this.stream.send(encodeMessage(m));
    }
    /** What to answer agent-info-request with. Requests that came earlier are answered now. */
    set localInfo(info) {
      this.info = info;
      for (const requestId of this.unanswered.splice(0)) {
        this.send({ type: TypeKey.AgentInfoResponse, requestId, agentInfo: info });
      }
    }
    /** Resolves with the next message of one of types; earlier messages of other types are dropped. */
    next(types, signal) {
      while (this.queue.length > 0) {
        const m = this.queue.shift();
        if (types.includes(m.type)) return Promise.resolve(m);
      }
      if (this.closedWith) return Promise.reject(this.closedWith);
      if (this.waiter) return Promise.reject(new Error("ControlChannel.next: already waiting"));
      return new Promise((resolve, reject) => {
        const onAbort = () => {
          this.waiter = null;
          reject(signal?.reason);
        };
        signal?.addEventListener("abort", onAbort, { once: true });
        this.waiter = {
          types,
          resolve: (m) => {
            signal?.removeEventListener("abort", onAbort);
            resolve(m);
          },
          reject: (e) => {
            signal?.removeEventListener("abort", onAbort);
            reject(e);
          }
        };
      });
    }
    close() {
      this.stream.close();
    }
    /** Handles one raw message from the control stream. */
    feed(b) {
      let m;
      try {
        m = decodeMessage(b);
      } catch {
        return;
      }
      if (m.type === TypeKey.AgentInfoRequest) {
        if (this.info) this.send({ type: TypeKey.AgentInfoResponse, requestId: m.requestId, agentInfo: this.info });
        else this.unanswered.push(m.requestId);
        return;
      }
      if (this.waiter && this.waiter.types.includes(m.type)) {
        const w = this.waiter;
        this.waiter = null;
        w.resolve(m);
      } else {
        this.queue.push(m);
      }
    }
    fail(err) {
      if (this.closedWith) return;
      this.closedWith = err;
      const w = this.waiter;
      this.waiter = null;
      w?.reject(err);
    }
  };
  var DataChannelEndpoint = class {
    state;
    onopen = null;
    onerror = null;
    onclose = null;
    requestId;
    frameHandler = null;
    heldFrames = [];
    params;
    stream;
    session;
    constructor(params, stream, session, opener) {
      this.params = params;
      this.stream = stream;
      this.session = session;
      this.state = opener ? "connecting" : "open";
      this.requestId = opener ? opener.requestId : null;
      stream.onmessage = (b) => this.receive(b);
      stream.onclose = () => this.closed();
    }
    /** Received data-frames. Frames that arrive while this is null are held. */
    set onframe(fn) {
      this.frameHandler = fn;
      while (fn && this.heldFrames.length > 0) fn(...this.heldFrames.shift());
    }
    send(encodingId, payload) {
      if (this.state !== "open") throw new DOMException("data channel is not open", "InvalidStateError");
      this.stream.send(encodeMessage({ type: TypeKey.DataFrame, encodingId, payload }));
    }
    close() {
      this.stream.close();
    }
    receive(b) {
      let m;
      try {
        m = decodeMessage(b);
      } catch (err) {
        this.onerror?.(err instanceof Error ? err : new ProtocolError(String(err)));
        this.stream.close();
        return;
      }
      if (this.state === "connecting") {
        if (m.type !== TypeKey.DataChannelOpenResponse || m.requestId !== this.requestId) {
          this.onerror?.(new ProtocolError(`expected data-channel-open-response, got type ${m.type}`));
          this.stream.close();
          return;
        }
        this.requestId = null;
        if (m.result !== Result.Success) {
          this.onerror?.(new ProtocolError(`peer refused the data channel (result ${m.result})`));
          this.stream.close();
          return;
        }
        this.state = "open";
        this.onopen?.();
        return;
      }
      if (m.type !== TypeKey.DataFrame) return;
      if (this.frameHandler) this.frameHandler(m.encodingId, m.payload);
      else this.heldFrames.push([m.encodingId, m.payload]);
    }
    closed() {
      if (this.state === "closed") return;
      this.state = "closed";
      this.session.releaseChannelId(this.params.id);
      this.onclose?.();
    }
  };
  var Session = class {
    role;
    remoteInfo = null;
    control = null;
    controlWaiters = [];
    usedIds = /* @__PURE__ */ new Set();
    channelIdParity;
    nextRequestId = 1;
    incoming = [];
    channelHandler = null;
    closeHandler = null;
    closeReason = null;
    connection;
    constructor(connection, role) {
      this.connection = connection;
      this.role = role;
      this.channelIdParity = role === "requester" ? 0 : 1;
      if (role === "requester") this.setControl(new ControlChannel(connection.openStream()));
      connection.onstream = (s) => this.classify(s);
      connection.onclose = (reason) => {
        this.closeReason = reason;
        this.closeHandler?.(reason);
      };
    }
    get closed() {
      return this.closeReason !== null;
    }
    set onclose(fn) {
      this.closeHandler = fn;
      if (fn && this.closeReason !== null) fn(this.closeReason);
    }
    /** Incoming data channels. Channels that arrive while this is null are held. */
    set ondatachannel(fn) {
      this.channelHandler = fn;
      while (fn && this.incoming.length > 0) fn(this.incoming.shift());
    }
    requestId() {
      return this.nextRequestId++;
    }
    /** The control stream, once the requester has opened it. */
    controlChannel(signal) {
      if (this.control) return Promise.resolve(this.control);
      return new Promise((resolve, reject) => {
        this.controlWaiters.push(resolve);
        signal?.addEventListener("abort", () => reject(signal.reason), { once: true });
      });
    }
    /** Exchanges agent-info: answers the peer's request and asks for the peer's. */
    async handshake(local, signal) {
      const control = await this.controlChannel(signal);
      control.localInfo = local;
      control.send({ type: TypeKey.AgentInfoRequest, requestId: this.requestId() });
      const res = await control.next([TypeKey.AgentInfoResponse], signal);
      this.remoteInfo = res.agentInfo;
      return res.agentInfo;
    }
    /**
     * Allocates the lowest free channel ID of the local parity, or validates
     * and reserves a requested one, which must be free and of that parity.
     */
    reserveChannelId(requested) {
      if (requested !== void 0) {
        if (requested % 2 !== this.channelIdParity) {
          throw new DOMException(`data channel id ${requested} has the peer's parity`, "OperationError");
        }
        if (this.usedIds.has(requested)) throw new DOMException(`data channel id ${requested} is in use`, "OperationError");
        this.usedIds.add(requested);
        return requested;
      }
      let id = this.channelIdParity;
      while (this.usedIds.has(id)) id += 2;
      if (id > 65534) throw new DOMException("out of data channel ids", "OperationError");
      this.usedIds.add(id);
      return id;
    }
    releaseChannelId(id) {
      this.usedIds.delete(id);
    }
    /** Opens a data channel; params.id must come from reserveChannelId. */
    openDataChannel(params) {
      const stream = this.connection.openStream();
      const requestId = this.requestId();
      const ep = new DataChannelEndpoint(params, stream, this, { requestId });
      stream.send(
        encodeMessage({
          type: TypeKey.DataChannelOpenRequest,
          requestId,
          channelId: params.id,
          label: params.label,
          protocol: params.protocol
        })
      );
      return ep;
    }
    close(reason) {
      this.connection.close(reason);
    }
    setControl(c) {
      this.control = c;
      for (const w of this.controlWaiters.splice(0)) w(c);
    }
    /** Looks at a new stream's first message to tell the control stream from data channels. */
    classify(stream) {
      stream.onmessage = (b) => {
        let m;
        try {
          m = decodeMessage(b);
        } catch {
          stream.close();
          return;
        }
        if (m.type === TypeKey.DataChannelOpenRequest) {
          this.acceptDataChannel(stream, m);
        } else if (!this.control && this.role === "receiver") {
          const c = new ControlChannel(stream);
          this.setControl(c);
          c.feed(b);
        } else {
          stream.close();
        }
      };
    }
    acceptDataChannel(stream, req) {
      const ok = !this.usedIds.has(req.channelId);
      stream.send(
        encodeMessage({
          type: TypeKey.DataChannelOpenResponse,
          requestId: req.requestId,
          result: ok ? Result.Success : Result.PermanentError
        })
      );
      if (!ok) {
        stream.close();
        return;
      }
      this.usedIds.add(req.channelId);
      const ep = new DataChannelEndpoint(
        { id: req.channelId, label: req.label, protocol: req.protocol },
        stream,
        this,
        null
      );
      if (this.channelHandler) this.channelHandler(ep);
      else this.incoming.push(ep);
    }
  };

  // src/ua/useragent.ts
  var LivePicker = class {
    map = /* @__PURE__ */ new Map();
    sorted = [];
    onchange = null;
    origin;
    nickname;
    constructor(origin, nickname) {
      this.origin = origin;
      this.nickname = nickname;
    }
    get peers() {
      return this.sorted;
    }
    update(e) {
      if (e.kind === "removed") this.map.delete(e.peer.id);
      else this.map.set(e.peer.id, { id: e.peer.id, nickname: e.peer.nickname, verified: false });
      this.sorted = [...this.map.values()].sort(
        (a, b) => a.nickname.localeCompare(b.nickname) || a.id.localeCompare(b.id)
      );
      this.onchange?.();
    }
  };
  function notAllowed(message) {
    return new DOMException(message, "NotAllowedError");
  }
  var UserAgent = class {
    transport;
    ui;
    pairing;
    origin;
    modelName;
    handshakeTimeout;
    log;
    constructor(config2) {
      this.transport = config2.transport;
      this.ui = config2.ui;
      this.pairing = config2.pairing ?? "pin";
      this.origin = config2.origin ?? globalThis.location?.origin ?? "null";
      this.modelName = config2.modelName ?? "lp2p-shim";
      this.handshakeTimeout = config2.handshakeTimeout ?? 1e4;
      this.log = config2.log ?? (() => {
      });
    }
    /**
     * LP2PReceiver.start(): asks to advertise as nickname and, if allowed,
     * advertises until closed. Connections the user grants go to onsession.
     */
    async advertise(nickname, onsession) {
      const allowed = await this.ui.requestPermission(
        { permission: "advertise", origin: this.origin, nickname },
        new AbortController().signal
      );
      if (!allowed) throw notAllowed("The user did not allow advertising.");
      const id = randomId();
      this.log(`advertising "${nickname}" as ${id} over ${this.transport.name}`);
      return this.transport.listen({ id, nickname }, (conn) => {
        this.admit(conn, nickname).then(
          (g) => g && onsession(g),
          (err) => this.log(`incoming connection from ${conn.remoteId} failed: ${String(err)}`)
        );
      });
    }
    /** Receiver side of a new connection: agent-info, consent, pairing. */
    async admit(conn, nickname) {
      const session = new Session(conn, "receiver");
      const ac = new AbortController();
      const cancel = () => ac.abort(new DOMException("connection closed", "AbortError"));
      session.onclose = cancel;
      try {
        const info = await this.withTimeout(session.handshake(this.agentInfo(nickname), ac.signal), ac);
        const peer = { id: conn.remoteId, nickname: info.displayName, verified: false };
        const control = await session.controlChannel();
        const ok = await this.ui.requestPermission(
          { permission: "accept-connection", origin: this.origin, nickname, peer },
          ac.signal
        );
        if (!ok) {
          this.log(`declined connection from ${peer.nickname}`);
          control.send({ type: TypeKey.AuthStatus, result: AuthResult.UnknownError });
          session.close("declined");
          return null;
        }
        if (this.pairing === "pin") {
          const shown = new AbortController();
          try {
            await presentPIN(
              control,
              (pin) => this.ui.presentPIN({ pin, origin: this.origin, nickname, peer }, shown.signal),
              ac.signal
            );
          } finally {
            shown.abort();
          }
        } else {
          skipAuthentication(control);
        }
        session.onclose = null;
        return { session, peer };
      } catch (err) {
        session.close(String(err));
        throw err;
      } finally {
        ac.abort();
      }
    }
    /**
     * LP2PRequest.start(): shows the picker, connects to the chosen peer and
     * pairs with it.
     */
    async request(nickname) {
      const picker = new LivePicker(this.origin, nickname);
      const pickAbort = new AbortController();
      const discovery = this.transport.discover((e) => picker.update(e));
      let chosen;
      try {
        chosen = await this.ui.pickPeer(picker, pickAbort.signal);
      } finally {
        pickAbort.abort();
        discovery.close();
      }
      if (!chosen) throw notAllowed("The user dismissed the device picker.");
      const peer = chosen;
      this.log(`connecting to ${peer.nickname} (${peer.id}) over ${this.transport.name}`);
      let conn;
      try {
        conn = await this.transport.connect(peer.id, { id: randomId(), nickname });
      } catch (err) {
        throw new DOMException(`Could not connect to the peer: ${String(err)}`, "NetworkError");
      }
      const session = new Session(conn, "requester");
      const ac = new AbortController();
      session.onclose = (reason) => ac.abort(new DOMException(`connection closed: ${reason}`, "NetworkError"));
      try {
        const info = await this.withTimeout(session.handshake(this.agentInfo(nickname), ac.signal), ac);
        const verified = { ...peer, nickname: info.displayName };
        const control = await session.controlChannel(ac.signal);
        await awaitAuthentication(
          control,
          () => this.ui.enterPIN({ origin: this.origin, nickname, peer: verified }, ac.signal),
          ac.signal
        );
        session.onclose = null;
        return { session, peer: verified };
      } catch (err) {
        session.close(String(err));
        if (err instanceof AuthError) {
          throw notAllowed(
            err.result === AuthResult.UnknownError && err.message.startsWith("authentication failed") ? "The peer declined the connection." : `Pairing failed: ${err.message}.`
          );
        }
        if (err instanceof DOMException) throw err;
        throw new DOMException(`Connection setup failed: ${String(err)}`, "NetworkError");
      } finally {
        ac.abort();
      }
    }
    agentInfo(nickname) {
      return {
        displayName: nickname,
        modelName: this.modelName,
        capabilities: [AgentCapability.DataChannels],
        stateToken: randomId(4),
        locales: globalThis.navigator?.languages ? [...globalThis.navigator.languages] : []
      };
    }
    withTimeout(p, ac) {
      const t = setTimeout(
        () => ac.abort(new DOMException("timed out waiting for the peer", "TimeoutError")),
        this.handshakeTimeout
      );
      return p.finally(() => clearTimeout(t));
    }
  };

  // src/config.ts
  var config = {};
  var current = null;
  function configure(c) {
    config = { ...config, ...c };
    current = null;
  }
  function getUserAgent() {
    if (current) return current;
    const hasDOM = typeof document !== "undefined";
    current = new UserAgent({
      transport: config.transport ?? (config.transport = new PostMessageTransport()),
      ui: config.ui ?? (hasDOM ? new DomUI() : new AutoUI()),
      pairing: config.pairing ?? (config.ui || hasDOM ? "pin" : "none"),
      ...config.origin !== void 0 ? { origin: config.origin } : {},
      ...config.log !== void 0 ? { log: config.log } : {}
    });
    return current;
  }

  // src/api/events.ts
  var connections = /* @__PURE__ */ new WeakMap();
  var channels = /* @__PURE__ */ new WeakMap();
  var LP2PConnectionEvent = class extends Event {
    constructor(type, connectionEventInitDict) {
      super(type, connectionEventInitDict);
      if (!connectionEventInitDict?.connection) throw new TypeError("LP2PConnectionEventInit.connection is required");
      connections.set(this, connectionEventInitDict.connection);
    }
    get connection() {
      return connections.get(this);
    }
  };
  var LP2PDataChannelEvent = class extends Event {
    constructor(type, DataChannelEventInitDict) {
      super(type, DataChannelEventInitDict);
      if (!DataChannelEventInitDict?.channel) throw new TypeError("LP2PDataChannelEventInit.channel is required");
      channels.set(this, DataChannelEventInitDict.channel);
    }
    get channel() {
      return channels.get(this);
    }
  };
  var handlers = /* @__PURE__ */ new WeakMap();
  function getHandler(target, type) {
    return handlers.get(target)?.get(type)?.fn ?? null;
  }
  function setHandler(target, type, fn) {
    let map = handlers.get(target);
    if (!map) handlers.set(target, map = /* @__PURE__ */ new Map());
    const entry = map.get(type);
    const value = typeof fn === "function" ? fn : null;
    if (entry) {
      entry.fn = value;
      return;
    }
    if (!value) return;
    const slot = {
      fn: value,
      listener: (ev) => {
        slot.fn?.call(target, ev);
      }
    };
    map.set(type, slot);
    target.addEventListener(type, slot.listener);
  }

  // src/api/datachannel.ts
  var textDecoder2 = new TextDecoder();
  var textEncoder2 = new TextEncoder();
  var adopting = null;
  function adoptChannel(endpoint, incoming) {
    adopting = { endpoint, incoming };
    try {
      const p = endpoint.params;
      return new LP2PDataChannel(p.label, { protocol: p.protocol, id: p.id });
    } finally {
      adopting = null;
    }
  }
  function enforceUnsignedShort(v, what) {
    const n = Number(v);
    if (!Number.isFinite(n)) throw new TypeError(`${what} is not a finite number`);
    const t = Math.trunc(n);
    if (t < 0 || t > 65535) throw new TypeError(`${what} is outside the unsigned short range`);
    return t;
  }
  var LP2PDataChannel = class extends EventTarget {
    #label;
    #protocol;
    #id;
    #readyState;
    #binaryType = "arraybuffer";
    #endpoint;
    #sendChain = null;
    /**
     * The IDL gives LP2PDataChannel a public constructor, but says nothing
     * about what a channel made that way is attached to. The shim lets you
     * construct one, and it is permanently "closed": channels that carry data
     * come from LP2PConnection.createDataChannel() or the datachannel event.
     */
    constructor(label, dataChannelDict = {}) {
      super();
      if (arguments.length < 1) throw new TypeError("LP2PDataChannel: label is required");
      this.#label = String(label);
      this.#protocol = String(dataChannelDict.protocol ?? "");
      this.#id = dataChannelDict.id === void 0 ? null : enforceUnsignedShort(dataChannelDict.id, "id");
      const a = adopting;
      adopting = null;
      if (!a) {
        this.#endpoint = null;
        this.#readyState = "closed";
        return;
      }
      const ep = a.endpoint;
      this.#endpoint = ep;
      this.#readyState = ep.state === "open" ? "open" : "connecting";
      ep.onopen = () => {
        if (this.#readyState !== "connecting") return;
        this.#readyState = "open";
        this.dispatchEvent(new Event("open"));
      };
      ep.onframe = (enc, payload) => this.#receive(enc, payload);
      ep.onerror = (err) => {
        const ev = new Event("error");
        Object.defineProperty(ev, "error", { value: err });
        this.dispatchEvent(ev);
      };
      ep.onclose = () => this.#closed();
      if (a.incoming) {
        queueMicrotask(() => {
          if (this.#readyState === "open") this.dispatchEvent(new Event("open"));
        });
      }
    }
    get label() {
      return this.#label;
    }
    get protocol() {
      return this.#protocol;
    }
    get id() {
      return this.#id;
    }
    /** Extension: not in the LP2P IDL, but needed to use the channel safely. */
    get readyState() {
      return this.#readyState;
    }
    get binaryType() {
      return this.#binaryType;
    }
    set binaryType(v) {
      if (v === "blob" || v === "arraybuffer") this.#binaryType = v;
    }
    get onopen() {
      return getHandler(this, "open");
    }
    set onopen(fn) {
      setHandler(this, "open", fn);
    }
    get onerror() {
      return getHandler(this, "error");
    }
    set onerror(fn) {
      setHandler(this, "error", fn);
    }
    get onclosing() {
      return getHandler(this, "closing");
    }
    set onclosing(fn) {
      setHandler(this, "closing", fn);
    }
    get onclose() {
      return getHandler(this, "close");
    }
    set onclose(fn) {
      setHandler(this, "close", fn);
    }
    get onmessage() {
      return getHandler(this, "message");
    }
    set onmessage(fn) {
      setHandler(this, "message", fn);
    }
    /**
     * Sends data. Strings go as encoding-id string, ArrayBuffer and views as
     * array-buffer, Blobs as blob. Blobs are read asynchronously; messages
     * sent after a Blob wait for it, so order is kept.
     */
    send(data) {
      if (this.#readyState !== "open" || !this.#endpoint) {
        throw new DOMException(`LP2PDataChannel is ${this.#readyState}`, "InvalidStateError");
      }
      const ep = this.#endpoint;
      let frame;
      if (typeof data === "string") {
        frame = [DataEncoding.String, textEncoder2.encode(data)];
      } else if (data instanceof ArrayBuffer) {
        frame = [DataEncoding.ArrayBuffer, new Uint8Array(data.slice(0))];
      } else if (ArrayBuffer.isView(data)) {
        frame = [DataEncoding.ArrayBuffer, new Uint8Array(data.buffer, data.byteOffset, data.byteLength).slice()];
      } else if (typeof Blob !== "undefined" && data instanceof Blob) {
        frame = data.arrayBuffer().then((b) => [DataEncoding.Blob, new Uint8Array(b)]);
      } else {
        frame = [DataEncoding.String, textEncoder2.encode(String(data))];
      }
      if (!(frame instanceof Promise) && !this.#sendChain) {
        ep.send(frame[0], frame[1]);
        return;
      }
      const pending = Promise.resolve(frame);
      const chain = (this.#sendChain ?? Promise.resolve()).then(async () => {
        const [enc, payload] = await pending;
        if (ep.state === "open") ep.send(enc, payload);
      });
      this.#sendChain = chain;
      chain.then(
        () => {
          if (this.#sendChain === chain) this.#sendChain = null;
        },
        () => {
          if (this.#sendChain === chain) this.#sendChain = null;
          this.dispatchEvent(new Event("error"));
        }
      );
    }
    /**
     * Closes the channel. readyState becomes "closing" and closing fires (as
     * in go-lp2p); close fires once the channel is closed. Messages queued
     * behind a Blob are sent first.
     */
    close() {
      if (this.#readyState === "closing" || this.#readyState === "closed") return;
      this.#readyState = "closing";
      queueMicrotask(() => this.dispatchEvent(new Event("closing")));
      const ep = this.#endpoint;
      void (this.#sendChain ?? Promise.resolve()).finally(() => ep?.close());
    }
    #receive(enc, payload) {
      if (this.#readyState !== "open") return;
      let data;
      if (enc === DataEncoding.String) data = textDecoder2.decode(payload);
      else if (this.#binaryType === "blob") data = new Blob([payload]);
      else data = payload.slice().buffer;
      this.dispatchEvent(new MessageEvent("message", { data }));
    }
    #closed() {
      if (this.#readyState === "closed") return;
      const wasClosing = this.#readyState === "closing";
      this.#readyState = "closing";
      queueMicrotask(() => {
        if (!wasClosing) this.dispatchEvent(new Event("closing"));
        this.#readyState = "closed";
        this.dispatchEvent(new Event("close"));
      });
    }
  };

  // src/api/connection.ts
  var adopting2 = null;
  function adoptConnection(session) {
    adopting2 = session;
    try {
      return new LP2PConnection();
    } finally {
      adopting2 = null;
    }
  }
  var LP2PConnection = class extends EventTarget {
    #session;
    #closed = false;
    /** Not constructible from script: the IDL has no constructor. */
    constructor() {
      super();
      const s = adopting2;
      adopting2 = null;
      if (!s) throw new TypeError("Illegal constructor");
      this.#session = s;
      s.ondatachannel = (ep) => {
        const channel = adoptChannel(ep, true);
        this.dispatchEvent(new LP2PDataChannelEvent("datachannel", { channel }));
      };
      s.onclose = () => {
        if (this.#closed) return;
        this.#closed = true;
        this.dispatchEvent(new Event("close"));
      };
    }
    createDataChannel(label, dataChannelDict = {}) {
      if (arguments.length < 1) throw new TypeError("createDataChannel: label is required");
      if (this.#closed) throw new DOMException("LP2PConnection is closed", "InvalidStateError");
      const requested = dataChannelDict.id === void 0 ? void 0 : enforceUnsignedShort(dataChannelDict.id, "id");
      if (requested === 65535) throw new TypeError("id 65535 is reserved");
      const id = this.#session.reserveChannelId(requested);
      const ep = this.#session.openDataChannel({
        id,
        label: String(label),
        protocol: String(dataChannelDict.protocol ?? "")
      });
      return adoptChannel(ep, false);
    }
    get ondatachannel() {
      return getHandler(this, "datachannel");
    }
    set ondatachannel(fn) {
      setHandler(this, "datachannel", fn);
    }
    /** Extension: not in the LP2P IDL. Closes the connection and all its channels; fires close. */
    close() {
      this.#session.close("closed by page");
    }
    /** Extension: fired when the connection closes, locally or by the peer. */
    get onclose() {
      return getHandler(this, "close");
    }
    set onclose(fn) {
      setHandler(this, "close", fn);
    }
  };

  // src/api/receiver.ts
  var LP2PReceiver = class extends EventTarget {
    #nickname;
    #started = null;
    #advertisement = null;
    #stopped = false;
    constructor(options = {}) {
      super();
      this.#nickname = options.nickname === void 0 ? "" : String(options.nickname);
    }
    /**
     * Asks the user for permission to advertise and starts advertising.
     * Resolves once advertising; rejects with NotAllowedError if denied.
     * Calling it again returns the same promise (the spec does not say).
     */
    start() {
      if (this.#started) return this.#started;
      this.#started = getUserAgent().advertise(this.#nickname, ({ session }) => {
        if (this.#stopped) {
          session.close("receiver stopped");
          return;
        }
        this.dispatchEvent(new LP2PConnectionEvent("connection", { connection: adoptConnection(session) }));
      }).then((ad) => {
        this.#advertisement = ad;
        if (this.#stopped) ad.close();
        return void 0;
      });
      this.#started.catch(() => {
        this.#started = null;
      });
      return this.#started;
    }
    /** Extension: not in the LP2P IDL. Stops advertising; existing connections stay open. */
    stop() {
      this.#stopped = true;
      this.#advertisement?.close();
    }
    get onconnection() {
      return getHandler(this, "connection");
    }
    set onconnection(fn) {
      setHandler(this, "connection", fn);
    }
  };

  // src/api/request.ts
  var LP2PRequest = class {
    #nickname;
    #started = null;
    constructor(options = {}) {
      this.#nickname = options.nickname === void 0 ? "" : String(options.nickname);
    }
    /**
     * Shows the device picker, connects to the chosen peer and pairs with it.
     * Rejects with NotAllowedError if the user dismisses the picker, the peer
     * declines or pairing fails, and NetworkError if the peer is unreachable.
     * While pending or after success, calling it again returns the same
     * promise; after a failure it starts over.
     */
    start() {
      if (this.#started) return this.#started;
      const p = getUserAgent().request(this.#nickname).then(({ session }) => adoptConnection(session));
      this.#started = p;
      p.catch(() => {
        if (this.#started === p) this.#started = null;
      });
      return p;
    }
  };

  // src/install.ts
  var interfaces = {
    LP2PReceiver,
    LP2PRequest,
    LP2PConnection,
    LP2PDataChannel,
    LP2PConnectionEvent,
    LP2PDataChannelEvent
  };
  function install(options = {}) {
    if (globalThis.isSecureContext === false) return [];
    const installed2 = [];
    for (const [name, value] of Object.entries(interfaces)) {
      if (!options.force && name in globalThis) continue;
      Object.defineProperty(globalThis, name, { value, writable: true, configurable: true, enumerable: false });
      installed2.push(name);
    }
    return installed2;
  }

  // src/transport/webrtc.ts
  var CONTROL_LABEL = "lp2p-shim/control";
  var DISCONNECT_GRACE_MS = 5e3;
  var ChannelStream = class extends BaseStream {
    pending = [];
    dc;
    conn;
    constructor(id, dc, conn, received = []) {
      super(id);
      this.dc = dc;
      this.conn = conn;
      dc.binaryType = "arraybuffer";
      for (const m of received) this.deliver(new Uint8Array(m));
      dc.onopen = () => {
        for (const m of this.pending.splice(0)) this.dc.send(m);
        if (this.closed) this.dc.close();
      };
      dc.onmessage = (e) => this.deliver(new Uint8Array(e.data));
      dc.onclose = () => {
        this.conn.forget(this.id);
        this.remoteClosed();
      };
    }
    send(message) {
      if (this.closed) throw new DOMException("stream is closed", "InvalidStateError");
      if (this.dc.readyState === "connecting") this.pending.push(message);
      else this.dc.send(message);
    }
    doClose() {
      this.conn.forget(this.id);
      if (this.dc.readyState !== "connecting") this.dc.close();
    }
  };
  var RTCConnection = class extends BaseConnection {
    nextId = 0;
    pc;
    control;
    constructor(remoteId, pc, control, early) {
      super(remoteId);
      this.pc = pc;
      this.control = control;
      pc.ondatachannel = (e) => this.accept(this.wrap(e.channel));
      for (const { dc, received } of early) this.accept(this.wrap(dc, received));
      control.onmessage = (e) => {
        if (e.data === "bye") this.shutdown("closed by peer");
      };
      control.onclose = () => this.shutdown("control channel closed");
      let grace;
      pc.onconnectionstatechange = () => {
        clearTimeout(grace);
        const state = pc.connectionState;
        if (state === "failed" || state === "closed") {
          this.shutdown(`peer connection ${state}`);
        } else if (state === "disconnected") {
          grace = setTimeout(() => this.shutdown("peer connection disconnected"), DISCONNECT_GRACE_MS);
        }
      };
    }
    openStream() {
      if (this.closed) throw new DOMException("connection is closed", "InvalidStateError");
      const s = this.wrap(this.pc.createDataChannel(`lp2p-shim/stream`, { ordered: true }));
      this.streams.set(s.id, s);
      return s;
    }
    forget(id) {
      this.streams.delete(id);
    }
    doClose() {
      if (this.control.readyState === "open") this.control.send("bye");
      setTimeout(() => this.pc.close(), 100);
    }
    wrap(dc, received) {
      return new ChannelStream(this.nextId++, dc, this, received);
    }
    shutdown(reason) {
      if (this.closed) return;
      this.remoteClosed(reason);
      this.pc.close();
    }
  };
  var CandidateQueue = class {
    queue = [];
    pc;
    constructor(pc) {
      this.pc = pc;
    }
    add(c) {
      if (this.queue) this.queue.push(c);
      else void this.pc.addIceCandidate(c).catch(() => {
      });
    }
    flush() {
      const q = this.queue ?? [];
      this.queue = null;
      for (const c of q) void this.pc.addIceCandidate(c).catch(() => {
      });
    }
  };
  var WebRTCTransport = class {
    name = "webrtc";
    rendezvous;
    trickle;
    connectTimeout;
    rtcConfiguration;
    constructor(options = {}) {
      const signaler = options.signaler ?? new BroadcastChannelSignaler(options.channelName ?? "lp2p-shim/webrtc");
      this.rendezvous = new Rendezvous(signaler, options);
      this.trickle = signaler.live;
      this.connectTimeout = options.connectTimeout ?? (signaler.live ? 15e3 : 6e5);
      this.rtcConfiguration = options.rtcConfiguration ?? { iceServers: [] };
    }
    listen(ad, onconnection) {
      const sessions = /* @__PURE__ */ new Map();
      return this.rendezvous.advertise(ad, (m) => {
        if (m.t !== "signal" || typeof m["conn"] !== "string") return;
        const connId = m["conn"];
        const data = m["data"];
        if (data.sdp?.type === "offer" && !sessions.has(connId)) {
          const pc = new RTCPeerConnection(this.rtcConfiguration);
          const candidates = new CandidateQueue(pc);
          sessions.set(connId, { pc, candidates });
          const control = pc.createDataChannel(CONTROL_LABEL, { negotiated: true, id: 0 });
          const early = collectEarlyChannels(pc);
          const reply = (d) => this.rendezvous.send({ t: "signal", from: ad.id, to: m.from, conn: connId, data: d });
          if (this.trickle) {
            pc.onicecandidate = (e) => e.candidate && reply({ candidate: e.candidate.toJSON() });
          }
          control.onopen = () => {
            sessions.delete(connId);
            onconnection(new RTCConnection(m.from, pc, control, early));
          };
          const fail = () => {
            sessions.delete(connId);
            pc.close();
          };
          const timer = setTimeout(fail, this.connectTimeout);
          control.addEventListener("open", () => clearTimeout(timer), { once: true });
          void (async () => {
            await pc.setRemoteDescription(data.sdp);
            candidates.flush();
            await pc.setLocalDescription(await pc.createAnswer());
            if (!this.trickle) await gathered(pc);
            reply({ sdp: pc.localDescription?.toJSON() });
          })().catch(fail);
        } else if (data.candidate) {
          sessions.get(connId)?.candidates.add(data.candidate);
        }
      });
    }
    discover(onevent) {
      return this.rendezvous.discover(onevent);
    }
    async connect(peerId, local, options = {}) {
      const connId = randomId();
      const pc = new RTCPeerConnection(this.rtcConfiguration);
      const candidates = new CandidateQueue(pc);
      const control = pc.createDataChannel(CONTROL_LABEL, { negotiated: true, id: 0 });
      const early = collectEarlyChannels(pc);
      const send = (d) => this.rendezvous.send({ t: "signal", from: local.id, to: peerId, conn: connId, nickname: local.nickname, data: d });
      let registration;
      let timer;
      let onAbort;
      try {
        return await new Promise((resolve, reject) => {
          onAbort = () => reject(options.signal?.reason);
          options.signal?.addEventListener("abort", onAbort);
          timer = setTimeout(() => reject(new DOMException(`could not connect to ${peerId}`, "NetworkError")), this.connectTimeout);
          registration = this.rendezvous.register(local.id, (m) => {
            if (m.t !== "signal" || m["conn"] !== connId) return;
            const data = m["data"];
            if (data.sdp?.type === "answer") {
              pc.setRemoteDescription(data.sdp).then(() => candidates.flush(), reject);
            } else if (data.candidate) {
              candidates.add(data.candidate);
            }
          });
          if (this.trickle) {
            pc.onicecandidate = (e) => e.candidate && send({ candidate: e.candidate.toJSON() });
          }
          control.onopen = () => resolve(new RTCConnection(peerId, pc, control, early));
          void (async () => {
            await pc.setLocalDescription(await pc.createOffer());
            if (!this.trickle) await gathered(pc);
            send({ sdp: pc.localDescription?.toJSON() });
          })().catch(reject);
        });
      } catch (err) {
        pc.close();
        throw err;
      } finally {
        clearTimeout(timer);
        registration?.close();
        if (onAbort) options.signal?.removeEventListener("abort", onAbort);
      }
    }
    close() {
      this.rendezvous.close();
    }
  };
  function collectEarlyChannels(pc) {
    const early = [];
    pc.ondatachannel = (e) => {
      const ch = { dc: e.channel, received: [] };
      ch.dc.binaryType = "arraybuffer";
      ch.dc.onmessage = (m) => ch.received.push(m.data);
      early.push(ch);
    };
    return early;
  }
  function gathered(pc) {
    if (pc.iceGatheringState === "complete") return Promise.resolve();
    return new Promise((resolve) => {
      pc.addEventListener("icegatheringstatechange", () => {
        if (pc.iceGatheringState === "complete") resolve();
      });
    });
  }

  // src/index.ts
  var version = "0.1.0";

  // src/polyfill.ts
  var installed = install();
  Object.defineProperty(globalThis, "LP2PShim", {
    value: Object.freeze({ ...index_exports, installed }),
    writable: true,
    configurable: true,
    enumerable: false
  });
})();
