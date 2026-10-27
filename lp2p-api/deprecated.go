package lp2p

// Names from before the API was aligned with the WebIDL, kept so existing
// code keeps compiling.

// Deprecated: use LP2PReceiverOptions.
type LP2PReceiverConfig = LP2PReceiverOptions

// Deprecated: use LP2PRequestOptions.
type LP2PRequestConfig = LP2PRequestOptions

// Deprecated: use LP2PDataChannel.
type DataChannel = LP2PDataChannel

// Deprecated: use LP2PDataChannelInit.
type DataChannelInit = LP2PDataChannelInit

// Deprecated: use LP2PConnectionEvent.
type OnConnectionEvent = LP2PConnectionEvent

// Deprecated: use LP2PDataChannelEvent.
type OnDataChannelEvent = LP2PDataChannelEvent

// Deprecated: use LP2PQuicTransportInit.
type LP2PWebTransportOptions = LP2PQuicTransportInit

// Deprecated: use NewLP2PReceiver; the old name had a typo.
func NewLP2Receiver(config LP2PReceiverConfig) (*LP2PReceiver, error) {
	return NewLP2PReceiver(config)
}

// Deprecated: use NewLP2PQuicTransport(c, init).
func (c *LP2PConnection) NewLP2PQuicTransport(init LP2PQuicTransportInit) (*LP2PQuicTransport, error) {
	return NewLP2PQuicTransport(c, init)
}

// Close stops the receiver.
//
// Deprecated: use Stop, the proposal's name.
func (r *LP2PReceiver) Close() error {
	r.Stop()
	return nil
}
