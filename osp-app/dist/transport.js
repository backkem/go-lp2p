// The transport interface: what the OSP application protocol needs from
// whatever carries it. OSP's network protocol (network.bs) provides this with
// QUIC; here it is pluggable.
//
// A transport moves framed messages (type key + one CBOR item) between two
// authenticated agents, keeps the order of messages within an ordering group
// and reports how it closed. Authentication, discovery and agent identity are
// the transport's job, not this package's.
/** Thrown by send() once the transport is closed. */
export class TransportClosedError extends Error {
    name = "TransportClosedError";
}
