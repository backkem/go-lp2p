package ospc

type TypeKey uint64

// Message structs are defined in:
// * messages_osp.go: Generated based on OSP CDDL
// * messages_lp2p.go: Generated based on lp2p CDDL
// * messages_wip.go: handwritten messages yet to be defined in CDDL

////
// The below is generated semi-automatically
////

const (
	// structure:
	// typeKeyFoo    TypeKey = 123
	typeKeyAgentInfoRequest                    TypeKey = 10
	typeKeyAgentInfoResponse                   TypeKey = 11
	typeKeyAgentStatusRequest                  TypeKey = 12
	typeKeyAgentStatusResponse                 TypeKey = 13
	typeKeyPresentationUrlAvailabilityRequest  TypeKey = 14
	typeKeyPresentationUrlAvailabilityResponse TypeKey = 15
	typeKeyPresentationConnectionMessage       TypeKey = 16
	typeKeyRemotePlaybackAvailabilityRequest   TypeKey = 17
	typeKeyRemotePlaybackAvailabilityResponse  TypeKey = 18
	typeKeyRemotePlaybackModifyRequest         TypeKey = 19
	typeKeyRemotePlaybackModifyResponse        TypeKey = 20
	typeKeyRemotePlaybackStateEvent            TypeKey = 21
	typeKeyAudioFrame                          TypeKey = 22
	typeKeyVideoFrame                          TypeKey = 23
	typeKeyDataFrame                           TypeKey = 24
	typeKeyPresentationUrlAvailabilityEvent    TypeKey = 103
	typeKeyPresentationStartRequest            TypeKey = 104
	typeKeyPresentationStartResponse           TypeKey = 105
	typeKeyPresentationTerminationRequest      TypeKey = 106
	typeKeyPresentationTerminationResponse     TypeKey = 107
	typeKeyPresentationTerminationEvent        TypeKey = 108
	typeKeyPresentationConnectionOpenRequest   TypeKey = 109
	typeKeyPresentationConnectionOpenResponse  TypeKey = 110
	typeKeyPresentationConnectionCloseEvent    TypeKey = 113
	typeKeyRemotePlaybackAvailabilityEvent     TypeKey = 114
	typeKeyRemotePlaybackStartRequest          TypeKey = 115
	typeKeyRemotePlaybackStartResponse         TypeKey = 116
	typeKeyRemotePlaybackTerminationRequest    TypeKey = 117
	typeKeyRemotePlaybackTerminationResponse   TypeKey = 118
	typeKeyRemotePlaybackTerminationEvent      TypeKey = 119
	typeKeyAgentInfoEvent                      TypeKey = 120
	typeKeyPresentationChangeEvent             TypeKey = 121
	typeKeyStreamingCapabilitiesRequest        TypeKey = 122
	typeKeyStreamingCapabilitiesResponse       TypeKey = 123
	typeKeyStreamingSessionStartRequest        TypeKey = 124
	typeKeyStreamingSessionStartResponse       TypeKey = 125
	typeKeyStreamingSessionModifyRequest       TypeKey = 126
	typeKeyStreamingSessionModifyResponse      TypeKey = 127
	typeKeyStreamingSessionTerminateRequest    TypeKey = 128
	typeKeyStreamingSessionTerminateResponse   TypeKey = 129
	typeKeyStreamingSessionTerminateEvent      TypeKey = 130
	typeKeyStreamingSessionSenderStatsEvent    TypeKey = 131
	typeKeyStreamingSessionReceiverStatsEvent  TypeKey = 132
	typeKeyAuthCapabilities                    TypeKey = 1001
	typeKeyAuthSpake2Confirmation              TypeKey = 1003
	typeKeyAuthStatus                          TypeKey = 1004
	typeKeyAuthSpake2Handshake                 TypeKey = 1005
	typeKeyDataChannelOpenRequest              TypeKey = 1101
	typeKeyDataChannelOpenResponse             TypeKey = 1102
)

func newMessageByType(key TypeKey) (interface{}, error) {
	// Case structure:
	// case typeKeyFoo:	return &msgFoo{}, nil
	switch key {
	case typeKeyAgentInfoRequest:
		return &msgAgentInfoRequest{}, nil
	case typeKeyAgentInfoResponse:
		return &msgAgentInfoResponse{}, nil
	case typeKeyAgentStatusRequest:
		return &msgAgentStatusRequest{}, nil
	case typeKeyAgentStatusResponse:
		return &msgAgentStatusResponse{}, nil
	case typeKeyPresentationUrlAvailabilityRequest:
		return &msgPresentationUrlAvailabilityRequest{}, nil
	case typeKeyPresentationUrlAvailabilityResponse:
		return &msgPresentationUrlAvailabilityResponse{}, nil
	case typeKeyPresentationConnectionMessage:
		return &msgPresentationConnectionMessage{}, nil
	case typeKeyRemotePlaybackAvailabilityRequest:
		return &msgRemotePlaybackAvailabilityRequest{}, nil
	case typeKeyRemotePlaybackAvailabilityResponse:
		return &msgRemotePlaybackAvailabilityResponse{}, nil
	case typeKeyRemotePlaybackModifyRequest:
		return &msgRemotePlaybackModifyRequest{}, nil
	case typeKeyRemotePlaybackModifyResponse:
		return &msgRemotePlaybackModifyResponse{}, nil
	case typeKeyRemotePlaybackStateEvent:
		return &msgRemotePlaybackStateEvent{}, nil
	case typeKeyAudioFrame:
		return &msgAudioFrame{}, nil
	case typeKeyVideoFrame:
		return &msgVideoFrame{}, nil
	case typeKeyDataFrame:
		return &msgDataFrame{}, nil
	case typeKeyPresentationUrlAvailabilityEvent:
		return &msgPresentationUrlAvailabilityEvent{}, nil
	case typeKeyPresentationStartRequest:
		return &msgPresentationStartRequest{}, nil
	case typeKeyPresentationStartResponse:
		return &msgPresentationStartResponse{}, nil
	case typeKeyPresentationTerminationRequest:
		return &msgPresentationTerminationRequest{}, nil
	case typeKeyPresentationTerminationResponse:
		return &msgPresentationTerminationResponse{}, nil
	case typeKeyPresentationTerminationEvent:
		return &msgPresentationTerminationEvent{}, nil
	case typeKeyPresentationConnectionOpenRequest:
		return &msgPresentationConnectionOpenRequest{}, nil
	case typeKeyPresentationConnectionOpenResponse:
		return &msgPresentationConnectionOpenResponse{}, nil
	case typeKeyPresentationConnectionCloseEvent:
		return &msgPresentationConnectionCloseEvent{}, nil
	case typeKeyRemotePlaybackAvailabilityEvent:
		return &msgRemotePlaybackAvailabilityEvent{}, nil
	case typeKeyRemotePlaybackStartRequest:
		return &msgRemotePlaybackStartRequest{}, nil
	case typeKeyRemotePlaybackStartResponse:
		return &msgRemotePlaybackStartResponse{}, nil
	case typeKeyRemotePlaybackTerminationRequest:
		return &msgRemotePlaybackTerminationRequest{}, nil
	case typeKeyRemotePlaybackTerminationResponse:
		return &msgRemotePlaybackTerminationResponse{}, nil
	case typeKeyRemotePlaybackTerminationEvent:
		return &msgRemotePlaybackTerminationEvent{}, nil
	case typeKeyAgentInfoEvent:
		return &msgAgentInfoEvent{}, nil
	case typeKeyPresentationChangeEvent:
		return &msgPresentationChangeEvent{}, nil
	case typeKeyStreamingCapabilitiesRequest:
		return &msgStreamingCapabilitiesRequest{}, nil
	case typeKeyStreamingCapabilitiesResponse:
		return &msgStreamingCapabilitiesResponse{}, nil
	case typeKeyStreamingSessionStartRequest:
		return &msgStreamingSessionStartRequest{}, nil
	case typeKeyStreamingSessionStartResponse:
		return &msgStreamingSessionStartResponse{}, nil
	case typeKeyStreamingSessionModifyRequest:
		return &msgStreamingSessionModifyRequest{}, nil
	case typeKeyStreamingSessionModifyResponse:
		return &msgStreamingSessionModifyResponse{}, nil
	case typeKeyStreamingSessionTerminateRequest:
		return &msgStreamingSessionTerminateRequest{}, nil
	case typeKeyStreamingSessionTerminateResponse:
		return &msgStreamingSessionTerminateResponse{}, nil
	case typeKeyStreamingSessionTerminateEvent:
		return &msgStreamingSessionTerminateEvent{}, nil
	case typeKeyStreamingSessionSenderStatsEvent:
		return &msgStreamingSessionSenderStatsEvent{}, nil
	case typeKeyStreamingSessionReceiverStatsEvent:
		return &msgStreamingSessionReceiverStatsEvent{}, nil
	case typeKeyAuthCapabilities:
		return &msgAuthCapabilities{}, nil
	case typeKeyAuthSpake2Confirmation:
		return &msgAuthSpake2Confirmation{}, nil
	case typeKeyAuthStatus:
		return &msgAuthStatus{}, nil
	case typeKeyAuthSpake2Handshake:
		return &msgAuthSpake2Handshake{}, nil
	case typeKeyDataChannelOpenRequest:
		return &msgDataChannelOpenRequest{}, nil
	case typeKeyDataChannelOpenResponse:
		return &msgDataChannelOpenResponse{}, nil

	default:
		return newMessageByTypeWIP(key)
		// return nil, fmt.Errorf("unknown type key: %d", key)
	}
}

func typeKeyByMessage(msg interface{}) (TypeKey, error) {
	// Case structure:
	// case *msgAgentInfoRequest:	return typeKeyAgentInfoRequest, nil
	switch msg.(type) {
	case *msgAgentInfoRequest:
		return typeKeyAgentInfoRequest, nil
	case *msgAgentInfoResponse:
		return typeKeyAgentInfoResponse, nil
	case *msgAgentStatusRequest:
		return typeKeyAgentStatusRequest, nil
	case *msgAgentStatusResponse:
		return typeKeyAgentStatusResponse, nil
	case *msgPresentationUrlAvailabilityRequest:
		return typeKeyPresentationUrlAvailabilityRequest, nil
	case *msgPresentationUrlAvailabilityResponse:
		return typeKeyPresentationUrlAvailabilityResponse, nil
	case *msgPresentationConnectionMessage:
		return typeKeyPresentationConnectionMessage, nil
	case *msgRemotePlaybackAvailabilityRequest:
		return typeKeyRemotePlaybackAvailabilityRequest, nil
	case *msgRemotePlaybackAvailabilityResponse:
		return typeKeyRemotePlaybackAvailabilityResponse, nil
	case *msgRemotePlaybackModifyRequest:
		return typeKeyRemotePlaybackModifyRequest, nil
	case *msgRemotePlaybackModifyResponse:
		return typeKeyRemotePlaybackModifyResponse, nil
	case *msgRemotePlaybackStateEvent:
		return typeKeyRemotePlaybackStateEvent, nil
	case *msgAudioFrame:
		return typeKeyAudioFrame, nil
	case *msgVideoFrame:
		return typeKeyVideoFrame, nil
	case *msgDataFrame:
		return typeKeyDataFrame, nil
	case *msgPresentationUrlAvailabilityEvent:
		return typeKeyPresentationUrlAvailabilityEvent, nil
	case *msgPresentationStartRequest:
		return typeKeyPresentationStartRequest, nil
	case *msgPresentationStartResponse:
		return typeKeyPresentationStartResponse, nil
	case *msgPresentationTerminationRequest:
		return typeKeyPresentationTerminationRequest, nil
	case *msgPresentationTerminationResponse:
		return typeKeyPresentationTerminationResponse, nil
	case *msgPresentationTerminationEvent:
		return typeKeyPresentationTerminationEvent, nil
	case *msgPresentationConnectionOpenRequest:
		return typeKeyPresentationConnectionOpenRequest, nil
	case *msgPresentationConnectionOpenResponse:
		return typeKeyPresentationConnectionOpenResponse, nil
	case *msgPresentationConnectionCloseEvent:
		return typeKeyPresentationConnectionCloseEvent, nil
	case *msgRemotePlaybackAvailabilityEvent:
		return typeKeyRemotePlaybackAvailabilityEvent, nil
	case *msgRemotePlaybackStartRequest:
		return typeKeyRemotePlaybackStartRequest, nil
	case *msgRemotePlaybackStartResponse:
		return typeKeyRemotePlaybackStartResponse, nil
	case *msgRemotePlaybackTerminationRequest:
		return typeKeyRemotePlaybackTerminationRequest, nil
	case *msgRemotePlaybackTerminationResponse:
		return typeKeyRemotePlaybackTerminationResponse, nil
	case *msgRemotePlaybackTerminationEvent:
		return typeKeyRemotePlaybackTerminationEvent, nil
	case *msgAgentInfoEvent:
		return typeKeyAgentInfoEvent, nil
	case *msgPresentationChangeEvent:
		return typeKeyPresentationChangeEvent, nil
	case *msgStreamingCapabilitiesRequest:
		return typeKeyStreamingCapabilitiesRequest, nil
	case *msgStreamingCapabilitiesResponse:
		return typeKeyStreamingCapabilitiesResponse, nil
	case *msgStreamingSessionStartRequest:
		return typeKeyStreamingSessionStartRequest, nil
	case *msgStreamingSessionStartResponse:
		return typeKeyStreamingSessionStartResponse, nil
	case *msgStreamingSessionModifyRequest:
		return typeKeyStreamingSessionModifyRequest, nil
	case *msgStreamingSessionModifyResponse:
		return typeKeyStreamingSessionModifyResponse, nil
	case *msgStreamingSessionTerminateRequest:
		return typeKeyStreamingSessionTerminateRequest, nil
	case *msgStreamingSessionTerminateResponse:
		return typeKeyStreamingSessionTerminateResponse, nil
	case *msgStreamingSessionTerminateEvent:
		return typeKeyStreamingSessionTerminateEvent, nil
	case *msgStreamingSessionSenderStatsEvent:
		return typeKeyStreamingSessionSenderStatsEvent, nil
	case *msgStreamingSessionReceiverStatsEvent:
		return typeKeyStreamingSessionReceiverStatsEvent, nil
	case *msgAuthCapabilities:
		return typeKeyAuthCapabilities, nil
	case *msgAuthSpake2Confirmation:
		return typeKeyAuthSpake2Confirmation, nil
	case *msgAuthStatus:
		return typeKeyAuthStatus, nil
	case *msgAuthSpake2Handshake:
		return typeKeyAuthSpake2Handshake, nil
	case *msgDataChannelOpenRequest:
		return typeKeyDataChannelOpenRequest, nil
	case *msgDataChannelOpenResponse:
		return typeKeyDataChannelOpenResponse, nil

	default:
		return typeKeyByMessageWIP(msg)
		// return 0, fmt.Errorf("unknown message type: %T", msg)
	}
}
