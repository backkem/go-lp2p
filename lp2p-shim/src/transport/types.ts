// The transport seam.
//
// A Transport moves opaque messages between two pages. It knows nothing about
// LP2P: it discovers and advertises peers, establishes connections and gives
// each connection any number of reliable, ordered, bidirectional message
// streams - the same service QUIC streams give the Open Screen Protocol. The
// session layer (../protocol/session.ts) speaks OSP messages over these
// streams, and the API layer (../api) is built on the session layer only.
//
// A transport that bridges to a native OSP agent (for example a WebSocket to
// a local go-lp2p helper that maps each stream to a QUIC stream) implements
// this same interface; nothing above it needs to change.

/** What a listening peer makes discoverable. */
export interface Advertisement {
  /** Transport-level peer ID; unique per listener. */
  id: string;
  /** The page-provided nickname (LP2PReceiverOptions.nickname). Unverified. */
  nickname: string;
}

/** A peer seen by discovery. */
export interface DiscoveredPeer {
  id: string;
  nickname: string;
}

export interface DiscoveryEvent {
  kind: "added" | "updated" | "removed";
  peer: DiscoveredPeer;
}

/** Something that can be stopped: an advertisement or a discovery session. */
export interface Closer {
  close(): void;
}

export interface Transport {
  /** Short name for logs and UI, e.g. "postmessage" or "webrtc". */
  readonly name: string;

  /**
   * Starts advertising ad. Every incoming connection is passed to
   * onconnection; the transport does no policy, the caller decides whether
   * to keep or close it.
   */
  listen(ad: Advertisement, onconnection: (connection: TransportConnection) => void): Closer;

  /** Reports peers as they appear, change and disappear, until closed. */
  discover(onevent: (event: DiscoveryEvent) => void): Closer;

  /**
   * Connects to the discovered peer peerId, identifying as local. Resolves
   * once the connection can carry streams.
   */
  connect(peerId: string, local: Advertisement, options?: { signal?: AbortSignal }): Promise<TransportConnection>;
}

export interface TransportConnection {
  /** The remote peer's transport-level ID. */
  readonly remoteId: string;
  /** True once the connection is closed, locally or by the peer. */
  readonly closed: boolean;

  /**
   * Opens a stream. It is usable at once; messages sent before the peer has
   * seen the stream are queued.
   */
  openStream(): TransportStream;

  /**
   * Called for every stream the peer opens. Streams that arrive while this
   * is null are held until it is set.
   */
  onstream: ((stream: TransportStream) => void) | null;

  /** Called once when the connection closes, for whatever reason. */
  onclose: ((reason: string) => void) | null;

  /** Closes the connection and all its streams. */
  close(reason?: string): void;
}

export interface TransportStream {
  /** Unique within its connection. */
  readonly id: number;
  readonly closed: boolean;

  /** Sends one message. Messages arrive whole, reliably and in order. */
  send(message: Uint8Array): void;

  /** Called for each message. Messages that arrive while this is null are held. */
  onmessage: ((message: Uint8Array) => void) | null;

  /**
   * Called once when the stream closes: after the peer closed it (and every
   * message it sent before that was delivered), after close(), or when the
   * connection closed.
   */
  onclose: (() => void) | null;

  /** Closes the stream in both directions. */
  close(): void;
}
