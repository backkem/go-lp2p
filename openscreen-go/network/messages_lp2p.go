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
	ChannelId uint64 `codec:"1"`
	Label     string `codec:"2"`
	Protocol  string `codec:"3"`
}

// (cddlc) Ident: data-channel-open-response
type msgDataChannelOpenResponse struct {
	msgResponse
	Result msgResult `codec:"1"`
}
