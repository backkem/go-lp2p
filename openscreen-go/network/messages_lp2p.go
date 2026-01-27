/*
  File generated using `cddlc.exe generate messages_lp2p.cddl`. DO NOT EDIT
*/

package ospc

const (
	AgentCapabilityDataChannels   msgAgentCapability = 1100
	AgentCapabilityQuickTransport msgAgentCapability = 1200
)

// (cddlc) Ident: msgDataChannelEncodingId
type msgDataChannelEncodingId uint64

const (
	DataChannelEncodingIdEncodingIdBlob        msgDataChannelEncodingId = 0
	DataChannelEncodingIdEncodingIdString      msgDataChannelEncodingId = 1
	DataChannelEncodingIdEncodingIdArrayBuffer msgDataChannelEncodingId = 2
)

// (cddlc) Ident: data-channel-open-request
type msgDataChannelOpenRequest struct {
	msgRequest
	ChannelId uint64 `cbor:"1,keyasint"`
	Label     string `cbor:"2,keyasint"`
	Protocol  string `cbor:"3,keyasint"`
}

// (cddlc) Ident: data-channel-open-response
type msgDataChannelOpenResponse struct {
	msgResponse
	Result msgResult `cbor:"1,keyasint"`
}
