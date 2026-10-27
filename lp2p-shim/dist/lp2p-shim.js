/*! lp2p-shim 0.2.0 - experimental Local Peer-to-Peer API polyfill. NOT SECURE: no real authentication or encryption. https://github.com/backkem/go-lp2p/tree/main/lp2p-shim */
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
    LP2PBidirectionalStream: () => LP2PBidirectionalStream,
    LP2PConnection: () => LP2PConnection,
    LP2PConnectionEvent: () => LP2PConnectionEvent,
    LP2PDataChannel: () => LP2PDataChannel,
    LP2PDataChannelEvent: () => LP2PDataChannelEvent,
    LP2PDatagramDuplexStream: () => LP2PDatagramDuplexStream,
    LP2PDatagramsWritable: () => LP2PDatagramsWritable,
    LP2PQuicTransport: () => LP2PQuicTransport,
    LP2PQuicTransportListener: () => LP2PQuicTransportListener,
    LP2PReceiveStream: () => LP2PReceiveStream,
    LP2PReceiver: () => LP2PReceiver,
    LP2PRequest: () => LP2PRequest,
    LP2PSendGroup: () => LP2PSendGroup,
    LP2PSendStream: () => LP2PSendStream,
    LocalState: () => LocalState,
    ManualSignaler: () => ManualSignaler,
    PortSignaler: () => PortSignaler,
    PostMessageTransport: () => PostMessageTransport,
    ProtocolError: () => ProtocolError,
    Rendezvous: () => Rendezvous,
    SESSION_GONE: () => SESSION_GONE,
    Session: () => Session,
    StreamFrame: () => StreamFrame,
    UserAgent: () => UserAgent,
    WTSession: () => WTSession,
    WTStream: () => WTStream,
    WebRTCTransport: () => WebRTCTransport,
    WebTransportError: () => WebTransportError,
    configure: () => configure,
    connect: () => connect,
    createIdentity: () => createIdentity,
    decodeCBOR: () => decodeCBOR,
    decodeVarint: () => decodeVarint,
    encodeCBOR: () => encodeCBOR,
    encodeVarint: () => encodeVarint,
    fingerprintOf: () => fingerprintOf,
    getConfig: () => getConfig,
    getUserAgent: () => getUserAgent,
    install: () => install,
    interfaces: () => interfaces,
    localStateFor: () => localStateFor,
    lp2p: () => lp2p,
    makeDiscoverable: () => makeDiscoverable,
    messages: () => messages_exports,
    parseOrigin: () => parseOrigin,
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
    unidirectional;
    constructor(id, unidirectional = false) {
      this.id = id;
      this.unidirectional = unidirectional;
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
  var MAX_HELD_DATAGRAMS = 64;
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
    datagramHandler = null;
    heldDatagrams = [];
    remoteId;
    /** Overridden by transports that carry datagrams. */
    get maxDatagramSize() {
      return 0;
    }
    get datagramsReliable() {
      return false;
    }
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
    get ondatagram() {
      return this.datagramHandler;
    }
    set ondatagram(fn) {
      this.datagramHandler = fn;
      while (fn && this.heldDatagrams.length > 0) fn(this.heldDatagrams.shift());
    }
    sendDatagram(datagram) {
      if (this.closed) throw new DOMException("connection is closed", "InvalidStateError");
      if (datagram.byteLength > this.maxDatagramSize) {
        throw new RangeError(`datagram of ${datagram.byteLength} bytes exceeds ${this.maxDatagramSize}`);
      }
      this.doSendDatagram(datagram);
    }
    /** For transports with datagrams: send one. */
    doSendDatagram(_datagram) {
      throw new DOMException("this transport has no datagrams", "NotSupportedError");
    }
    /** For implementations: deliver a received datagram. */
    deliverDatagram(datagram) {
      if (this.closed) return;
      if (this.datagramHandler) {
        this.datagramHandler(datagram);
        return;
      }
      this.heldDatagrams.push(datagram);
      if (this.heldDatagrams.length > MAX_HELD_DATAGRAMS) this.heldDatagrams.shift();
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
      this.heldDatagrams.length = 0;
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
    constructor(id, conn, unidirectional = false) {
      super(id, unidirectional);
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
    get maxDatagramSize() {
      return 1200;
    }
    get datagramsReliable() {
      return true;
    }
    doSendDatagram(d) {
      this.post({ k: "dgram", d });
    }
    openStream(options = {}) {
      if (this.closed) throw new DOMException("connection is closed", "InvalidStateError");
      const uni = Boolean(options.unidirectional);
      const s = new PortStream(this.nextStreamId, this, uni);
      this.nextStreamId += 2;
      this.streams.set(s.id, s);
      this.post(uni ? { k: "open", s: s.id, u: true } : { k: "open", s: s.id });
      return s;
    }
    post(frame2) {
      if (!this.closed) this.port.postMessage(frame2);
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
          this.accept(new PortStream(f.s, this, f.u === true));
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
        case "dgram":
          this.deliverDatagram(f.d);
          break;
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
    const device = p.displayName ? p.verified ? p.displayName : `${p.displayName}?` : "";
    if (!p.nickname) return device ? `${device}${p.verified ? "" : " (unverified)"}` : "Unnamed device";
    if (p.verified && device) return `${p.nickname} on ${device}`;
    return `${p.nickname} (unverified)`;
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
    DecodeError: () => DecodeError,
    MAX_CLOSE_REASON_BYTES: () => MAX_CLOSE_REASON_BYTES,
    MAX_EXCHANGE_ID: () => MAX_EXCHANGE_ID,
    PskStatus: () => PskStatus,
    Result: () => Result,
    SESSION_BINDING_BYTES: () => SESSION_BINDING_BYTES,
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
    DataChannelOpenResponse: 1102,
    // Session extension: the requesting origin, before authentication.
    SessionRequest: 1110,
    SessionResponse: 1111,
    // WebTransport extension (see webtransport.ts). 1204 is reserved.
    DataTransportStartRequest: 1201,
    DataTransportStartResponse: 1202,
    DataTransportStream: 1203,
    DataTransportClose: 1205,
    DataTransportDrain: 1206,
    // Not OSP: the shim's stand-in for the TLS handshake underneath OSP (see
    // ../protocol/identity.ts). Private-use type keys, sent first on the
    // control stream.
    ShimHello: 32001,
    ShimFinished: 32002
  };
  var MAX_EXCHANGE_ID = 2 ** 62;
  var MAX_CLOSE_REASON_BYTES = 1024;
  var SESSION_BINDING_BYTES = 32;
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
      case TypeKey.SessionResponse:
      case TypeKey.DataTransportStartResponse:
        return { 0: m.requestId, 1: m.result };
      case TypeKey.SessionRequest:
        return { 0: m.requestId, 1: m.requestingOrigin };
      case TypeKey.DataTransportStartRequest:
        checkExchangeId(m.exchangeId);
        if (m.sessionBinding === void 0) return { 0: m.requestId, 1: m.exchangeId };
        checkBinding(m.sessionBinding);
        return { 0: m.requestId, 1: m.exchangeId, 2: m.sessionBinding };
      case TypeKey.DataTransportStream:
        checkExchangeId(m.exchangeId);
        return { 1: m.exchangeId };
      case TypeKey.DataTransportClose:
        checkCloseCode(m.code);
        checkReason(m.reason);
        return { 0: m.code, 1: m.reason };
      case TypeKey.DataTransportDrain:
        return {};
      case TypeKey.ShimHello:
        return { 0: m.signingKey, 1: m.exchangeKey, 2: m.nonce };
      case TypeKey.ShimFinished:
        return { 0: m.signature };
    }
  }
  var DecodeError = class extends Error {
    name = "DecodeError";
  };
  function checkExchangeId(id) {
    if (!Number.isSafeInteger(id) || id < 0 || id >= MAX_EXCHANGE_ID) throw new DecodeError(`exchange-id ${id} out of range`);
  }
  function checkCloseCode(code) {
    if (!Number.isSafeInteger(code) || code < 0 || code > 4294967295) throw new DecodeError(`close-code ${code} out of range`);
  }
  function checkReason(reason) {
    if (new TextEncoder().encode(reason).length > MAX_CLOSE_REASON_BYTES) throw new DecodeError("reason longer than 1024 bytes");
  }
  function checkBinding(b) {
    if (b.length !== SESSION_BINDING_BYTES) throw new DecodeError("session-binding must be 32 bytes");
  }
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
      case TypeKey.SessionRequest:
        return { type, requestId: uint(m, 0, "lp2p-session-request"), requestingOrigin: text(m, 1, "lp2p-session-request") };
      case TypeKey.SessionResponse:
        return { type, requestId: uint(m, 0, "lp2p-session-response"), result: uint(m, 1, "lp2p-session-response") };
      case TypeKey.DataTransportStartRequest: {
        const what = "data-transport-start-request";
        const exchangeId = uint(m, 1, what);
        checkExchangeId(exchangeId);
        if (m[2] === void 0) return { type, requestId: uint(m, 0, what), exchangeId };
        const sessionBinding = bytes(m, 2, what);
        checkBinding(sessionBinding);
        return { type, requestId: uint(m, 0, what), exchangeId, sessionBinding };
      }
      case TypeKey.DataTransportStartResponse:
        return { type, requestId: uint(m, 0, "data-transport-start-response"), result: uint(m, 1, "data-transport-start-response") };
      case TypeKey.DataTransportStream: {
        const exchangeId = uint(m, 1, "data-transport-stream");
        checkExchangeId(exchangeId);
        return { type, exchangeId };
      }
      case TypeKey.DataTransportClose: {
        const code = uint(m, 0, "data-transport-close");
        const reason = text(m, 1, "data-transport-close");
        checkCloseCode(code);
        checkReason(reason);
        return { type, code, reason };
      }
      case TypeKey.DataTransportDrain:
        return { type };
      case TypeKey.ShimHello:
        return {
          type,
          signingKey: bytes(m, 0, "shim-hello"),
          exchangeKey: bytes(m, 1, "shim-hello"),
          nonce: bytes(m, 2, "shim-hello")
        };
      case TypeKey.ShimFinished:
        return { type, signature: bytes(m, 0, "shim-finished") };
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
  var TAG = new TextEncoder().encode("lp2p-shim toy pairing v2");
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
  async function proof(pin, serverNonce, clientNonce, parties) {
    const e = new TextEncoder();
    const parts = [TAG, e.encode(normalizePIN(pin)), serverNonce, clientNonce, e.encode(parties.client), e.encode(parties.server)];
    const input = new Uint8Array(parts.reduce((n, p) => n + p.length, 0));
    let off = 0;
    for (const p of parts) {
      input.set(p, off);
      off += p.length;
    }
    return new Uint8Array(await crypto.subtle.digest("SHA-512", input));
  }
  function equal(a, b) {
    return a.length === b.length && a.every((x, i) => x === b[i]);
  }
  function sendAuthCapabilities(control) {
    control.send({ type: TypeKey.AuthCapabilities, pskEaseOfInput: 50, pskInputMethods: [0], pskMinBitsOfEntropy: 20 });
  }
  async function answerCeremony(control, parties, showPIN, signal) {
    const hs = await control.next([TypeKey.AuthSpake2Handshake], signal);
    if (hs.pskStatus !== PskStatus.NeedsPresentation) {
      control.send({ type: TypeKey.AuthStatus, result: AuthResult.UnknownError });
      throw new AuthError("unexpected psk-status from the client", AuthResult.UnknownError);
    }
    if (!showPIN) {
      control.send({ type: TypeKey.AuthStatus, result: AuthResult.Authenticated });
      return;
    }
    const pin = generatePIN();
    const nS = nonce();
    showPIN(pin);
    control.send({ type: TypeKey.AuthSpake2Handshake, pskStatus: PskStatus.Input, publicValue: nS });
    const conf = await control.next([TypeKey.AuthSpake2Confirmation], signal);
    const ok = equal(conf.confirmationValue, await proof(pin, nS, hs.publicValue, parties));
    const result = ok ? AuthResult.Authenticated : AuthResult.ProofInvalid;
    control.send({ type: TypeKey.AuthStatus, result });
    if (!ok) throw new AuthError("the peer entered a wrong PIN", result);
  }
  async function startCeremony(control, parties, enterPIN, signal) {
    const nQ = nonce();
    control.send({ type: TypeKey.AuthSpake2Handshake, pskStatus: PskStatus.NeedsPresentation, publicValue: nQ });
    for (; ; ) {
      const m = await control.next([TypeKey.AuthSpake2Handshake, TypeKey.AuthStatus], signal);
      if (m.type === TypeKey.AuthStatus) {
        if (m.result === AuthResult.Authenticated) return;
        throw new AuthError(m.result === AuthResult.ProofInvalid ? "wrong PIN" : `authentication failed (${m.result})`, m.result);
      }
      if (m.pskStatus !== PskStatus.Input) continue;
      const pin = await enterPIN();
      if (pin === null) throw new AuthError("the user canceled pairing", AuthResult.UnknownError);
      control.send({ type: TypeKey.AuthSpake2Confirmation, confirmationValue: await proof(pin, m.publicValue, nQ, parties) });
    }
  }

  // src/protocol/identity.ts
  var TAG2 = new TextEncoder().encode("lp2p-shim tls v1");
  var subtle = () => globalThis.crypto.subtle;
  var HandshakeError = class extends Error {
    name = "HandshakeError";
  };
  function concat(...parts) {
    const out = new Uint8Array(parts.reduce((n, p) => n + p.length, 0));
    let off = 0;
    for (const p of parts) {
      out.set(p, off);
      off += p.length;
    }
    return out;
  }
  function base64(b) {
    let s = "";
    for (const x of b) s += String.fromCharCode(x);
    return btoa(s);
  }
  async function sha256(b) {
    return new Uint8Array(await subtle().digest("SHA-256", b));
  }
  async function fingerprintOf(spki) {
    return base64(await sha256(spki));
  }
  async function createIdentity() {
    const keys = await subtle().generateKey({ name: "ECDSA", namedCurve: "P-256" }, false, ["sign", "verify"]);
    const spki = new Uint8Array(await subtle().exportKey("spki", keys.publicKey));
    return { keys, spki, fingerprint: await fingerprintOf(spki) };
  }
  async function handshake(control, identity, role, signal) {
    const eph = await subtle().generateKey({ name: "ECDH", namedCurve: "P-256" }, false, ["deriveBits"]);
    const ephRaw = new Uint8Array(await subtle().exportKey("raw", eph.publicKey));
    const nonce2 = crypto.getRandomValues(new Uint8Array(32));
    control.send({ type: TypeKey.ShimHello, signingKey: identity.spki, exchangeKey: ephRaw, nonce: nonce2 });
    const hello = await control.next([TypeKey.ShimHello], signal);
    const mine = concat(identity.spki, ephRaw, nonce2);
    const theirs = concat(hello.signingKey, hello.exchangeKey, hello.nonce);
    const transcript = role === "client" ? concat(TAG2, mine, theirs) : concat(TAG2, theirs, mine);
    const transcriptHash = await sha256(transcript);
    const toSign = (r) => concat(transcriptHash, new TextEncoder().encode(r));
    const sig = new Uint8Array(
      await subtle().sign({ name: "ECDSA", hash: "SHA-256" }, identity.keys.privateKey, toSign(role))
    );
    control.send({ type: TypeKey.ShimFinished, signature: sig });
    const fin = await control.next([TypeKey.ShimFinished], signal);
    let peerKey;
    let peerEph;
    try {
      peerKey = await subtle().importKey(
        "spki",
        hello.signingKey,
        { name: "ECDSA", namedCurve: "P-256" },
        false,
        ["verify"]
      );
      peerEph = await subtle().importKey(
        "raw",
        hello.exchangeKey,
        { name: "ECDH", namedCurve: "P-256" },
        false,
        []
      );
    } catch (err) {
      throw new HandshakeError(`bad peer key: ${String(err)}`);
    }
    const peerRole = role === "client" ? "server" : "client";
    const ok = await subtle().verify(
      { name: "ECDSA", hash: "SHA-256" },
      peerKey,
      fin.signature,
      toSign(peerRole)
    );
    if (!ok) throw new HandshakeError("the peer's signature does not verify");
    const secret = new Uint8Array(await subtle().deriveBits({ name: "ECDH", public: peerEph }, eph.privateKey, 256));
    return { peerFingerprint: await fingerprintOf(hello.signingKey), secret, transcriptHash };
  }
  async function exportKeyingMaterial(h, label, context, length) {
    const ikm = await subtle().importKey("raw", h.secret, "HKDF", false, ["deriveBits"]);
    const info = concat(new TextEncoder().encode(label), new Uint8Array([0]), context);
    const bits = await subtle().deriveBits(
      { name: "HKDF", hash: "SHA-256", salt: h.transcriptHash, info },
      ikm,
      length * 8
    );
    return new Uint8Array(bits);
  }
  function webTransportExporterContext(exchangeId, label, context) {
    if (label.length > 255 || context.length > 255) throw new RangeError("label and context are at most 255 bytes");
    const out = new Uint8Array(8 + 1 + label.length + 1 + context.length);
    const view = new DataView(out.buffer);
    view.setBigUint64(0, BigInt(exchangeId));
    out[8] = label.length;
    out.set(label, 9);
    out[9 + label.length] = context.length;
    out.set(context, 10 + label.length);
    return out;
  }

  // src/protocol/webtransport.ts
  var StreamFrame = {
    Data: 0,
    Fin: 1,
    Reset: 2,
    StopSending: 3
  };
  var SESSION_GONE = 2 ** 32;
  var HOLD_MS = 2e3;
  var CLOSE_LINGER_MS = 2e3;
  function frame(tag, body) {
    const out = new Uint8Array(1 + (body?.length ?? 0));
    out[0] = tag;
    if (body) out.set(body, 1);
    return out;
  }
  var WTStream = class {
    unidirectional;
    /** True if this side opened it. */
    outgoing;
    bytesSent = 0;
    bytesReceived = 0;
    onfinished = null;
    sendDone;
    recvDone;
    sink = null;
    held = [];
    stream;
    constructor(stream, unidirectional, outgoing) {
      this.stream = stream;
      this.unidirectional = unidirectional;
      this.outgoing = outgoing;
      this.sendDone = unidirectional && !outgoing;
      this.recvDone = unidirectional && outgoing;
      stream.onmessage = (b) => this.receive(b);
      stream.onclose = () => this.closedUnderneath();
    }
    /** Delivers held events, then everything that follows, to sink. */
    attach(sink) {
      this.sink = sink;
      for (const ev of this.held.splice(0)) ev(sink);
    }
    get sending() {
      return !this.sendDone;
    }
    send(chunk) {
      if (this.sendDone) throw new DOMException("stream is not writable", "InvalidStateError");
      this.stream.send(frame(StreamFrame.Data, chunk));
      this.bytesSent += chunk.length;
    }
    fin() {
      if (this.sendDone) return;
      this.sendDone = true;
      this.stream.send(frame(StreamFrame.Fin));
      this.maybeClose();
    }
    reset(code) {
      if (this.sendDone) return;
      this.sendDone = true;
      this.stream.send(frame(StreamFrame.Reset, encodeVarint(code)));
      this.maybeClose();
    }
    stopSending(code) {
      if (this.recvDone) return;
      this.recvDone = true;
      this.stream.send(frame(StreamFrame.StopSending, encodeVarint(code)));
      this.maybeClose();
    }
    /**
     * Refuses a stream nobody takes: STOP_SENDING and, for a bidirectional
     * stream, RESET_STREAM, with code (session-gone).
     */
    refuse(code) {
      if (!this.stream.closed) {
        this.stream.send(frame(StreamFrame.StopSending, encodeVarint(code)));
        if (!this.unidirectional) this.stream.send(frame(StreamFrame.Reset, encodeVarint(code)));
        this.stream.close();
      }
      this.sendDone = this.recvDone = true;
    }
    /** The session closed: reset and stop the stream with session-gone, and tell the sink. */
    sessionGone(reason) {
      if (this.sendDone && this.recvDone) return;
      if (!this.stream.closed) {
        if (!this.recvDone) this.stream.send(frame(StreamFrame.StopSending, encodeVarint(SESSION_GONE)));
        if (!this.sendDone) this.stream.send(frame(StreamFrame.Reset, encodeVarint(SESSION_GONE)));
        this.stream.close();
      }
      this.sendDone = this.recvDone = true;
      this.emit((s) => s.abort(reason));
      this.onfinished?.();
    }
    emit(ev) {
      if (this.sink) ev(this.sink);
      else this.held.push(ev);
    }
    receive(b) {
      const tag = b[0];
      switch (tag) {
        case StreamFrame.Data:
          if (this.recvDone) return;
          this.bytesReceived += b.length - 1;
          this.emit((s) => s.data(b.subarray(1)));
          break;
        case StreamFrame.Fin:
          if (this.recvDone) return;
          this.recvDone = true;
          this.emit((s) => s.fin());
          this.maybeClose();
          break;
        case StreamFrame.Reset: {
          if (this.recvDone) return;
          this.recvDone = true;
          const [code] = decodeVarint(b, 1);
          if (code >= SESSION_GONE) this.emit((s) => s.abort("the session is gone"));
          else this.emit((s) => s.reset(code));
          this.maybeClose();
          break;
        }
        case StreamFrame.StopSending: {
          if (this.sendDone) return;
          const [code] = decodeVarint(b, 1);
          if (code >= SESSION_GONE) {
            this.sendDone = true;
            this.emit((s) => s.abort("the session is gone"));
            this.maybeClose();
          } else {
            this.emit((s) => s.stopSending(code));
          }
          break;
        }
      }
    }
    maybeClose() {
      if (!this.sendDone || !this.recvDone) return;
      this.stream.close();
      this.onfinished?.();
    }
    closedUnderneath() {
      if (this.sendDone && this.recvDone) return;
      this.sendDone = this.recvDone = true;
      this.emit((s) => s.abort("stream closed"));
      this.onfinished?.();
    }
  };
  var WTSession = class {
    exchangeId;
    outgoing;
    state;
    bytesSent = 0;
    bytesReceived = 0;
    datagramsSent = 0;
    datagramsReceived = 0;
    /** Outgoing: the peer accepted. */
    onready = null;
    /** The peer sent data-transport-drain. */
    ondrain = null;
    /** Called once: info is null for an abrupt close (refused, or the connection went away). */
    onclose = null;
    host;
    control;
    requestIdValue;
    streams = /* @__PURE__ */ new Set();
    heldStreams = [];
    /** Incoming streams that arrived while this side was still waiting for 1202. */
    early = [];
    earlyTimer = null;
    streamHandler = null;
    heldDatagrams = [];
    datagramHandler = null;
    prefix;
    peerClose = null;
    lingering = false;
    constructor(host, exchangeId, control, outgoing, requestId) {
      this.host = host;
      this.exchangeId = exchangeId;
      this.control = control;
      this.outgoing = outgoing;
      this.requestIdValue = requestId;
      this.state = outgoing ? "connecting" : "pending";
      this.prefix = encodeVarint(exchangeId);
      control.onmessage = (b) => this.receiveControl(b);
      control.onclose = () => this.controlEnded();
      if (outgoing) {
        control.send(encodeMessage({ type: TypeKey.DataTransportStartRequest, requestId, exchangeId }));
      }
    }
    get maxDatagramSize() {
      return Math.max(0, this.host.maxDatagramSize - this.prefix.length);
    }
    get datagramsReliable() {
      return this.host.datagramsReliable;
    }
    /** Incoming streams. Streams that arrive while this is null are held. */
    set onstream(fn) {
      this.streamHandler = fn;
      while (fn && this.heldStreams.length > 0) fn(this.heldStreams.shift());
    }
    /** Incoming datagrams. Up to 64 that arrive while this is null are held. */
    set ondatagram(fn) {
      this.datagramHandler = fn;
      while (fn && this.heldDatagrams.length > 0) fn(this.heldDatagrams.shift());
    }
    /** Incoming, pending: this user agent accepts the session. */
    accept() {
      if (this.state !== "pending") return;
      this.state = "open";
      this.control.send(
        encodeMessage({ type: TypeKey.DataTransportStartResponse, requestId: this.requestIdValue, result: Result.Success })
      );
    }
    /** Incoming, pending: refuses the session (permanent-error, then FIN). */
    refuse() {
      if (this.state !== "pending") return;
      this.control.send(
        encodeMessage({ type: TypeKey.DataTransportStartResponse, requestId: this.requestIdValue, result: Result.PermanentError })
      );
      this.teardown(null, "refused");
    }
    openStream(unidirectional) {
      if (this.state !== "open") throw new DOMException("session is not open", "InvalidStateError");
      const t = this.host.openStream(unidirectional);
      t.send(encodeMessage({ type: TypeKey.DataTransportStream, exchangeId: this.exchangeId }));
      const s = new WTStream(t, unidirectional, true);
      this.track(s);
      return s;
    }
    /** From the session layer: the peer opened a stream for this session. */
    addIncomingStream(s) {
      if (this.state === "connecting") {
        this.early.push(s);
        this.earlyTimer ??= setTimeout(() => {
          for (const e of this.early.splice(0)) e.refuse(SESSION_GONE);
        }, HOLD_MS);
        return;
      }
      if (this.state !== "open") {
        s.refuse(SESSION_GONE);
        return;
      }
      this.track(s);
      if (this.streamHandler) this.streamHandler(s);
      else this.heldStreams.push(s);
    }
    sendDatagram(payload) {
      if (this.state !== "open") throw new DOMException("session is not open", "InvalidStateError");
      const d = new Uint8Array(this.prefix.length + payload.length);
      d.set(this.prefix);
      d.set(payload, this.prefix.length);
      this.host.sendDatagram(d);
      this.datagramsSent++;
      this.bytesSent += payload.length;
    }
    /** From the session layer: a datagram for this session, without its prefix. */
    receiveDatagram(payload) {
      if (this.state !== "open") return;
      this.datagramsReceived++;
      this.bytesReceived += payload.length;
      if (this.datagramHandler) {
        this.datagramHandler(payload);
        return;
      }
      this.heldDatagrams.push(payload);
      if (this.heldDatagrams.length > 64) this.heldDatagrams.shift();
    }
    /** Sends data-transport-drain: asks the peer to finish and close. */
    drain() {
      if (this.state === "open") this.control.send(encodeMessage({ type: TypeKey.DataTransportDrain }));
    }
    /** Keying material that only this session yields (spec "export keying material"). */
    exportKeyingMaterial(label, context, length) {
      return this.host.exportKeyingMaterial(
        "EXPORTER-WebTransport",
        webTransportExporterContext(this.exchangeId, label, context),
        length
      );
    }
    /**
     * Closes the session. When open, tells the peer code and reason (1205)
     * and ends the control stream; when connecting, abandons it; when
     * pending, refuses it.
     */
    close(code = 0, reason = "") {
      if (this.state === "closed") return;
      if (this.state === "pending") {
        this.refuse();
        return;
      }
      if (this.state === "open") {
        this.control.send(encodeMessage({ type: TypeKey.DataTransportClose, code, reason }));
        this.lingering = true;
        const done = new Promise((resolve) => {
          const t = setTimeout(() => {
            this.control.close();
            resolve();
          }, CLOSE_LINGER_MS);
          this.control.onclose = () => {
            clearTimeout(t);
            resolve();
          };
        });
        this.host.closing.add(done);
        void done.then(() => this.host.closing.delete(done));
      }
      this.teardown({ code, reason }, "closed locally");
    }
    /** Payload bytes sent and received so far, on streams and in datagrams. */
    totals() {
      let sent = this.bytesSent;
      let received = this.bytesReceived;
      for (const s of this.streams) {
        sent += s.bytesSent;
        received += s.bytesReceived;
      }
      return { sent, received };
    }
    track(s) {
      this.streams.add(s);
      s.onfinished = () => {
        this.streams.delete(s);
        this.bytesSent += s.bytesSent;
        this.bytesReceived += s.bytesReceived;
      };
    }
    receiveControl(b) {
      let m;
      try {
        m = decodeMessage(b);
      } catch {
        return;
      }
      if (m.type === TypeKey.DataTransportStartResponse && this.state === "connecting") {
        if (m.requestId !== this.requestIdValue || m.result !== Result.Success) {
          this.teardown(null, `the peer refused the session (result ${m.result})`);
          return;
        }
        this.state = "open";
        this.onready?.();
        if (this.earlyTimer) clearTimeout(this.earlyTimer);
        for (const s of this.early.splice(0)) this.addIncomingStream(s);
      } else if (m.type === TypeKey.DataTransportClose && this.state === "open") {
        this.peerClose = { code: m.code, reason: m.reason };
        this.teardown(this.peerClose, "closed by the peer");
      } else if (m.type === TypeKey.DataTransportDrain && this.state === "open") {
        this.ondrain?.();
      }
    }
    /** The control stream ended: FIN without 1205 is a clean close with code 0; a lost connection is abrupt. */
    controlEnded() {
      if (this.state === "closed") return;
      if (this.state === "open" && !this.host.connectionClosed) {
        this.teardown({ code: 0, reason: "" }, "closed by the peer");
        return;
      }
      const why = this.state === "connecting" ? "the peer refused the session" : this.state === "pending" ? "the peer abandoned the session" : "connection closed";
      this.teardown(null, why);
    }
    teardown(info, why) {
      if (this.state === "closed") return;
      this.state = "closed";
      if (this.earlyTimer) clearTimeout(this.earlyTimer);
      for (const s of this.early.splice(0)) s.refuse(SESSION_GONE);
      if (!this.lingering) this.control.close();
      for (const s of [...this.streams]) s.sessionGone(why);
      this.streams.clear();
      for (const s of this.heldStreams.splice(0)) s.refuse(SESSION_GONE);
      this.heldDatagrams.length = 0;
      this.host.forgetTransport(this.exchangeId);
      this.onclose?.(info, why);
    }
  };

  // src/protocol/session.ts
  var ProtocolError = class extends Error {
    name = "ProtocolError";
  };
  var CONTROL_TYPES = [
    TypeKey.ShimHello,
    TypeKey.AgentInfoRequest,
    TypeKey.AgentInfoResponse,
    TypeKey.AuthCapabilities,
    TypeKey.AuthSpake2Handshake
  ];
  var REFUSAL_LINGER_MS = 2e3;
  var ControlChannel = class {
    queue = [];
    waiters = [];
    closedWith = null;
    info = null;
    unanswered = [];
    stream;
    /** first is a message already read from the stream, handled before any that follow. */
    constructor(stream, first) {
      this.stream = stream;
      if (first) this.feed(first);
      stream.onmessage = (b) => this.feed(b);
      stream.onclose = () => this.fail(new ProtocolError("control stream closed"));
    }
    send(m) {
      if (!this.stream.closed) this.stream.send(encodeMessage(m));
    }
    /** What to answer agent-info-request with. Requests that came earlier are answered now. */
    set localInfo(info) {
      this.info = info;
      for (const requestId of this.unanswered.splice(0)) {
        this.send({ type: TypeKey.AgentInfoResponse, requestId, agentInfo: info });
      }
    }
    /**
     * Resolves with the oldest message of one of types, now or when it
     * arrives. Messages of other types stay queued for other callers.
     */
    next(types, signal) {
      const i = this.queue.findIndex((m) => types.includes(m.type));
      if (i >= 0) return Promise.resolve(this.queue.splice(i, 1)[0]);
      if (this.closedWith) return Promise.reject(this.closedWith);
      if (signal?.aborted) return Promise.reject(signal.reason);
      return new Promise((resolve, reject) => {
        const w = {
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
        const onAbort = () => {
          const j = this.waiters.indexOf(w);
          if (j >= 0) this.waiters.splice(j, 1);
          reject(signal?.reason);
        };
        signal?.addEventListener("abort", onAbort, { once: true });
        this.waiters.push(w);
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
      const i = this.waiters.findIndex((w) => w.types.includes(m.type));
      if (i >= 0) this.waiters.splice(i, 1)[0].resolve(m);
      else this.queue.push(m);
    }
    fail(err) {
      if (this.closedWith) return;
      this.closedWith = err;
      for (const w of this.waiters.splice(0)) w.reject(err);
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
    /** Set by the handshake: the peer's proven identity and the exporter secret. */
    tls = null;
    control = null;
    controlWaiters = [];
    usedIds = /* @__PURE__ */ new Set();
    parity;
    nextRequestId = 1;
    incoming = [];
    channelHandler = null;
    closeHandler = null;
    closeListeners = [];
    closeReason = null;
    transports = /* @__PURE__ */ new Map();
    nextExchangeId;
    incomingTransports = [];
    transportHandler = null;
    sessionRequestHandler = null;
    heldSessionRequest = null;
    sawSessionRequest = false;
    isAccepted = false;
    /** Client: streams the server opened before its 1111 arrived. */
    early = [];
    connection;
    constructor(connection, role) {
      this.connection = connection;
      this.role = role;
      this.parity = role === "client" ? 0 : 1;
      this.nextExchangeId = this.parity;
      if (role === "client") this.setControl(new ControlChannel(connection.openStream()));
      connection.onstream = (s) => this.classify(s);
      connection.ondatagram = (d) => this.receiveDatagram(d);
      connection.onclose = (reason) => {
        this.closeReason = reason;
        this.early.length = 0;
        this.closeHandler?.(reason);
        for (const fn of this.closeListeners.splice(0)) fn(reason);
      };
    }
    get closed() {
      return this.closeReason !== null;
    }
    /** True once the server sent, or the client received, a success lp2p-session-response. */
    get accepted() {
      return this.isAccepted;
    }
    set onclose(fn) {
      this.closeHandler = fn;
      if (fn && this.closeReason !== null) fn(this.closeReason);
    }
    /** Like onclose, for any number of observers; called at once if already closed. */
    whenClosed(fn) {
      if (this.closeReason !== null) fn(this.closeReason);
      else this.closeListeners.push(fn);
    }
    /** Incoming data channels. Channels that arrive while this is null are held. */
    set ondatachannel(fn) {
      this.channelHandler = fn;
      while (fn && this.incoming.length > 0) fn(this.incoming.shift());
    }
    /** Incoming WebTransport sessions, pending. Sessions that arrive while this is null are held. */
    set ontransport(fn) {
      this.transportHandler = fn;
      while (fn && this.incomingTransports.length > 0) fn(this.incomingTransports.shift());
    }
    /** Server: the client's lp2p-session-request. One that arrives while this is null is held. */
    set onsessionrequest(fn) {
      this.sessionRequestHandler = fn;
      const held = this.heldSessionRequest;
      if (fn && held) {
        this.heldSessionRequest = null;
        fn(held);
      }
    }
    requestId() {
      return this.nextRequestId++;
    }
    get maxDatagramSize() {
      return this.connection.maxDatagramSize;
    }
    get datagramsReliable() {
      return this.connection.datagramsReliable;
    }
    get connectionClosed() {
      return this.closed || this.connection.closed;
    }
    openStream(unidirectional) {
      return this.connection.openStream({ unidirectional });
    }
    sendDatagram(d) {
      this.connection.sendDatagram(d);
    }
    forgetTransport(exchangeId) {
      this.transports.delete(exchangeId);
      const i = this.incomingTransports.findIndex((t) => t.exchangeId === exchangeId);
      if (i >= 0) this.incomingTransports.splice(i, 1);
    }
    /** Keying material for one WebTransport session (exporter of the connection's handshake). */
    exportKeyingMaterial(label, context, length) {
      if (!this.tls) return Promise.reject(new DOMException("no handshake", "InvalidStateError"));
      return exportKeyingMaterial(this.tls, label, context, length);
    }
    /** Opens a WebTransport session (1201). It is "connecting" until the peer's user agent accepts it. */
    openTransport() {
      if (this.closed) throw new DOMException("session is closed", "InvalidStateError");
      if (!this.isAccepted) throw new DOMException("session is not accepted yet", "InvalidStateError");
      const id = this.nextExchangeId;
      this.nextExchangeId += 2;
      const t = new WTSession(this, id, this.connection.openStream(), true, this.requestId());
      this.transports.set(id, t);
      return t;
    }
    receiveDatagram(d) {
      let id;
      let n;
      try {
        [id, n] = decodeVarint(d);
      } catch {
        return;
      }
      this.transports.get(id)?.receiveDatagram(d.subarray(n));
    }
    /** The control stream, once the client has opened it. */
    controlChannel(signal) {
      if (this.control) return Promise.resolve(this.control);
      return new Promise((resolve, reject) => {
        this.controlWaiters.push(resolve);
        signal?.addEventListener("abort", () => reject(signal.reason), { once: true });
      });
    }
    /** Exchanges agent-info: answers the peer's request and asks for the peer's. */
    async exchangeInfo(local, signal) {
      const control = await this.controlChannel(signal);
      control.localInfo = local;
      control.send({ type: TypeKey.AgentInfoRequest, requestId: this.requestId() });
      const res = await control.next([TypeKey.AgentInfoResponse], signal);
      this.remoteInfo = res.agentInfo;
      return res.agentInfo;
    }
    /**
     * Client: sends lp2p-session-request with origin on a new stream. The
     * promise resolves with the server's result, or rejects if the stream or
     * the connection closes first.
     */
    requestSession(origin) {
      const stream = this.connection.openStream();
      const requestId = this.requestId();
      stream.send(encodeMessage({ type: TypeKey.SessionRequest, requestId, requestingOrigin: origin }));
      return new Promise((resolve, reject) => {
        stream.onmessage = (b) => {
          let m;
          try {
            m = decodeMessage(b);
          } catch {
            return;
          }
          if (m.type !== TypeKey.SessionResponse || m.requestId !== requestId) return;
          resolve(m.result);
          stream.onclose = null;
          stream.close();
        };
        stream.onclose = () => reject(new ProtocolError("session stream closed without a response"));
      });
    }
    /**
     * Allocates the lowest free channel ID of the local parity, or validates
     * and reserves a requested one, which must be free and of that parity.
     */
    reserveChannelId(requested) {
      if (requested !== void 0) {
        if (requested % 2 !== this.parity) {
          throw new DOMException(`data channel id ${requested} has the peer's parity`, "OperationError");
        }
        if (this.usedIds.has(requested)) throw new DOMException(`data channel id ${requested} is in use`, "OperationError");
        this.usedIds.add(requested);
        return requested;
      }
      let id = this.parity;
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
    /**
     * Closes the connection. WebTransport sessions that sent
     * data-transport-close get until their peer ended the control stream, or
     * a timeout, so the close isn't lost.
     */
    close(reason) {
      if (this.closing.size === 0) {
        this.connection.close(reason);
        return;
      }
      void Promise.all(this.closing).then(() => this.connection.close(reason));
    }
    /** WebTransport sessions waiting for the peer to end their control stream. Internal. */
    closing = /* @__PURE__ */ new Set();
    setControl(c) {
      this.control = c;
      for (const w of this.controlWaiters.splice(0)) w(c);
    }
    /**
     * The session is accepted: the server sent success, or the client got
     * success and verified the server. Streams the client held are routed now.
     */
    markAccepted() {
      if (this.isAccepted) return;
      this.isAccepted = true;
      for (const [stream, first] of this.early.splice(0)) this.route(stream, first);
    }
    /** Looks at a new stream's first message to tell what it is. */
    classify(stream) {
      stream.onmessage = (b) => {
        stream.onmessage = null;
        this.route(stream, b);
      };
    }
    route(stream, b) {
      let m;
      try {
        m = decodeMessage(b);
      } catch {
        stream.close();
        return;
      }
      if (this.role === "server" && !this.control && CONTROL_TYPES.includes(m.type)) {
        this.setControl(new ControlChannel(stream, b));
        return;
      }
      if (m.type === TypeKey.SessionRequest && this.role === "server") {
        this.receiveSessionRequest(stream, m.requestId, m.requestingOrigin);
        return;
      }
      const extension = m.type === TypeKey.DataChannelOpenRequest || m.type === TypeKey.DataTransportStartRequest || m.type === TypeKey.DataTransportStream;
      if (!extension) {
        stream.close();
        return;
      }
      if (!this.isAccepted) {
        if (this.role === "client") this.early.push([stream, b]);
        else stream.close();
        return;
      }
      if (m.type === TypeKey.DataChannelOpenRequest) {
        this.acceptDataChannel(stream, m);
      } else if (m.type === TypeKey.DataTransportStartRequest) {
        this.acceptTransport(stream, m.requestId, m.exchangeId);
      } else if (m.type === TypeKey.DataTransportStream) {
        const ws = new WTStream(stream, stream.unidirectional, false);
        const t = this.transports.get(m.exchangeId);
        if (t) t.addIncomingStream(ws);
        else ws.refuse(SESSION_GONE);
      }
    }
    receiveSessionRequest(stream, requestId, requestingOrigin) {
      if (this.sawSessionRequest) {
        this.close("second lp2p-session-request");
        return;
      }
      this.sawSessionRequest = true;
      let answered = false;
      const req = {
        requestingOrigin,
        respond: (result) => {
          if (answered || stream.closed) return Promise.resolve();
          answered = true;
          stream.send(encodeMessage({ type: TypeKey.SessionResponse, requestId, result }));
          if (result === Result.Success) {
            this.markAccepted();
            stream.close();
            return Promise.resolve();
          }
          return new Promise((resolve) => {
            const t = setTimeout(resolve, REFUSAL_LINGER_MS);
            stream.onclose = () => {
              clearTimeout(t);
              resolve();
            };
          });
        }
      };
      if (this.sessionRequestHandler) this.sessionRequestHandler(req);
      else this.heldSessionRequest = req;
    }
    acceptTransport(stream, requestId, exchangeId) {
      if (this.transports.has(exchangeId) || exchangeId % 2 === this.parity) {
        stream.send(encodeMessage({ type: TypeKey.DataTransportStartResponse, requestId, result: Result.PermanentError }));
        stream.close();
        return;
      }
      const t = new WTSession(this, exchangeId, stream, false, requestId);
      this.transports.set(exchangeId, t);
      if (this.transportHandler) this.transportHandler(t);
      else this.incomingTransports.push(t);
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

  // src/ua/localstate.ts
  var LocalState = class {
    anchors = /* @__PURE__ */ new Map();
    knownPeers = /* @__PURE__ */ new Map();
    grants = /* @__PURE__ */ new Map();
    /** Receivers currently advertising on this device, in start order. */
    receivers = /* @__PURE__ */ new Set();
    #identity = null;
    /** The device's agent key pair, created on first use. */
    identity() {
      return this.#identity ??= createIdentity();
    }
    /** Whether identity chains to an anchor in the trust store (spec "authenticated"). Derived, never stored. */
    isAuthenticated(identity) {
      for (const a of this.anchors.values()) {
        if (a.scope === "one-peer" && a.subject === identity && a.key === identity) return true;
      }
      return false;
    }
    /** A trust ceremony with a self-anchored peer adds a one-peer anchor on its key. */
    addSelfAnchor(identity) {
      this.anchors.set(`one-peer:${identity}`, { key: identity, scope: "one-peer", subject: identity });
    }
    /** Adds a vouching anchor, which replaces the one-peer anchors with the same key. */
    addVouchingAnchor(key) {
      for (const [id, a] of this.anchors) if (a.scope === "one-peer" && a.key === key) this.anchors.delete(id);
      this.anchors.set(`vouching:${key}`, { key, scope: "vouching", subject: "" });
    }
    /** Removes an anchor, and the grants of peers that are no longer authenticated. */
    removeAnchor(anchor) {
      for (const [id, a] of this.anchors) {
        if (a.key === anchor.key && a.scope === anchor.scope && a.subject === anchor.subject) this.anchors.delete(id);
      }
      for (const peers of this.grants.values()) {
        for (const p of [...peers]) if (!this.isAuthenticated(p)) peers.delete(p);
      }
    }
    rememberPeer(p) {
      this.knownPeers.set(p.identity, p);
    }
    /** Removes a known peer, and it from all peer grants. */
    forgetPeer(identity) {
      this.knownPeers.delete(identity);
      for (const peers of this.grants.values()) peers.delete(identity);
    }
    hasGrant(origin, identity) {
      return this.grants.get(origin)?.has(identity) ?? false;
    }
    addGrant(origin, identity) {
      let s = this.grants.get(origin);
      if (!s) this.grants.set(origin, s = /* @__PURE__ */ new Set());
      s.add(identity);
    }
    /** Clearing an origin's site data or permissions removes its peer grants. */
    clearOrigin(origin) {
      this.grants.delete(origin);
    }
  };
  var perTransport = /* @__PURE__ */ new WeakMap();
  function localStateFor(transport) {
    let s = perTransport.get(transport);
    if (!s) perTransport.set(transport, s = new LocalState());
    return s;
  }
  function parseOrigin(value) {
    try {
      return new URL(value).origin;
    } catch {
      return null;
    }
  }

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
    deviceName;
    local;
    modelName;
    handshakeTimeout;
    log;
    constructor(config2) {
      this.transport = config2.transport;
      this.ui = config2.ui;
      this.pairing = config2.pairing ?? "pin";
      this.origin = config2.origin ?? globalThis.location?.origin ?? "null";
      this.deviceName = config2.deviceName ?? "lp2p-shim device";
      this.modelName = config2.modelName ?? "lp2p-shim";
      this.handshakeTimeout = config2.handshakeTimeout ?? 1e4;
      this.local = config2.localState ?? localStateFor(config2.transport);
      this.log = config2.log ?? (() => {
      });
    }
    /**
     * LP2PReceiver.start(): asks to advertise as nickname and, if allowed,
     * advertises until closed. Accepted sessions go to a receiver of this
     * device that allows their requesting origin, preferring receiver.
     */
    async advertise(receiver) {
      const allowed = await this.ui.requestPermission(
        { permission: "advertise", origin: receiver.origin, nickname: receiver.nickname },
        new AbortController().signal
      );
      if (!allowed) throw notAllowed("The user did not allow advertising.");
      const id = randomId();
      this.log(`advertising "${receiver.nickname}" as ${id} over ${this.transport.name}`);
      this.local.receivers.add(receiver);
      const ad = this.transport.listen({ id, nickname: receiver.nickname }, (conn) => {
        this.admit(conn, receiver).catch(
          (err) => this.log(`incoming connection from ${conn.remoteId} failed: ${String(err)}`)
        );
      });
      return {
        close: () => {
          this.local.receivers.delete(receiver);
          ad.close();
        }
      };
    }
    /** The server side of a new connection. */
    async admit(conn, own) {
      const session = new Session(conn, "server");
      const ac = new AbortController();
      session.onclose = (reason) => ac.abort(new DOMException(`connection closed: ${reason}`, "AbortError"));
      const refuse = (req, why) => {
        this.log(`refused session: ${why}`);
        void req.respond(Result.PermanentError).then(() => session.close(`refused: ${why}`));
      };
      try {
        const identity = await this.local.identity();
        const request = new Promise((resolve, reject) => {
          session.onsessionrequest = resolve;
          ac.signal.addEventListener("abort", () => reject(ac.signal.reason), { once: true });
        });
        let info;
        const { control, tls, req } = await this.withTimeout(
          (async () => {
            const control2 = await session.controlChannel(ac.signal);
            const tls2 = await handshake(control2, identity, "server", ac.signal);
            session.tls = tls2;
            info = session.exchangeInfo(this.agentInfo(), ac.signal);
            info.catch(() => {
            });
            return { control: control2, tls: tls2, req: await request };
          })(),
          ac
        );
        const hint = parseOrigin(req.requestingOrigin) ?? "null";
        if (!this.receiverFor(hint, own)) {
          refuse(req, `no receiver allows ${hint}`);
          return;
        }
        const parties = { client: tls.peerFingerprint, server: identity.fingerprint };
        const remote = await this.withTimeout(info, ac);
        const peer = { id: conn.remoteId, nickname: "", displayName: remote.displayName, verified: false };
        const showPIN = this.pairing === "pin" ? this.pinPresenter(own, peer, ac.signal) : null;
        if (this.local.isAuthenticated(tls.peerFingerprint)) {
          const live2 = new AbortController();
          session.whenClosed(() => live2.abort(new DOMException("connection closed", "AbortError")));
          const show = this.pairing === "pin" ? this.pinPresenter(own, peer, live2.signal) : null;
          answerCeremony(control, parties, show, live2.signal).catch((err) => {
            if (err instanceof AuthError) session.close("authentication after success failed");
          });
        } else {
          sendAuthCapabilities(control);
          await answerCeremony(control, parties, showPIN, ac.signal);
          this.local.addSelfAnchor(tls.peerFingerprint);
        }
        peer.verified = true;
        const receiver = this.receiverFor(hint, own);
        if (!receiver) {
          refuse(req, `no receiver allows ${hint} anymore`);
          return;
        }
        this.local.rememberPeer({ identity: tls.peerFingerprint, anchorKey: tls.peerFingerprint, displayName: remote.displayName });
        if (!this.local.hasGrant(receiver.origin, tls.peerFingerprint)) {
          const ok = await this.ui.requestPermission(
            { permission: "accept-connection", origin: receiver.origin, nickname: receiver.nickname, peer },
            ac.signal
          );
          if (!ok) {
            refuse(req, "the user declined");
            return;
          }
          this.local.addGrant(receiver.origin, tls.peerFingerprint);
        }
        req.respond(Result.Success);
        session.onclose = null;
        setTimeout(() => {
          if (!receiver.deliver(session)) session.close("receiver stopped");
        }, 0);
      } catch (err) {
        session.close(String(err));
        throw err;
      } finally {
        ac.abort();
      }
    }
    /** A started receiver of this device that allows origin, preferring preferred. */
    receiverFor(origin, preferred) {
      if (this.local.receivers.has(preferred) && preferred.allows(origin)) return preferred;
      for (const r of this.local.receivers) if (r.allows(origin)) return r;
      return null;
    }
    pinPresenter(own, peer, signal) {
      return (pin) => {
        const shown = new AbortController();
        signal.addEventListener("abort", () => shown.abort(), { once: true });
        this.ui.presentPIN({ pin, origin: own.origin, nickname: own.nickname, peer }, shown.signal);
      };
    }
    /**
     * LP2PRequest.start(): shows the picker, connects to the chosen peer,
     * runs the session request and, when needed, the trust ceremony. Rejects
     * with NotAllowedError on every failure, as the spec requires.
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
        throw notAllowed(`Could not connect to the peer: ${String(err)}`);
      }
      const session = new Session(conn, "client");
      const ac = new AbortController();
      session.onclose = (reason) => ac.abort(new DOMException(`connection closed: ${reason}`, "NetworkError"));
      try {
        const identity = await this.local.identity();
        const control = await session.controlChannel(ac.signal);
        const tls = await this.withTimeout(handshake(control, identity, "client", ac.signal), ac);
        session.tls = tls;
        const remote = await this.withTimeout(session.exchangeInfo(this.agentInfo(), ac.signal), ac);
        const result = session.requestSession(this.origin);
        result.catch(() => {
        });
        const cue = control.next([TypeKey.AuthCapabilities], ac.signal).then(() => "cue");
        cue.catch(() => {
        });
        const parties = { client: identity.fingerprint, server: tls.peerFingerprint };
        const trusted = this.local.isAuthenticated(tls.peerFingerprint);
        const first = await Promise.race([cue, result]);
        if (first !== "cue" && first !== Result.Success) throw notAllowed("The peer refused the session.");
        const verified = { ...peer, displayName: remote.displayName, verified: false };
        if (first === "cue" || !trusted) {
          await startCeremony(control, parties, () => this.ui.enterPIN({ origin: this.origin, nickname, peer: verified }, ac.signal), ac.signal);
          this.local.addSelfAnchor(tls.peerFingerprint);
        }
        verified.verified = true;
        if (await result !== Result.Success) throw notAllowed("The peer refused the session.");
        session.markAccepted();
        this.local.rememberPeer({ identity: tls.peerFingerprint, anchorKey: tls.peerFingerprint, displayName: remote.displayName });
        this.local.addGrant(this.origin, tls.peerFingerprint);
        session.onclose = null;
        return { session, peer: verified };
      } catch (err) {
        session.close(String(err));
        if (err instanceof DOMException && err.name === "NotAllowedError") throw err;
        if (err instanceof AuthError) throw notAllowed(`Pairing failed: ${err.message}.`);
        throw notAllowed(`Connection setup failed: ${String(err instanceof Error ? err.message : err)}`);
      } finally {
        ac.abort();
      }
    }
    agentInfo() {
      return {
        displayName: this.deviceName,
        modelName: this.modelName,
        capabilities: [AgentCapability.DataChannels, AgentCapability.QuicTransport],
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
  function getConfig() {
    return config;
  }
  function getUserAgent() {
    if (current) return current;
    const hasDOM = typeof document !== "undefined";
    current = new UserAgent({
      transport: config.transport ?? (config.transport = new PostMessageTransport()),
      ui: config.ui ?? (hasDOM ? new DomUI() : new AutoUI()),
      pairing: config.pairing ?? (config.ui || hasDOM ? "pin" : "none"),
      ...config.origin !== void 0 ? { origin: config.origin } : {},
      ...config.deviceName !== void 0 ? { deviceName: config.deviceName } : {},
      ...config.localState !== void 0 ? { localState: config.localState } : {},
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
      if (!a.incoming) ep.onframe = (enc, payload) => this.#receive(enc, payload);
      ep.onerror = (err) => {
        const ev = new Event("error");
        Object.defineProperty(ev, "error", { value: err });
        this.dispatchEvent(ev);
      };
      ep.onclose = () => this.#closed();
      if (a.incoming) {
        queueMicrotask(() => {
          if (this.#readyState === "open") this.dispatchEvent(new Event("open"));
          ep.onframe = (enc, payload) => this.#receive(enc, payload);
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
      let frame2;
      if (typeof data === "string") {
        frame2 = [DataEncoding.String, textEncoder2.encode(data)];
      } else if (data instanceof ArrayBuffer) {
        frame2 = [DataEncoding.ArrayBuffer, new Uint8Array(data.slice(0))];
      } else if (ArrayBuffer.isView(data)) {
        frame2 = [DataEncoding.ArrayBuffer, new Uint8Array(data.buffer, data.byteOffset, data.byteLength).slice()];
      } else if (typeof Blob !== "undefined" && data instanceof Blob) {
        frame2 = data.arrayBuffer().then((b) => [DataEncoding.Blob, new Uint8Array(b)]);
      } else {
        frame2 = [DataEncoding.String, textEncoder2.encode(String(data))];
      }
      if (!(frame2 instanceof Promise) && !this.#sendChain) {
        ep.send(frame2[0], frame2[1]);
        return;
      }
      const pending = Promise.resolve(frame2);
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
  var slots = /* @__PURE__ */ new WeakMap();
  function slotsOf(connection) {
    const s = slots.get(connection);
    if (!s) throw new TypeError("not an LP2PConnection");
    return s;
  }
  function adoptConnection(session) {
    adopting2 = session;
    try {
      return new LP2PConnection();
    } finally {
      adopting2 = null;
    }
  }
  var LP2PConnection = class extends EventTarget {
    #slots;
    /** Not constructible from script: the IDL has no constructor. */
    constructor() {
      super();
      const s = adopting2;
      adopting2 = null;
      if (!s) throw new TypeError("Illegal constructor");
      this.#slots = { session: s, transports: /* @__PURE__ */ new Set(), listener: null, closed: false };
      slots.set(this, this.#slots);
      s.whenClosed(() => setTimeout(() => this.#finalize(), 0));
      setTimeout(() => {
        s.ondatachannel = (ep) => {
          const channel = adoptChannel(ep, true);
          this.dispatchEvent(new LP2PDataChannelEvent("datachannel", { channel }));
        };
        s.ontransport = (t) => {
          const listener = this.#slots.listener;
          if (this.#slots.closed || !listener || !listener.incoming(t)) t.refuse();
        };
      }, 0);
    }
    createDataChannel(label, dataChannelDict = {}) {
      if (arguments.length < 1) throw new TypeError("createDataChannel: label is required");
      if (this.#slots.closed) throw new DOMException("LP2PConnection is closed", "InvalidStateError");
      const requested = dataChannelDict.id === void 0 ? void 0 : enforceUnsignedShort(dataChannelDict.id, "id");
      if (requested === 65535) throw new TypeError("id 65535 is reserved");
      const session = this.#slots.session;
      const id = session.reserveChannelId(requested);
      const ep = session.openDataChannel({
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
    /** Closes the session, its data channels and its transports, on both sides; then fires close. */
    close() {
      if (this.#slots.closed) return;
      this.#slots.session.close("closed by page");
      setTimeout(() => this.#finalize(), 0);
    }
    /** Spec "finalize": fail the transports, close the listener, fire close. At most once. */
    #finalize() {
      if (this.#slots.closed) return;
      this.#slots.closed = true;
      for (const t of [...this.#slots.transports]) t.failFromConnection();
      this.#slots.listener?.closeFromConnection();
      this.dispatchEvent(new Event("close"));
    }
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
    #allowedOrigins;
    #state = "initial";
    #startPromise = null;
    #rejectStart = null;
    #advertisement = null;
    constructor(options = {}) {
      super();
      this.#nickname = options?.nickname === void 0 ? "" : String(options.nickname);
      this.#allowedOrigins = [];
      for (const entry of options?.allowedOrigins ?? []) {
        const origin = parseOrigin(String(entry));
        if (origin === null) throw new TypeError(`allowedOrigins: "${String(entry)}" is not a valid URL`);
        this.#allowedOrigins.push(origin);
      }
    }
    /**
     * Asks the user for permission to advertise and starts advertising.
     * Resolves once advertising; rejects with NotAllowedError if denied, and
     * InvalidStateError after stop(). Calling it again returns the same
     * promise while pending or after success.
     */
    start() {
      if (this.#state === "stopped") {
        return Promise.reject(new DOMException("the receiver was stopped", "InvalidStateError"));
      }
      if (this.#startPromise) return this.#startPromise;
      const ua = getUserAgent();
      if (ua.origin === "null") return Promise.reject(new DOMException("opaque origin", "NotAllowedError"));
      const handle = {
        origin: ua.origin,
        nickname: this.#nickname,
        allows: (o) => this.#allows(o, ua.origin),
        deliver: (s) => this.#deliver(s)
      };
      const promise = new Promise((resolve, reject) => {
        this.#rejectStart = reject;
        ua.advertise(handle).then(
          (ad) => {
            if (this.#state === "stopped") {
              ad.close();
              return;
            }
            this.#advertisement = ad;
            this.#state = "advertising";
            this.#rejectStart = null;
            resolve(void 0);
          },
          () => {
            if (this.#startPromise === promise) this.#startPromise = null;
            this.#rejectStart = null;
            reject(new DOMException("The user did not allow advertising.", "NotAllowedError"));
          }
        );
      });
      this.#startPromise = promise;
      return promise;
    }
    /** Stops advertising. Connections already delivered stay open. The receiver can't be started again. */
    stop() {
      if (this.#state === "stopped") return;
      this.#state = "stopped";
      this.#rejectStart?.(new DOMException("stopped", "AbortError"));
      this.#rejectStart = null;
      this.#advertisement?.close();
      this.#advertisement = null;
    }
    #allows(origin, own) {
      if (origin === "null") return false;
      return origin === own || this.#allowedOrigins.includes(origin);
    }
    #deliver(session) {
      if (this.#state !== "advertising") return false;
      const connection = adoptConnection(session);
      this.dispatchEvent(new LP2PConnectionEvent("connection", { connection }));
      return true;
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
      this.#nickname = options?.nickname === void 0 ? "" : String(options.nickname);
    }
    /**
     * Shows the device picker, connects to the chosen peer and, if needed,
     * pairs with it. Rejects with NotAllowedError on every failure: dismissed
     * picker, unreachable peer, failed pairing or refused session, so script
     * can't tell them apart. While pending or after success, calling it again
     * returns the same promise; after a failure it starts over.
     */
    start() {
      if (this.#started) return this.#started;
      const ua = getUserAgent();
      if (ua.origin === "null") return Promise.reject(new DOMException("opaque origin", "NotAllowedError"));
      const p = ua.request(this.#nickname).then(({ session }) => adoptConnection(session));
      this.#started = p;
      p.catch(() => {
        if (this.#started === p) this.#started = null;
      });
      return p;
    }
  };

  // src/api/quictransport.ts
  var WebTransportErrorPolyfill = class extends DOMException {
    #source;
    #code;
    constructor(message = "", options = {}) {
      super(message, "WebTransportError");
      this.#source = options.source ?? "stream";
      const c = options.streamErrorCode;
      this.#code = c === void 0 || c === null ? null : Math.min(Math.max(Math.round(Number(c)) || 0, 0), 4294967295);
    }
    get source() {
      return this.#source;
    }
    get streamErrorCode() {
      return this.#code;
    }
  };
  var native = globalThis.WebTransportError;
  if (native) Object.setPrototypeOf(WebTransportErrorPolyfill.prototype, native.prototype);
  var WebTransportError = WebTransportErrorPolyfill;
  function sessionError(message) {
    return new WebTransportError(message, { source: "session" });
  }
  function codeOf(reason) {
    const c = reason?.streamErrorCode;
    return reason instanceof DOMException && typeof c === "number" ? c : 0;
  }
  function toBytes(chunk) {
    if (chunk instanceof ArrayBuffer) return new Uint8Array(chunk.slice(0));
    if (ArrayBuffer.isView(chunk)) return new Uint8Array(chunk.buffer, chunk.byteOffset, chunk.byteLength).slice();
    throw new TypeError("chunk must be an ArrayBuffer or an ArrayBufferView");
  }
  function handled(p) {
    p.catch(() => {
    });
    return p;
  }
  var CHUNK = 16 * 1024;
  var LP2PSendGroup = class {
    #streams = /* @__PURE__ */ new Set();
    #owner;
    constructor() {
      const o = groupOwner;
      groupOwner = null;
      if (!o) throw new TypeError("Illegal constructor");
      this.#owner = o;
    }
    /** Internal. */
    static ownerOf(g) {
      return g.#owner;
    }
    /** Internal. */
    static add(g, s) {
      g.#streams.add(s);
    }
    getStats() {
      let n = 0;
      for (const s of this.#streams) n += s.bytesSent;
      return Promise.resolve({ bytesWritten: n, bytesSent: n, bytesAcknowledged: n });
    }
  };
  var groupOwner = null;
  var StreamHalves = class {
    readable = null;
    writable = null;
    readableDone = false;
    writableDone = false;
    ws;
    constructor(ws) {
      this.ws = ws;
    }
    data(chunk) {
      if (this.readableDone || !this.readable) return;
      this.readable.enqueue(chunk.slice());
    }
    fin() {
      if (this.readableDone || !this.readable) return;
      this.readableDone = true;
      this.readable.close();
      try {
        this.readable.byobRequest?.respond(0);
      } catch {
      }
    }
    reset(code) {
      this.errorReadable(new WebTransportError("the peer reset the stream", { source: "stream", streamErrorCode: code }));
    }
    stopSending(code) {
      this.errorWritable(new WebTransportError("the peer stopped reading", { source: "stream", streamErrorCode: code }));
      this.ws.reset(code);
    }
    abort(reason) {
      const err = sessionError(reason);
      this.errorReadable(err);
      this.errorWritable(err);
    }
    errorReadable(err) {
      if (this.readableDone || !this.readable) return;
      this.readableDone = true;
      this.readable.error(err);
    }
    errorWritable(err) {
      if (this.writableDone || !this.writable) return;
      this.writableDone = true;
      this.writable.error(err);
    }
  };
  var ByteStream = ReadableStream;
  var LP2PReceiveStream = class extends ByteStream {
    #halves;
    constructor(halves) {
      if (!(halves instanceof StreamHalves)) throw new TypeError("Illegal constructor");
      super({
        type: "bytes",
        start: (c) => {
          halves.readable = c;
        },
        cancel: (reason) => {
          halves.readableDone = true;
          halves.ws.stopSending(codeOf(reason));
        }
      });
      this.#halves = halves;
    }
    getStats() {
      const h = this.#halves;
      const queued = h.readableDone || !h.readable ? 0 : Math.max(0, -(h.readable.desiredSize ?? 0));
      return Promise.resolve({ bytesReceived: h.ws.bytesReceived, bytesRead: h.ws.bytesReceived - queued });
    }
  };
  var LP2PSendStream = class extends WritableStream {
    #halves;
    #transport;
    #sendGroup = null;
    #sendOrder = 0;
    constructor(halves, transport, options) {
      if (!(halves instanceof StreamHalves)) throw new TypeError("Illegal constructor");
      super({
        start: (c) => {
          halves.writable = c;
        },
        write: (chunk) => {
          const bytes2 = toBytes(chunk);
          if (!halves.ws.sending) throw new DOMException("stream is not writable", "InvalidStateError");
          for (let off = 0; off < bytes2.length; off += CHUNK) halves.ws.send(bytes2.subarray(off, off + CHUNK));
        },
        close: () => {
          halves.writableDone = true;
          halves.ws.fin();
        },
        abort: (reason) => {
          halves.writableDone = true;
          halves.ws.reset(codeOf(reason));
        }
      });
      this.#halves = halves;
      this.#transport = transport;
      this.sendGroup = options.sendGroup ?? null;
      this.sendOrder = options.sendOrder ?? 0;
    }
    get sendGroup() {
      return this.#sendGroup;
    }
    set sendGroup(g) {
      if (g !== null && (!(g instanceof LP2PSendGroup) || LP2PSendGroup.ownerOf(g) !== this.#transport)) {
        throw new DOMException("sendGroup belongs to another transport", "InvalidStateError");
      }
      this.#sendGroup = g;
      if (g) LP2PSendGroup.add(g, this.#halves.ws);
    }
    /** Stored, not applied: the shim's transports have no send scheduler to prioritize with. */
    get sendOrder() {
      return this.#sendOrder;
    }
    set sendOrder(v) {
      this.#sendOrder = Math.trunc(Number(v)) || 0;
    }
    getStats() {
      const n = this.#halves.ws.bytesSent;
      return Promise.resolve({ bytesWritten: n, bytesSent: n, bytesAcknowledged: n });
    }
    /** A WebTransportWriter. Without flow control, atomicWrite() is write() and commit() does nothing. */
    getWriter() {
      const w = super.getWriter();
      return Object.assign(w, {
        atomicWrite: (chunk) => w.write(chunk).then(() => void 0),
        commit: () => void 0
      });
    }
  };
  var LP2PBidirectionalStream = class {
    #readable;
    #writable;
    constructor(readable, writable) {
      if (!(readable instanceof LP2PReceiveStream) || !(writable instanceof LP2PSendStream)) {
        throw new TypeError("Illegal constructor");
      }
      this.#readable = readable;
      this.#writable = writable;
    }
    get readable() {
      return this.#readable;
    }
    get writable() {
      return this.#writable;
    }
  };
  var DEFAULT_MAX_BUFFERED_DATAGRAMS = 1024;
  var LP2PDatagramsWritable = class extends WritableStream {
    #sendGroup;
    #sendOrder;
    constructor(sink, options) {
      super(sink, { highWaterMark: 1 });
      this.#sendGroup = options.sendGroup ?? null;
      this.#sendOrder = options.sendOrder ?? 0;
    }
    get sendGroup() {
      return this.#sendGroup;
    }
    set sendGroup(g) {
      this.#sendGroup = g;
    }
    get sendOrder() {
      return this.#sendOrder;
    }
    set sendOrder(v) {
      this.#sendOrder = Math.trunc(Number(v)) || 0;
    }
  };
  function maxAge(v) {
    if (v === null) return null;
    const n = Number(v);
    if (Number.isNaN(n) || n < 0) throw new RangeError("max age must be a non-negative number or null");
    return n === 0 ? null : n;
  }
  var LP2PDatagramDuplexStream = class {
    #transport;
    #readable;
    #controller = null;
    #writable = null;
    #incomingMaxAge = null;
    #outgoingMaxAge = null;
    #incomingMaxBuffered = DEFAULT_MAX_BUFFERED_DATAGRAMS;
    #outgoingMaxBuffered = DEFAULT_MAX_BUFFERED_DATAGRAMS;
    #done = false;
    droppedIncoming = 0;
    constructor(transport, readableType) {
      if (!(transport instanceof LP2PQuicTransport)) throw new TypeError("Illegal constructor");
      this.#transport = transport;
      const start = (c) => {
        this.#controller = c;
      };
      this.#readable = readableType === "bytes" ? new ReadableStream({ type: "bytes", start }) : new ReadableStream({ start }, { highWaterMark: 0 });
    }
    get readable() {
      return this.#readable;
    }
    /** Legacy (still shipped by Chromium): one shared writable. */
    get writable() {
      return this.#writable ??= this.createWritable();
    }
    createWritable(options = {}) {
      const t = this.#transport;
      return new LP2PDatagramsWritable(
        {
          write: async (chunk) => {
            const bytes2 = toBytes(chunk);
            await internalsOf(t).ready;
            const ws = internalsOf(t).session;
            if (!ws || ws.state !== "open") throw new DOMException("transport is closed", "InvalidStateError");
            if (bytes2.length > ws.maxDatagramSize) return;
            ws.sendDatagram(bytes2);
          }
        },
        options
      );
    }
    /** 0 until the transport is ready, then what fits one datagram of the underlying connection. */
    get maxDatagramSize() {
      return internalsOf(this.#transport).session?.maxDatagramSize ?? 0;
    }
    /** Stored; the shim does not expire datagrams. */
    get incomingMaxAge() {
      return this.#incomingMaxAge;
    }
    set incomingMaxAge(v) {
      this.#incomingMaxAge = maxAge(v);
    }
    get outgoingMaxAge() {
      return this.#outgoingMaxAge;
    }
    set outgoingMaxAge(v) {
      this.#outgoingMaxAge = maxAge(v);
    }
    get incomingMaxBufferedDatagrams() {
      return this.#incomingMaxBuffered;
    }
    set incomingMaxBufferedDatagrams(v) {
      this.#incomingMaxBuffered = Math.max(0, Math.trunc(Number(v)) || 0);
    }
    get outgoingMaxBufferedDatagrams() {
      return this.#outgoingMaxBuffered;
    }
    set outgoingMaxBufferedDatagrams(v) {
      this.#outgoingMaxBuffered = Math.max(0, Math.trunc(Number(v)) || 0);
    }
    /** Legacy names. */
    get incomingHighWaterMark() {
      return this.incomingMaxBufferedDatagrams;
    }
    set incomingHighWaterMark(v) {
      this.incomingMaxBufferedDatagrams = v;
    }
    get outgoingHighWaterMark() {
      return this.outgoingMaxBufferedDatagrams;
    }
    set outgoingHighWaterMark(v) {
      this.outgoingMaxBufferedDatagrams = v;
    }
    /** Internal: a datagram arrived. */
    receive(d) {
      const c = this.#controller;
      if (this.#done || !c) return;
      if (-(c.desiredSize ?? 0) >= this.#incomingMaxBuffered) {
        this.droppedIncoming++;
        return;
      }
      c.enqueue(d.slice());
    }
    /** Internal: the session ended. */
    finish(err) {
      if (this.#done || !this.#controller) return;
      this.#done = true;
      if (err === null) this.#controller.close();
      else this.#controller.error(err);
    }
  };
  var transportInternals = /* @__PURE__ */ new WeakMap();
  function internalsOf(t) {
    return transportInternals.get(t);
  }
  function enforceUnsignedShortOrNull(v, what) {
    if (v === null || v === void 0) return null;
    const n = Math.trunc(Number(v));
    if (!Number.isFinite(Number(v)) || n < 0 || n > 65535) throw new TypeError(`${what} is outside the unsigned short range`);
    return n;
  }
  function bufferSource(v, what) {
    if (v instanceof ArrayBuffer) return new Uint8Array(v);
    if (ArrayBuffer.isView(v)) return new Uint8Array(v.buffer, v.byteOffset, v.byteLength);
    throw new TypeError(`${what} must be a BufferSource`);
  }
  var CONGESTION_CONTROL = ["default", "throughput", "low-latency"];
  var MAX_REASON_BYTES = 1024;
  var EXPORTER_LENGTH = 32;
  var adopting3 = null;
  function adoptTransport(session, init, connection) {
    adopting3 = { session, init, connection };
    try {
      return new LP2PQuicTransport(connection, init);
    } finally {
      adopting3 = null;
    }
  }
  var LP2PQuicTransport = class {
    #state = "connecting";
    #session = null;
    #slots;
    #connection;
    #reliability = "pending";
    #congestionControl;
    #requireUnreliable;
    #anticipatedUni;
    #anticipatedBidi;
    #ready;
    #resolveReady;
    #rejectReady;
    #closed;
    #resolveClosed;
    #rejectClosed;
    #draining;
    #resolveDraining;
    #datagrams;
    #incomingBidiController;
    #incomingUniController;
    #incomingBidi;
    #incomingUni;
    #member = { failFromConnection: () => this.#fail(sessionError("the LP2P connection closed")) };
    /** Always true: some shim transports only give reliable-only sessions. */
    static get supportsReliableOnly() {
      return true;
    }
    /**
     * Opens a new WebTransport session on connection. Each construction opens
     * its own session; the peer receives it through its
     * LP2PQuicTransportListener on the connection.
     */
    constructor(connection, options = {}) {
      const a = adopting3;
      adopting3 = null;
      if (!a) {
        if (arguments.length < 1) throw new TypeError("LP2PQuicTransport: connection is required");
        if (!(connection instanceof LP2PConnection)) throw new TypeError("LP2PQuicTransport: connection must be an LP2PConnection");
      }
      const slots2 = slotsOf(connection);
      if (!a && slots2.closed) throw new DOMException("the LP2PConnection is closed", "InvalidStateError");
      this.#slots = slots2;
      this.#connection = connection;
      const init = options ?? {};
      const cc = init.congestionControl ?? "default";
      if (!CONGESTION_CONTROL.includes(cc)) throw new TypeError(`invalid congestionControl "${String(cc)}"`);
      if (init.datagramsReadableType !== void 0 && init.datagramsReadableType !== "bytes") {
        throw new TypeError(`invalid datagramsReadableType "${String(init.datagramsReadableType)}"`);
      }
      this.#congestionControl = cc;
      this.#requireUnreliable = Boolean(init.requireUnreliable);
      this.#anticipatedUni = enforceUnsignedShortOrNull(
        init.anticipatedConcurrentIncomingUnidirectionalStreams,
        "anticipatedConcurrentIncomingUnidirectionalStreams"
      );
      this.#anticipatedBidi = enforceUnsignedShortOrNull(
        init.anticipatedConcurrentIncomingBidirectionalStreams,
        "anticipatedConcurrentIncomingBidirectionalStreams"
      );
      this.#ready = handled(
        new Promise((resolve, reject) => {
          this.#resolveReady = () => resolve(void 0);
          this.#rejectReady = reject;
        })
      );
      this.#closed = handled(
        new Promise((resolve, reject) => {
          this.#resolveClosed = resolve;
          this.#rejectClosed = reject;
        })
      );
      this.#draining = new Promise((resolve) => {
        this.#resolveDraining = () => resolve(void 0);
      });
      this.#datagrams = new LP2PDatagramDuplexStream(this, init.datagramsReadableType);
      this.#incomingBidi = new ReadableStream(
        { start: (c) => void (this.#incomingBidiController = c) },
        { highWaterMark: 0 }
      );
      this.#incomingUni = new ReadableStream(
        { start: (c) => void (this.#incomingUniController = c) },
        { highWaterMark: 0 }
      );
      transportInternals.set(this, { session: null, ready: this.#ready });
      slots2.transports.add(this.#member);
      if (a) {
        this.#attach(a.session);
        return;
      }
      const session = slots2.session;
      if (!session.remoteInfo?.capabilities.includes(AgentCapability.QuicTransport)) {
        queueMicrotask(() => this.#fail(sessionError("the peer does not support quic-transport")));
        return;
      }
      let out;
      try {
        out = session.openTransport();
      } catch (err) {
        queueMicrotask(() => this.#fail(sessionError(String(err))));
        return;
      }
      this.#session = out;
      out.onready = () => this.#attach(out);
      out.onclose = (_info, why) => this.#fail(sessionError(why));
    }
    #attach(ws) {
      if (this.#state !== "connecting") {
        ws.onclose = null;
        ws.close();
        return;
      }
      this.#session = ws;
      internalsOf(this).session = ws;
      const reliability = ws.datagramsReliable || ws.maxDatagramSize === 0 ? "reliable-only" : "supports-unreliable";
      if (this.#requireUnreliable && reliability !== "supports-unreliable") {
        this.#fail(new WebTransportError("requireUnreliable: this session is reliable-only", { source: "session" }));
        return;
      }
      this.#reliability = reliability;
      this.#state = "connected";
      ws.onclose = (info, why) => this.#cleanup(info, why);
      ws.ondrain = () => this.#resolveDraining();
      ws.onstream = (s) => this.#incoming(s);
      ws.ondatagram = (d) => this.#datagrams.receive(d);
      this.#resolveReady();
    }
    #incoming(s) {
      const halves = new StreamHalves(s);
      const readable = new LP2PReceiveStream(halves);
      if (s.unidirectional) {
        s.attach(halves);
        this.#incomingUniController.enqueue(readable);
        return;
      }
      const writable = new LP2PSendStream(halves, this, {});
      s.attach(halves);
      this.#incomingBidiController.enqueue(new LP2PBidirectionalStream(readable, writable));
    }
    #fail(err) {
      if (this.#state === "closed" || this.#state === "failed") return;
      const ws = this.#session;
      this.#state = "failed";
      this.#reliability = "pending";
      this.#slots.transports.delete(this.#member);
      if (ws) {
        ws.onclose = null;
        ws.close();
      }
      this.#rejectReady(err);
      this.#rejectClosed(err);
      this.#incomingBidiController.error(err);
      this.#incomingUniController.error(err);
      this.#datagrams.finish(err);
      this.#resolveDraining();
    }
    #cleanup(info, why) {
      if (this.#state === "closed" || this.#state === "failed") return;
      if (info === null) {
        this.#fail(sessionError(why));
        return;
      }
      this.#state = "closed";
      this.#slots.transports.delete(this.#member);
      this.#resolveClosed({ closeCode: info.code, reason: info.reason });
      this.#incomingBidiController.close();
      this.#incomingUniController.close();
      this.#datagrams.finish(null);
      this.#resolveDraining();
    }
    /** [[Connection]]: the LP2PConnection this transport's session is on. */
    get connection() {
      return this.#connection;
    }
    get ready() {
      return this.#ready;
    }
    get closed() {
      return this.#closed;
    }
    /** Resolves when the peer drains the session (data-transport-drain), or when it ends. */
    get draining() {
      return this.#draining;
    }
    get reliability() {
      return this.#reliability;
    }
    get congestionControl() {
      return this.#congestionControl;
    }
    get anticipatedConcurrentIncomingUnidirectionalStreams() {
      return this.#anticipatedUni;
    }
    set anticipatedConcurrentIncomingUnidirectionalStreams(v) {
      this.#anticipatedUni = enforceUnsignedShortOrNull(v, "anticipatedConcurrentIncomingUnidirectionalStreams");
    }
    get anticipatedConcurrentIncomingBidirectionalStreams() {
      return this.#anticipatedBidi;
    }
    set anticipatedConcurrentIncomingBidirectionalStreams(v) {
      this.#anticipatedBidi = enforceUnsignedShortOrNull(v, "anticipatedConcurrentIncomingBidirectionalStreams");
    }
    /** Always null: there is no HTTP exchange. */
    get responseHeaders() {
      return null;
    }
    /** Always "": there is no subprotocol negotiation. */
    get protocol() {
      return "";
    }
    get datagrams() {
      return this.#datagrams;
    }
    get incomingBidirectionalStreams() {
      return this.#incomingBidi;
    }
    get incomingUnidirectionalStreams() {
      return this.#incomingUni;
    }
    async #openStream(unidirectional) {
      if (this.#state === "connecting") await this.#ready;
      const ws = this.#session;
      if (this.#state !== "connected" || !ws) throw new DOMException("transport is closed", "InvalidStateError");
      return new StreamHalves(ws.openStream(unidirectional));
    }
    async createBidirectionalStream(options = {}) {
      const halves = await this.#openStream(false);
      const readable = new LP2PReceiveStream(halves);
      const writable = new LP2PSendStream(halves, this, options ?? {});
      halves.ws.attach(halves);
      return new LP2PBidirectionalStream(readable, writable);
    }
    async createUnidirectionalStream(options = {}) {
      const halves = await this.#openStream(true);
      const writable = new LP2PSendStream(halves, this, options ?? {});
      halves.ws.attach(halves);
      return writable;
    }
    createSendGroup() {
      groupOwner = this;
      return new LP2PSendGroup();
    }
    /** Counts payload bytes; RTT and packet counts are not observable through the shim's transports. */
    getStats() {
      const t = this.#session?.totals() ?? { sent: 0, received: 0 };
      return Promise.resolve({
        bytesSent: t.sent,
        bytesReceived: t.received,
        datagrams: { droppedIncoming: this.#datagrams.droppedIncoming, expiredIncoming: 0, expiredOutgoing: 0, lostOutgoing: 0 },
        estimatedSendRate: null,
        atSendCapacity: false
      });
    }
    /**
     * Keying material of this session only, derived as the binding says (the
     * connection's exporter with label "EXPORTER-WebTransport" and the
     * WebTransport Exporter Context, with the exchange-id as session ID). In
     * the shim the connection's exporter is the handshake stand-in of
     * protocol/identity.ts, not TLS. Extension: outputLength, default 32.
     */
    async exportKeyingMaterial(label, context, outputLength = EXPORTER_LENGTH) {
      const l = bufferSource(label, "label");
      const c = context === void 0 ? new Uint8Array(0) : bufferSource(context, "context");
      if (l.length > 255 || c.length > 255) throw new TypeError("label and context are at most 255 bytes");
      const n = Math.trunc(Number(outputLength));
      if (!(n > 0 && n <= 255 * 32)) throw new RangeError("outputLength out of range");
      if (this.#state === "connecting") await this.#ready;
      const ws = this.#session;
      if (this.#state !== "connected" || !ws) throw new DOMException("transport is closed", "InvalidStateError");
      const out = await ws.exportKeyingMaterial(l, c, n);
      return out.buffer.slice(out.byteOffset, out.byteOffset + out.byteLength);
    }
    close(closeInfo = {}) {
      if (this.#state === "closed" || this.#state === "failed") return;
      if (this.#state === "connecting") {
        this.#fail(new WebTransportError("closed before the session was established", { source: "session" }));
        return;
      }
      const code = Math.min(Math.max(Math.trunc(Number(closeInfo?.closeCode ?? 0)) || 0, 0), 4294967295);
      let reason = String(closeInfo?.reason ?? "");
      const enc = new TextEncoder();
      while (enc.encode(reason).length > MAX_REASON_BYTES) reason = [...reason].slice(0, -1).join("");
      this.#session?.close(code, reason);
      return;
    }
  };
  var LISTENER_HIGH_WATER_MARK = 8;
  var LP2PQuicTransportListener = class {
    #incoming;
    #controller;
    #closed = false;
    #streamDone = false;
    #slots;
    /**
     * Receives the incoming WebTransport sessions of connection. There is at
     * most one listener per connection at a time; with none, or with a full
     * queue, incoming sessions are refused.
     */
    constructor(connection, options = {}) {
      if (arguments.length < 1) throw new TypeError("LP2PQuicTransportListener: connection is required");
      if (!(connection instanceof LP2PConnection)) {
        throw new TypeError("LP2PQuicTransportListener: connection must be an LP2PConnection");
      }
      const slots2 = slotsOf(connection);
      if (slots2.closed) throw new DOMException("the LP2PConnection is closed", "InvalidStateError");
      if (slots2.listener) throw new DOMException("the LP2PConnection already has a listener", "InvalidStateError");
      const init = { ...options ?? {} };
      if (init.congestionControl !== void 0 && !CONGESTION_CONTROL.includes(init.congestionControl)) {
        throw new TypeError(`invalid congestionControl "${String(init.congestionControl)}"`);
      }
      this.#slots = slots2;
      this.#incoming = new ReadableStream(
        {
          start: (c) => void (this.#controller = c),
          cancel: () => {
            this.#streamDone = true;
            this.close();
          }
        },
        { highWaterMark: LISTENER_HIGH_WATER_MARK }
      );
      slots2.listener = {
        closeFromConnection: () => this.close(),
        incoming: (t) => {
          if (this.#closed || (this.#controller.desiredSize ?? 0) <= 0) return false;
          if (init.requireUnreliable && (t.datagramsReliable || t.maxDatagramSize === 0)) return false;
          t.accept();
          this.#controller.enqueue(adoptTransport(t, init, connection));
          return true;
        }
      };
    }
    get incomingTransports() {
      return this.#incoming;
    }
    /** Stops listening. Transports already queued can still be read; a new listener can then be constructed. */
    close() {
      if (this.#closed) return;
      this.#closed = true;
      this.#slots.listener = null;
      if (!this.#streamDone) {
        this.#streamDone = true;
        try {
          this.#controller.close();
        } catch {
        }
      }
      return;
    }
  };

  // src/api/shorthand.ts
  function receiverOptions(o) {
    const r = {};
    if (o.nickname !== void 0) r.nickname = o.nickname;
    if (o.allowedOrigins !== void 0) r.allowedOrigins = o.allowedOrigins;
    return r;
  }
  function transportOptions(o) {
    const { requireUnreliable, congestionControl, datagramsReadableType } = o;
    const t = {};
    if (requireUnreliable !== void 0) t.requireUnreliable = requireUnreliable;
    if (congestionControl !== void 0) t.congestionControl = congestionControl;
    if (datagramsReadableType !== void 0) t.datagramsReadableType = datagramsReadableType;
    if (o.anticipatedConcurrentIncomingBidirectionalStreams !== void 0) {
      t.anticipatedConcurrentIncomingBidirectionalStreams = o.anticipatedConcurrentIncomingBidirectionalStreams;
    }
    if (o.anticipatedConcurrentIncomingUnidirectionalStreams !== void 0) {
      t.anticipatedConcurrentIncomingUnidirectionalStreams = o.anticipatedConcurrentIncomingUnidirectionalStreams;
    }
    return t;
  }
  var MERGED_HIGH_WATER_MARK = 8;
  function makeDiscoverable(options = {}) {
    const opts = options ?? {};
    const receiver = new LP2PReceiver(receiverOptions(opts));
    const listenerInit = transportOptions(opts);
    const listeners = /* @__PURE__ */ new Set();
    let controller;
    let done = false;
    let wake = null;
    const pulled = () => new Promise((r) => wake = r);
    const merged = new ReadableStream(
      {
        start: (c) => void (controller = c),
        pull: () => {
          wake?.();
          wake = null;
        },
        cancel: () => {
          done = true;
          receiver.stop();
          for (const l of listeners) l.close();
          wake?.();
        }
      },
      { highWaterMark: MERGED_HIGH_WATER_MARK }
    );
    const pump = async (listener) => {
      const reader = listener.incomingTransports.getReader();
      for (; ; ) {
        while (!done && (controller.desiredSize ?? 0) <= 0) await pulled();
        if (done) break;
        const { value, done: end } = await reader.read();
        if (end) break;
        if (done) {
          value.close();
          break;
        }
        controller.enqueue(value);
      }
      reader.releaseLock();
    };
    receiver.addEventListener("connection", (e) => {
      const connection = e.connection;
      if (done) {
        connection.close();
        return;
      }
      const listener = new LP2PQuicTransportListener(connection, listenerInit);
      listeners.add(listener);
      connection.addEventListener("close", () => listeners.delete(listener));
      void pump(listener);
    });
    receiver.start().catch((err) => {
      if (done) return;
      done = true;
      controller.error(err);
    });
    return merged;
  }
  async function connect(options = {}) {
    const opts = options ?? {};
    const connection = await new LP2PRequest(opts).start();
    try {
      const transport = new LP2PQuicTransport(connection, transportOptions(opts));
      await transport.ready;
      return transport;
    } catch (err) {
      connection.close();
      throw err;
    }
  }
  var lp2p = Object.freeze({ makeDiscoverable, connect });

  // src/install.ts
  var interfaces = {
    LP2PReceiver,
    LP2PRequest,
    LP2PConnection,
    LP2PDataChannel,
    LP2PConnectionEvent,
    LP2PDataChannelEvent,
    LP2PQuicTransport,
    LP2PQuicTransportListener,
    // The explainer's shorthand namespace.
    lp2p
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
  var DATAGRAM_LABEL = "lp2p-shim/datagrams";
  var MAX_DATAGRAM_SIZE = 1200;
  function createDatagramChannel(pc) {
    const dc = pc.createDataChannel(DATAGRAM_LABEL, { negotiated: true, id: 1, ordered: false, maxRetransmits: 0 });
    dc.binaryType = "arraybuffer";
    return dc;
  }
  var DISCONNECT_GRACE_MS = 5e3;
  var STREAM_LABEL = "lp2p-shim/stream";
  var UNI_LABEL = "lp2p-shim/uni";
  var ChannelStream = class extends BaseStream {
    pending = [];
    dc;
    conn;
    constructor(id, dc, conn, received = []) {
      super(id, dc.label === UNI_LABEL);
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
    datagrams;
    constructor(remoteId, pc, control, datagrams, early) {
      super(remoteId);
      this.pc = pc;
      this.control = control;
      this.datagrams = datagrams;
      datagrams.onmessage = (e) => this.deliverDatagram(new Uint8Array(e.data));
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
    openStream(options = {}) {
      if (this.closed) throw new DOMException("connection is closed", "InvalidStateError");
      const label = options.unidirectional ? UNI_LABEL : STREAM_LABEL;
      const s = this.wrap(this.pc.createDataChannel(label, { ordered: true }));
      this.streams.set(s.id, s);
      return s;
    }
    get maxDatagramSize() {
      return MAX_DATAGRAM_SIZE;
    }
    doSendDatagram(d) {
      if (this.datagrams.readyState !== "open" || this.datagrams.bufferedAmount > 256 * 1024) return;
      this.datagrams.send(d);
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
          const datagrams = createDatagramChannel(pc);
          const early = collectEarlyChannels(pc);
          const reply = (d) => this.rendezvous.send({ t: "signal", from: ad.id, to: m.from, conn: connId, data: d });
          if (this.trickle) {
            pc.onicecandidate = (e) => e.candidate && reply({ candidate: e.candidate.toJSON() });
          }
          control.onopen = () => {
            sessions.delete(connId);
            onconnection(new RTCConnection(m.from, pc, control, datagrams, early));
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
      const datagrams = createDatagramChannel(pc);
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
          control.onopen = () => resolve(new RTCConnection(peerId, pc, control, datagrams, early));
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
  var version = "0.2.0";

  // src/polyfill.ts
  var installed = install();
  Object.defineProperty(globalThis, "LP2PShim", {
    value: Object.freeze({ ...index_exports, installed }),
    writable: true,
    configurable: true,
    enumerable: false
  });
})();
