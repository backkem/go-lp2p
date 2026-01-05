/*
  File generated using `cddlc.exe generate messages_lp2p.cddl`. DO NOT EDIT
*/

package ospc

// (cddlc) Ident: agent-info-request
type msgAgentInfoRequest struct {
	msgRequest
}

// (cddlc) Ident: agent-info-response
type msgAgentInfoResponse struct {
	msgResponse
	AgentInfo msgAgentInfo `codec:"1"`
}

// (cddlc) Ident: agent-info-event
type msgAgentInfoEvent struct {
	AgentInfo msgAgentInfo `codec:"0"`
}

// (cddlc) Ident: msgAgentCapability
type msgAgentCapability uint64

const (
	AgentCapabilityReceiveAudio          msgAgentCapability = 1
	AgentCapabilityReceiveVideo          msgAgentCapability = 2
	AgentCapabilityReceivePresentation   msgAgentCapability = 3
	AgentCapabilityControlPresentation   msgAgentCapability = 4
	AgentCapabilityReceiveRemotePlayback msgAgentCapability = 5
	AgentCapabilityControlRemotePlayback msgAgentCapability = 6
	AgentCapabilityReceiveStreaming      msgAgentCapability = 7
	AgentCapabilitySendStreaming         msgAgentCapability = 8
)

// (cddlc) Ident: agent-info
type msgAgentInfo struct {
	DisplayName  string               `codec:"0"`
	ModelName    string               `codec:"1"`
	Capabilities []msgAgentCapability `codec:"2"`
	StateToken   string               `codec:"3"`
	Locales      []string             `codec:"4"`
}

// (cddlc) Ident: agent-status-request
type msgAgentStatusRequest struct {
	msgRequest
	Status *msgStatus `codec:"1,omitempty"`
}

// (cddlc) Ident: agent-status-response
type msgAgentStatusResponse struct {
	msgResponse
	Status *msgStatus `codec:"1,omitempty"`
}

// (cddlc) Ident: status
type msgStatus struct {
	Status string `codec:"0"`
}

// (cddlc) Ident: request
type msgRequest struct {
	RequestId msgRequestId `codec:"0"`
}

// (cddlc) Ident: response
type msgResponse struct {
	RequestId msgRequestId `codec:"0"`
}

// (cddlc) Ident: request-id
type msgRequestId uint64

// (cddlc) Ident: microseconds
type msgMicroseconds uint64

// (cddlc) Ident: epoch-time
type msgEpochTime int64

// (cddlc) Ident: media-timeline
type msgMediaTimeline float64

// (cddlc) Ident: media-timeline-range
type msgMediaTimelineRange struct {
	Start msgMediaTimeline
	End   msgMediaTimeline
}

// (cddlc) Ident: auth-capabilities
type msgAuthCapabilities struct {
	PskEaseOfInput      uint64              `codec:"0"`
	PskInputMethods     []msgPskInputMethod `codec:"1"`
	PskMinBitsOfEntropy uint64              `codec:"2"`
}

// (cddlc) Ident: msgPskInputMethod
type msgPskInputMethod uint64

const (
	PskInputMethodNumeric msgPskInputMethod = 0
	PskInputMethodQrCode  msgPskInputMethod = 1
)

// (cddlc) Ident: auth-initiation-token
// TODO: w3c/openscreenprotocol#338
// type msgAuthInitiationToken struct {
// 	Token *string `codec:"0,omitempty"`
// }

// (cddlc) Ident: msgAuthSpake2PskStatus
type msgAuthSpake2PskStatus uint64

const (
	AuthSpake2PskStatusPskNeedsPresentation msgAuthSpake2PskStatus = 0
	AuthSpake2PskStatusPskShown             msgAuthSpake2PskStatus = 1
	AuthSpake2PskStatusPskInput             msgAuthSpake2PskStatus = 2
)

// (cddlc) Ident: auth-spake2-confirmation
type msgAuthSpake2Confirmation struct {
	ConfirmationValue []byte `codec:"0"`
}

// (cddlc) Ident: msgAuthStatusResult
type msgAuthStatusResult uint64

const (
	AuthStatusResultAuthenticated         msgAuthStatusResult = 0
	AuthStatusResultUnknownError          msgAuthStatusResult = 1
	AuthStatusResultTimeout               msgAuthStatusResult = 2
	AuthStatusResultSecretUnknown         msgAuthStatusResult = 3
	AuthStatusResultValidationTookTooLong msgAuthStatusResult = 4
	AuthStatusResultProofInvalid          msgAuthStatusResult = 5
)

// (cddlc) Ident: auth-status
type msgAuthStatus struct {
	Result msgAuthStatusResult `codec:"0"`
}

// (cddlc) Ident: auth-spake2-handshake
type msgAuthSpake2Handshake struct {
	PskStatus   msgAuthSpake2PskStatus `codec:"0"`
	PublicValue []byte                 `codec:"1"`
}

// (cddlc) Ident: watch-id
type msgWatchId uint64

// (cddlc) Ident: presentation-url-availability-request
type msgPresentationUrlAvailabilityRequest struct {
	msgRequest
	Urls          []string        `codec:"1"`
	WatchDuration msgMicroseconds `codec:"2"`
	WatchId       msgWatchId      `codec:"3"`
}

// (cddlc) Ident: presentation-url-availability-response
type msgPresentationUrlAvailabilityResponse struct {
	msgResponse
	UrlAvailabilities []msgUrlAvailability `codec:"1"`
}

// (cddlc) Ident: presentation-url-availability-event
type msgPresentationUrlAvailabilityEvent struct {
	WatchId           msgWatchId           `codec:"0"`
	UrlAvailabilities []msgUrlAvailability `codec:"1"`
}

// (cddlc) Ident: msgUrlAvailability
type msgUrlAvailability uint64

const (
	UrlAvailabilityAvailable   msgUrlAvailability = 0
	UrlAvailabilityUnavailable msgUrlAvailability = 1
	UrlAvailabilityInvalid     msgUrlAvailability = 10
)

// (cddlc) Ident: presentation-start-request
type msgPresentationStartRequest struct {
	msgRequest
	PresentationId string          `codec:"1"`
	Url            string          `codec:"2"`
	Headers        []msgHttpHeader `codec:"3"`
}

// (cddlc) Ident: http-header
type msgHttpHeader struct {
	Key   string
	Value string
}

// (cddlc) Ident: presentation-start-response
type msgPresentationStartResponse struct {
	msgResponse
	Result           msgResult `codec:"1"`
	ConnectionId     uint64    `codec:"2"`
	HttpResponseCode *uint64   `codec:"3,omitempty"`
}

// (cddlc) Ident: msgPresentationTerminationSource
type msgPresentationTerminationSource uint64

const (
	PresentationTerminationSourceController msgPresentationTerminationSource = 1
	PresentationTerminationSourceReceiver   msgPresentationTerminationSource = 2
	PresentationTerminationSourceUnknown    msgPresentationTerminationSource = 255
)

// (cddlc) Ident: msgPresentationTerminationReason
type msgPresentationTerminationReason uint64

const (
	PresentationTerminationReasonApplicationRequest           msgPresentationTerminationReason = 1
	PresentationTerminationReasonUserRequest                  msgPresentationTerminationReason = 2
	PresentationTerminationReasonReceiverReplacedPresentation msgPresentationTerminationReason = 20
	PresentationTerminationReasonReceiverIdleTooLong          msgPresentationTerminationReason = 30
	PresentationTerminationReasonReceiverAttemptedToNavigate  msgPresentationTerminationReason = 31
	PresentationTerminationReasonReceiverPoweringDown         msgPresentationTerminationReason = 100
	PresentationTerminationReasonReceiverError                msgPresentationTerminationReason = 101
	PresentationTerminationReasonUnknown                      msgPresentationTerminationReason = 255
)

// (cddlc) Ident: presentation-termination-request
type msgPresentationTerminationRequest struct {
	msgRequest
	PresentationId string                           `codec:"1"`
	Reason         msgPresentationTerminationReason `codec:"2"`
}

// (cddlc) Ident: presentation-termination-response
type msgPresentationTerminationResponse struct {
	msgResponse
	Result msgResult `codec:"1"`
}

// (cddlc) Ident: presentation-termination-event
type msgPresentationTerminationEvent struct {
	PresentationId string                           `codec:"0"`
	Source         msgPresentationTerminationSource `codec:"1"`
	Reason         msgPresentationTerminationReason `codec:"2"`
}

// (cddlc) Ident: presentation-connection-open-request
type msgPresentationConnectionOpenRequest struct {
	msgRequest
	PresentationId string `codec:"1"`
	Url            string `codec:"2"`
}

// (cddlc) Ident: presentation-connection-open-response
type msgPresentationConnectionOpenResponse struct {
	msgResponse
	Result          msgResult `codec:"1"`
	ConnectionId    uint64    `codec:"2"`
	ConnectionCount uint64    `codec:"3"`
}

// (cddlc) Ident: msgPresentationConnectionCloseEventReason
type msgPresentationConnectionCloseEventReason uint64

const (
	PresentationConnectionCloseEventReasonCloseMethodCalled                                msgPresentationConnectionCloseEventReason = 1
	PresentationConnectionCloseEventReasonConnectionObjectDiscarded                        msgPresentationConnectionCloseEventReason = 10
	PresentationConnectionCloseEventReasonUnrecoverableErrorWhileSendingOrReceivingMessage msgPresentationConnectionCloseEventReason = 100
)

// (cddlc) Ident: presentation-connection-close-event
type msgPresentationConnectionCloseEvent struct {
	ConnectionId    uint64                                    `codec:"0"`
	Reason          msgPresentationConnectionCloseEventReason `codec:"1"`
	ErrorMessage    *string                                   `codec:"2,omitempty"`
	ConnectionCount uint64                                    `codec:"3"`
}

// (cddlc) Ident: presentation-change-event
type msgPresentationChangeEvent struct {
	PresentationId  string `codec:"0"`
	ConnectionCount uint64 `codec:"1"`
}

// (cddlc) Ident: presentation-connection-message
type msgPresentationConnectionMessage struct {
	ConnectionId uint64 `codec:"0"`
	Message      []byte `codec:"1"`
}

// (cddlc) Ident: msgResult
type msgResult uint64

const (
	ResultSuccess               msgResult = 1
	ResultInvalidUrl            msgResult = 10
	ResultInvalidPresentationId msgResult = 11
	ResultTimeout               msgResult = 100
	ResultTransientError        msgResult = 101
	ResultPermanentError        msgResult = 102
	ResultTerminating           msgResult = 103
	ResultUnknownError          msgResult = 199
)

// (cddlc) Ident: remote-playback-availability-request
type msgRemotePlaybackAvailabilityRequest struct {
	msgRequest
	Sources       []msgRemotePlaybackSource `codec:"1"`
	WatchDuration msgMicroseconds           `codec:"2"`
	WatchId       msgWatchId                `codec:"3"`
}

// (cddlc) Ident: remote-playback-availability-response
type msgRemotePlaybackAvailabilityResponse struct {
	msgResponse
	UrlAvailabilities []msgUrlAvailability `codec:"1"`
}

// (cddlc) Ident: remote-playback-availability-event
type msgRemotePlaybackAvailabilityEvent struct {
	WatchId           msgWatchId           `codec:"0"`
	UrlAvailabilities []msgUrlAvailability `codec:"1"`
}

// (cddlc) Ident: remote-playback-start-request
type msgRemotePlaybackStartRequest struct {
	msgRequest
	RemotePlaybackId msgRemotePlaybackId        `codec:"1"`
	Sources          []msgRemotePlaybackSource  `codec:"2,omitempty"`
	TextTrackUrls    []string                   `codec:"3,omitempty"`
	Headers          []msgHttpHeader            `codec:"4,omitempty"`
	Controls         *msgRemotePlaybackControls `codec:"5,omitempty"`
	Remoting         *struct {
		msgStreamingSessionStartRequestParams
	} `codec:"6,omitempty"`
}

// (cddlc) Ident: remote-playback-source
type msgRemotePlaybackSource struct {
	Url              string `codec:"0"`
	ExtendedMimeType string `codec:"1"`
}

// (cddlc) Ident: remote-playback-start-response
type msgRemotePlaybackStartResponse struct {
	msgResponse
	State    *msgRemotePlaybackState `codec:"1,omitempty"`
	Remoting *struct {
		msgStreamingSessionStartResponseParams
	} `codec:"2,omitempty"`
}

// (cddlc) Ident: msgRemotePlaybackTerminationRequestReason
type msgRemotePlaybackTerminationRequestReason uint64

const (
	RemotePlaybackTerminationRequestReasonUserTerminatedViaController msgRemotePlaybackTerminationRequestReason = 11
	RemotePlaybackTerminationRequestReasonUnknown                     msgRemotePlaybackTerminationRequestReason = 255
)

// (cddlc) Ident: remote-playback-termination-request
type msgRemotePlaybackTerminationRequest struct {
	msgRequest
	RemotePlaybackId msgRemotePlaybackId                       `codec:"1"`
	Reason           msgRemotePlaybackTerminationRequestReason `codec:"2"`
}

// (cddlc) Ident: remote-playback-termination-response
type msgRemotePlaybackTerminationResponse struct {
	msgResponse
	Result msgResult `codec:"1"`
}

// (cddlc) Ident: msgRemotePlaybackTerminationEventReason
type msgRemotePlaybackTerminationEventReason uint64

const (
	RemotePlaybackTerminationEventReasonReceiverCalledTerminate   msgRemotePlaybackTerminationEventReason = 1
	RemotePlaybackTerminationEventReasonUserTerminatedViaReceiver msgRemotePlaybackTerminationEventReason = 2
	RemotePlaybackTerminationEventReasonReceiverIdleTooLong       msgRemotePlaybackTerminationEventReason = 30
	RemotePlaybackTerminationEventReasonReceiverPoweringDown      msgRemotePlaybackTerminationEventReason = 100
	RemotePlaybackTerminationEventReasonReceiverCrashed           msgRemotePlaybackTerminationEventReason = 101
	RemotePlaybackTerminationEventReasonUnknown                   msgRemotePlaybackTerminationEventReason = 255
)

// (cddlc) Ident: remote-playback-termination-event
type msgRemotePlaybackTerminationEvent struct {
	RemotePlaybackId msgRemotePlaybackId                     `codec:"0"`
	Reason           msgRemotePlaybackTerminationEventReason `codec:"1"`
}

// (cddlc) Ident: remote-playback-modify-request
type msgRemotePlaybackModifyRequest struct {
	msgRequest
	RemotePlaybackId msgRemotePlaybackId       `codec:"1"`
	Controls         msgRemotePlaybackControls `codec:"2"`
}

// (cddlc) Ident: remote-playback-modify-response
type msgRemotePlaybackModifyResponse struct {
	msgResponse
	Result msgResult               `codec:"1"`
	State  *msgRemotePlaybackState `codec:"2,omitempty"`
}

// (cddlc) Ident: remote-playback-state-event
type msgRemotePlaybackStateEvent struct {
	RemotePlaybackId msgRemotePlaybackId    `codec:"0"`
	State            msgRemotePlaybackState `codec:"1"`
}

// (cddlc) Ident: remote-playback-id
type msgRemotePlaybackId uint64

// (cddlc) Ident: msgRemotePlaybackControlsPreload
type msgRemotePlaybackControlsPreload uint64

const (
	RemotePlaybackControlsPreloadNone     msgRemotePlaybackControlsPreload = 0
	RemotePlaybackControlsPreloadMetadata msgRemotePlaybackControlsPreload = 1
	RemotePlaybackControlsPreloadAuto     msgRemotePlaybackControlsPreload = 2
)

// (cddlc) Ident: remote-playback-controls
type msgRemotePlaybackControls struct {
	Source               *msgRemotePlaybackSource          `codec:"0,omitempty"`
	Preload              *msgRemotePlaybackControlsPreload `codec:"1,omitempty"`
	Loop                 *bool                             `codec:"2,omitempty"`
	Paused               *bool                             `codec:"3,omitempty"`
	Muted                *bool                             `codec:"4,omitempty"`
	Volume               *float64                          `codec:"5,omitempty"`
	Seek                 *msgMediaTimeline                 `codec:"6,omitempty"`
	FastSeek             *msgMediaTimeline                 `codec:"7,omitempty"`
	PlaybackRate         *float64                          `codec:"8,omitempty"`
	Poster               *string                           `codec:"9,omitempty"`
	EnabledAudioTrackIds []string                          `codec:"10,omitempty"`
	SelectedVideoTrackId *string                           `codec:"11,omitempty"`
	AddedTextTracks      []msgAddedTextTrack               `codec:"12,omitempty"`
	ChangedTextTracks    []msgChangedTextTrack             `codec:"13,omitempty"`
}

// (cddlc) Ident: msgRemotePlaybackStateLoading
type msgRemotePlaybackStateLoading uint64

const (
	RemotePlaybackStateLoadingEmpty    msgRemotePlaybackStateLoading = 0
	RemotePlaybackStateLoadingIdle     msgRemotePlaybackStateLoading = 1
	RemotePlaybackStateLoadingLoading  msgRemotePlaybackStateLoading = 2
	RemotePlaybackStateLoadingNoSource msgRemotePlaybackStateLoading = 3
)

// (cddlc) Ident: msgRemotePlaybackStateLoaded
type msgRemotePlaybackStateLoaded uint64

const (
	RemotePlaybackStateLoadedNothing  msgRemotePlaybackStateLoaded = 0
	RemotePlaybackStateLoadedMetadata msgRemotePlaybackStateLoaded = 1
	RemotePlaybackStateLoadedCurrent  msgRemotePlaybackStateLoaded = 2
	RemotePlaybackStateLoadedFuture   msgRemotePlaybackStateLoaded = 3
	RemotePlaybackStateLoadedEnough   msgRemotePlaybackStateLoaded = 4
)

// (cddlc) Ident: remote-playback-state
type msgRemotePlaybackState struct {
	Supports *struct {
		Rate           bool `codec:"0"`
		Preload        bool `codec:"1"`
		Poster         bool `codec:"2"`
		AddedTextTrack bool `codec:"3"`
		AddedCues      bool `codec:"4"`
	} `codec:"0,omitempty"`
	Source             *msgRemotePlaybackSource       `codec:"1,omitempty"`
	Loading            *msgRemotePlaybackStateLoading `codec:"2,omitempty"`
	Loaded             *msgRemotePlaybackStateLoaded  `codec:"3,omitempty"`
	Error              *msgMediaError                 `codec:"4,omitempty"`
	Epoch              *msgEpochTime                  `codec:"5,omitempty"`
	Duration           *msgMediaTimeline              `codec:"6,omitempty"`
	BufferedTimeRanges []msgMediaTimelineRange        `codec:"7,omitempty"`
	SeekableTimeRanges []msgMediaTimelineRange        `codec:"8,omitempty"`
	PlayedTimeRanges   []msgMediaTimelineRange        `codec:"9,omitempty"`
	Position           *msgMediaTimeline              `codec:"10,omitempty"`
	PlaybackRate       *float64                       `codec:"11,omitempty"`
	Paused             *bool                          `codec:"12,omitempty"`
	Seeking            *bool                          `codec:"13,omitempty"`
	Stalled            *bool                          `codec:"14,omitempty"`
	Ended              *bool                          `codec:"15,omitempty"`
	Volume             *float64                       `codec:"16,omitempty"`
	Muted              *bool                          `codec:"17,omitempty"`
	Resolution         *msgVideoResolution            `codec:"18,omitempty"`
	AudioTracks        []msgAudioTrackState           `codec:"19,omitempty"`
	VideoTracks        []msgVideoTrackState           `codec:"20,omitempty"`
	TextTracks         []msgTextTrackState            `codec:"21,omitempty"`
}

// (cddlc) Ident: msgAddedTextTrackKind
type msgAddedTextTrackKind uint64

const (
	AddedTextTrackKindSubtitles    msgAddedTextTrackKind = 1
	AddedTextTrackKindCaptions     msgAddedTextTrackKind = 2
	AddedTextTrackKindDescriptions msgAddedTextTrackKind = 3
	AddedTextTrackKindChapters     msgAddedTextTrackKind = 4
	AddedTextTrackKindMetadata     msgAddedTextTrackKind = 5
)

// (cddlc) Ident: added-text-track
type msgAddedTextTrack struct {
	Kind     msgAddedTextTrackKind `codec:"0"`
	Label    *string               `codec:"1,omitempty"`
	Language *string               `codec:"2,omitempty"`
}

// (cddlc) Ident: changed-text-track
type msgChangedTextTrack struct {
	Id            string            `codec:"0"`
	Mode          msgTextTrackMode  `codec:"1"`
	AddedCues     []msgTextTrackCue `codec:"2,omitempty"`
	RemovedCueIds []string          `codec:"3,omitempty"`
}

// (cddlc) Ident: msgTextTrackMode
type msgTextTrackMode uint64

const (
	TextTrackModeDisabled msgTextTrackMode = 1
	TextTrackModeShowing  msgTextTrackMode = 2
	TextTrackModeHidden   msgTextTrackMode = 3
)

// (cddlc) Ident: text-track-cue
type msgTextTrackCue struct {
	Id    string                `codec:"0"`
	Range msgMediaTimelineRange `codec:"1"`
	Text  string                `codec:"2"`
}

// (cddlc) Ident: media-sync-time
type msgMediaSyncTime struct {
	Value uint64
	Scale uint64
}

// (cddlc) Ident: msgMediaErrorMsgCode
type msgMediaErrorMsgCode uint64

const (
	MediaErrorMsgCodeUserAborted        msgMediaErrorMsgCode = 1
	MediaErrorMsgCodeNetworkError       msgMediaErrorMsgCode = 2
	MediaErrorMsgCodeDecodeError        msgMediaErrorMsgCode = 3
	MediaErrorMsgCodeSourceNotSupported msgMediaErrorMsgCode = 4
	MediaErrorMsgCodeUnknownError       msgMediaErrorMsgCode = 5
)

// (cddlc) Ident: media-error
type msgMediaError struct {
	Code    msgMediaErrorMsgCode
	Message string
}

// (cddlc) Ident: track-state
type msgTrackState struct {
	Id       string `codec:"0"`
	Label    string `codec:"1"`
	Language string `codec:"2"`
}

// (cddlc) Ident: audio-track-state
type msgAudioTrackState struct {
	msgTrackState
	Enabled bool `codec:"3"`
}

// (cddlc) Ident: video-track-state
type msgVideoTrackState struct {
	msgTrackState
	Selected bool `codec:"3"`
}

// (cddlc) Ident: text-track-state
type msgTextTrackState struct {
	msgTrackState
	Mode msgTextTrackMode `codec:"3"`
}

// (cddlc) Ident: audio-frame
type msgAudioFrame struct {
	EncodingId uint64
	StartTime  uint64
	Payload    []byte
	Optional   *struct {
		Duration *uint64           `codec:"0,omitempty"`
		SyncTime *msgMediaSyncTime `codec:"1,omitempty"`
	}
}

// (cddlc) Ident: video-frame
type msgVideoFrame struct {
	EncodingId     uint64            `codec:"0"`
	SequenceNumber uint64            `codec:"1"`
	DependsOn      []int64           `codec:"2,omitempty"`
	StartTime      uint64            `codec:"3"`
	Duration       *uint64           `codec:"4,omitempty"`
	Payload        []byte            `codec:"5"`
	VideoRotation  *uint64           `codec:"6,omitempty"`
	SyncTime       *msgMediaSyncTime `codec:"7,omitempty"`
}

// (cddlc) Ident: data-frame
type msgDataFrame struct {
	EncodingId     uint64            `codec:"0"`
	SequenceNumber *uint64           `codec:"1,omitempty"`
	StartTime      *uint64           `codec:"2,omitempty"`
	Duration       *uint64           `codec:"3,omitempty"`
	Payload        []byte            `codec:"4"`
	SyncTime       *msgMediaSyncTime `codec:"5,omitempty"`
}

// (cddlc) Ident: ratio
type msgRatio struct {
	Antecedent uint64
	Consequent uint64
}

// (cddlc) Ident: streaming-capabilities-request
type msgStreamingCapabilitiesRequest struct {
	msgRequest
}

// (cddlc) Ident: streaming-capabilities-response
type msgStreamingCapabilitiesResponse struct {
	msgResponse
	StreamingCapabilities msgStreamingCapabilities `codec:"1"`
}

// (cddlc) Ident: streaming-capabilities
type msgStreamingCapabilities struct {
	ReceiveAudio []msgReceiveAudioCapability `codec:"0"`
	ReceiveVideo []msgReceiveVideoCapability `codec:"1"`
	ReceiveData  []msgReceiveDataCapability  `codec:"2"`
}

// (cddlc) Ident: format
type msgFormat struct {
	CodecName string `codec:"0"`
}

// (cddlc) Ident: receive-audio-capability
type msgReceiveAudioCapability struct {
	Codec            msgFormat `codec:"0"`
	MaxAudioChannels *uint64   `codec:"1,omitempty"`
	MinBitRate       *uint64   `codec:"2,omitempty"`
}

// (cddlc) Ident: video-resolution
type msgVideoResolution struct {
	Height uint64 `codec:"0"`
	Width  uint64 `codec:"1"`
}

// (cddlc) Ident: video-hdr-format
type msgVideoHdrFormat struct {
	TransferFunction string  `codec:"0"`
	HdrMetadata      *string `codec:"1,omitempty"`
}

// (cddlc) Ident: receive-video-capability
type msgReceiveVideoCapability struct {
	Codec              msgFormat            `codec:"0"`
	MaxResolution      *msgVideoResolution  `codec:"1,omitempty"`
	MaxFramesPerSecond *msgRatio            `codec:"2,omitempty"`
	MaxPixelsPerSecond *uint64              `codec:"3,omitempty"`
	MinBitRate         *uint64              `codec:"4,omitempty"`
	AspectRatio        *msgRatio            `codec:"5,omitempty"`
	ColorGamut         *string              `codec:"6,omitempty"`
	NativeResolutions  []msgVideoResolution `codec:"7,omitempty"`
	SupportsScaling    *bool                `codec:"8,omitempty"`
	SupportsRotation   *bool                `codec:"9,omitempty"`
	HdrFormats         []msgVideoHdrFormat  `codec:"10,omitempty"`
}

// (cddlc) Ident: receive-data-capability
type msgReceiveDataCapability struct {
	DataType msgFormat `codec:"0"`
}

// (cddlc) Ident: streaming-session-start-request
type msgStreamingSessionStartRequest struct {
	msgRequest
	msgStreamingSessionStartRequestParams
}

// (cddlc) Ident: streaming-session-start-response
type msgStreamingSessionStartResponse struct {
	msgResponse
	msgStreamingSessionStartResponseParams
}

// (cddlc) Ident: streaming-session-start-request-params
type msgStreamingSessionStartRequestParams struct {
	StreamingSessionId   uint64                `codec:"1"`
	StreamOffers         []msgMediaStreamOffer `codec:"2"`
	DesiredStatsInterval msgMicroseconds       `codec:"3"`
}

// (cddlc) Ident: streaming-session-modify-request
type msgStreamingSessionModifyRequest struct {
	msgRequest
	msgStreamingSessionModifyRequestParams
}

// (cddlc) Ident: streaming-session-start-response-params
type msgStreamingSessionStartResponseParams struct {
	Result               msgResult               `codec:"1"`
	StreamRequests       []msgMediaStreamRequest `codec:"2"`
	DesiredStatsInterval msgMicroseconds         `codec:"3"`
}

// (cddlc) Ident: streaming-session-modify-request-params
type msgStreamingSessionModifyRequestParams struct {
	StreamingSessionId uint64                  `codec:"1"`
	StreamRequests     []msgMediaStreamRequest `codec:"2"`
}

// (cddlc) Ident: streaming-session-modify-response
type msgStreamingSessionModifyResponse struct {
	msgResponse
	Result msgResult `codec:"1"`
}

// (cddlc) Ident: streaming-session-terminate-request
type msgStreamingSessionTerminateRequest struct {
	msgRequest
	StreamingSessionId uint64 `codec:"1"`
}

// (cddlc) Ident: streaming-session-terminate-response
type msgStreamingSessionTerminateResponse struct {
	msgResponse
}

// (cddlc) Ident: streaming-session-terminate-event
type msgStreamingSessionTerminateEvent struct {
	StreamingSessionId uint64 `codec:"0"`
}

// (cddlc) Ident: media-stream-offer
type msgMediaStreamOffer struct {
	MediaStreamId uint64                  `codec:"0"`
	DisplayName   *string                 `codec:"1,omitempty"`
	Audio         []msgAudioEncodingOffer `codec:"2,omitempty"`
	Video         []msgVideoEncodingOffer `codec:"3,omitempty"`
	Data          []msgDataEncodingOffer  `codec:"4,omitempty"`
}

// (cddlc) Ident: media-stream-request
type msgMediaStreamRequest struct {
	MediaStreamId uint64                   `codec:"0"`
	Audio         *msgAudioEncodingRequest `codec:"1,omitempty"`
	Video         *msgVideoEncodingRequest `codec:"2,omitempty"`
	Data          *msgDataEncodingRequest  `codec:"3,omitempty"`
}

// (cddlc) Ident: audio-encoding-offer
type msgAudioEncodingOffer struct {
	EncodingId      uint64  `codec:"0"`
	CodecName       string  `codec:"1"`
	TimeScale       uint64  `codec:"2"`
	DefaultDuration *uint64 `codec:"3,omitempty"`
}

// (cddlc) Ident: video-encoding-offer
type msgVideoEncodingOffer struct {
	EncodingId      uint64            `codec:"0"`
	CodecName       string            `codec:"1"`
	TimeScale       uint64            `codec:"2"`
	DefaultDuration *uint64           `codec:"3,omitempty"`
	DefaultRotation *msgVideoRotation `codec:"4,omitempty"`
}

// (cddlc) Ident: data-encoding-offer
type msgDataEncodingOffer struct {
	EncodingId      uint64  `codec:"0"`
	DataTypeName    string  `codec:"1"`
	TimeScale       uint64  `codec:"2"`
	DefaultDuration *uint64 `codec:"3,omitempty"`
}

// (cddlc) Ident: audio-encoding-request
type msgAudioEncodingRequest struct {
	EncodingId uint64 `codec:"0"`
}

// (cddlc) Ident: video-encoding-request
type msgVideoEncodingRequest struct {
	EncodingId         uint64              `codec:"0"`
	TargetResolution   *msgVideoResolution `codec:"1,omitempty"`
	MaxFramesPerSecond *msgRatio           `codec:"2,omitempty"`
}

// (cddlc) Ident: data-encoding-request
type msgDataEncodingRequest struct {
	EncodingId uint64 `codec:"0"`
}

// (cddlc) Ident: msgVideoRotation
type msgVideoRotation uint64

const (
	VideoRotationVideoRotation0   msgVideoRotation = 0
	VideoRotationVideoRotation90  msgVideoRotation = 1
	VideoRotationVideoRotation180 msgVideoRotation = 2
	VideoRotationVideoRotation270 msgVideoRotation = 3
)

// (cddlc) Ident: sender-stats-audio
type msgSenderStatsAudio struct {
	EncodingId            uint64           `codec:"0"`
	CumulativeSentFrames  *uint64          `codec:"1,omitempty"`
	CumulativeEncodeDelay *msgMicroseconds `codec:"2,omitempty"`
}

// (cddlc) Ident: sender-stats-video
type msgSenderStatsVideo struct {
	EncodingId              uint64           `codec:"0"`
	CumulativeSentDuration  *msgMicroseconds `codec:"1,omitempty"`
	CumulativeEncodeDelay   *msgMicroseconds `codec:"2,omitempty"`
	CumulativeDroppedFrames *uint64          `codec:"3,omitempty"`
}

// (cddlc) Ident: streaming-session-sender-stats-event
type msgStreamingSessionSenderStatsEvent struct {
	StreamingSessionId uint64                `codec:"0"`
	SystemTime         msgMicroseconds       `codec:"1"`
	Audio              []msgSenderStatsAudio `codec:"2,omitempty"`
	Video              []msgSenderStatsVideo `codec:"3,omitempty"`
}

// (cddlc) Ident: msgStreamingBufferStatus
type msgStreamingBufferStatus uint64

const (
	StreamingBufferStatusEnoughData       msgStreamingBufferStatus = 0
	StreamingBufferStatusInsufficientData msgStreamingBufferStatus = 1
	StreamingBufferStatusTooMuchData      msgStreamingBufferStatus = 2
)

// (cddlc) Ident: receiver-stats-audio
type msgReceiverStatsAudio struct {
	EncodingId                 uint64                    `codec:"0"`
	CumulativeReceivedDuration *msgMicroseconds          `codec:"1,omitempty"`
	CumulativeLostDuration     *msgMicroseconds          `codec:"2,omitempty"`
	CumulativeBufferDelay      *msgMicroseconds          `codec:"3,omitempty"`
	CumulativeDecodeDelay      *msgMicroseconds          `codec:"4,omitempty"`
	RemoteBufferStatus         *msgStreamingBufferStatus `codec:"5,omitempty"`
}

// (cddlc) Ident: receiver-stats-video
type msgReceiverStatsVideo struct {
	EncodingId              uint64                    `codec:"0"`
	CumulativeDecodedFrames *uint64                   `codec:"1,omitempty"`
	CumulativeLostFrames    *uint64                   `codec:"2,omitempty"`
	CumulativeBufferDelay   *msgMicroseconds          `codec:"3,omitempty"`
	CumulativeDecodeDelay   *msgMicroseconds          `codec:"4,omitempty"`
	RemoteBufferStatus      *msgStreamingBufferStatus `codec:"5,omitempty"`
}

// (cddlc) Ident: streaming-session-receiver-stats-event
type msgStreamingSessionReceiverStatsEvent struct {
	StreamingSessionId uint64                  `codec:"0"`
	SystemTime         msgMicroseconds         `codec:"1"`
	Audio              []msgReceiverStatsAudio `codec:"2,omitempty"`
	Video              []msgReceiverStatsVideo `codec:"3,omitempty"`
}
