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
	AgentInfo msgAgentInfo `cbor:"1,keyasint"`
}

// (cddlc) Ident: agent-info-event
type msgAgentInfoEvent struct {
	AgentInfo msgAgentInfo `cbor:"0,keyasint"`
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
	DisplayName  string               `cbor:"0,keyasint"`
	ModelName    string               `cbor:"1,keyasint"`
	Capabilities []msgAgentCapability `cbor:"2,keyasint"`
	StateToken   string               `cbor:"3,keyasint"`
	Locales      []string             `cbor:"4,keyasint"`
}

// (cddlc) Ident: agent-status-request
type msgAgentStatusRequest struct {
	msgRequest
	Status *msgStatus `cbor:"1,keyasint,omitempty"`
}

// (cddlc) Ident: agent-status-response
type msgAgentStatusResponse struct {
	msgResponse
	Status *msgStatus `cbor:"1,keyasint,omitempty"`
}

// (cddlc) Ident: status
type msgStatus struct {
	Status string `cbor:"0,keyasint"`
}

// (cddlc) Ident: request
type msgRequest struct {
	RequestId msgRequestId `cbor:"0,keyasint"`
}

// (cddlc) Ident: response
type msgResponse struct {
	RequestId msgRequestId `cbor:"0,keyasint"`
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
	PskEaseOfInput      uint64              `cbor:"0,keyasint"`
	PskInputMethods     []msgPskInputMethod `cbor:"1,keyasint"`
	PskMinBitsOfEntropy uint64              `cbor:"2,keyasint"`
}

// (cddlc) Ident: msgPskInputMethod
type msgPskInputMethod uint64

const (
	PskInputMethodNumeric msgPskInputMethod = 0
	PskInputMethodQrCode  msgPskInputMethod = 1
)

// (cddlc) Ident: auth-initiation-token
type msgAuthInitiationToken struct {
	Token *string `cbor:"0,keyasint,omitempty"`
}

// (cddlc) Ident: msgAuthSpake2PskStatus
type msgAuthSpake2PskStatus uint64

const (
	AuthSpake2PskStatusPskNeedsPresentation msgAuthSpake2PskStatus = 0
	AuthSpake2PskStatusPskShown             msgAuthSpake2PskStatus = 1
	AuthSpake2PskStatusPskInput             msgAuthSpake2PskStatus = 2
)

// (cddlc) Ident: auth-spake2-confirmation
type msgAuthSpake2Confirmation struct {
	ConfirmationValue []byte `cbor:"0,keyasint"`
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
	Result msgAuthStatusResult `cbor:"0,keyasint"`
}

// (cddlc) Ident: auth-spake2-handshake
type msgAuthSpake2Handshake struct {
	AuthInitiationToken msgAuthInitiationToken `cbor:"0,keyasint"`
	PskStatus           msgAuthSpake2PskStatus `cbor:"1,keyasint"`
	PublicValue         []byte                 `cbor:"2,keyasint"`
}

// (cddlc) Ident: watch-id
type msgWatchId uint64

// (cddlc) Ident: presentation-url-availability-request
type msgPresentationUrlAvailabilityRequest struct {
	msgRequest
	Urls          []string        `cbor:"1,keyasint"`
	WatchDuration msgMicroseconds `cbor:"2,keyasint"`
	WatchId       msgWatchId      `cbor:"3,keyasint"`
}

// (cddlc) Ident: presentation-url-availability-response
type msgPresentationUrlAvailabilityResponse struct {
	msgResponse
	UrlAvailabilities []msgUrlAvailability `cbor:"1,keyasint"`
}

// (cddlc) Ident: presentation-url-availability-event
type msgPresentationUrlAvailabilityEvent struct {
	WatchId           msgWatchId           `cbor:"0,keyasint"`
	UrlAvailabilities []msgUrlAvailability `cbor:"1,keyasint"`
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
	PresentationId string          `cbor:"1,keyasint"`
	Url            string          `cbor:"2,keyasint"`
	Headers        []msgHttpHeader `cbor:"3,keyasint"`
}

// (cddlc) Ident: http-header
type msgHttpHeader struct {
	Key   string
	Value string
}

// (cddlc) Ident: presentation-start-response
type msgPresentationStartResponse struct {
	msgResponse
	Result           msgResult `cbor:"1,keyasint"`
	ConnectionId     uint64    `cbor:"2,keyasint"`
	HttpResponseCode *uint64   `cbor:"3,keyasint,omitempty"`
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
	PresentationId string                           `cbor:"1,keyasint"`
	Reason         msgPresentationTerminationReason `cbor:"2,keyasint"`
}

// (cddlc) Ident: presentation-termination-response
type msgPresentationTerminationResponse struct {
	msgResponse
	Result msgResult `cbor:"1,keyasint"`
}

// (cddlc) Ident: presentation-termination-event
type msgPresentationTerminationEvent struct {
	PresentationId string                           `cbor:"0,keyasint"`
	Source         msgPresentationTerminationSource `cbor:"1,keyasint"`
	Reason         msgPresentationTerminationReason `cbor:"2,keyasint"`
}

// (cddlc) Ident: presentation-connection-open-request
type msgPresentationConnectionOpenRequest struct {
	msgRequest
	PresentationId string `cbor:"1,keyasint"`
	Url            string `cbor:"2,keyasint"`
}

// (cddlc) Ident: presentation-connection-open-response
type msgPresentationConnectionOpenResponse struct {
	msgResponse
	Result          msgResult `cbor:"1,keyasint"`
	ConnectionId    uint64    `cbor:"2,keyasint"`
	ConnectionCount uint64    `cbor:"3,keyasint"`
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
	ConnectionId    uint64                                    `cbor:"0,keyasint"`
	Reason          msgPresentationConnectionCloseEventReason `cbor:"1,keyasint"`
	ErrorMessage    *string                                   `cbor:"2,keyasint,omitempty"`
	ConnectionCount uint64                                    `cbor:"3,keyasint"`
}

// (cddlc) Ident: presentation-change-event
type msgPresentationChangeEvent struct {
	PresentationId  string `cbor:"0,keyasint"`
	ConnectionCount uint64 `cbor:"1,keyasint"`
}

// (cddlc) Ident: presentation-connection-message
type msgPresentationConnectionMessage struct {
	ConnectionId uint64 `cbor:"0,keyasint"`
	Message      []byte `cbor:"1,keyasint"`
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
	Sources       []msgRemotePlaybackSource `cbor:"1,keyasint"`
	WatchDuration msgMicroseconds           `cbor:"2,keyasint"`
	WatchId       msgWatchId                `cbor:"3,keyasint"`
}

// (cddlc) Ident: remote-playback-availability-response
type msgRemotePlaybackAvailabilityResponse struct {
	msgResponse
	UrlAvailabilities []msgUrlAvailability `cbor:"1,keyasint"`
}

// (cddlc) Ident: remote-playback-availability-event
type msgRemotePlaybackAvailabilityEvent struct {
	WatchId           msgWatchId           `cbor:"0,keyasint"`
	UrlAvailabilities []msgUrlAvailability `cbor:"1,keyasint"`
}

// (cddlc) Ident: remote-playback-start-request
type msgRemotePlaybackStartRequest struct {
	msgRequest
	RemotePlaybackId msgRemotePlaybackId        `cbor:"1,keyasint"`
	Sources          []msgRemotePlaybackSource  `cbor:"2,keyasint,omitempty"`
	TextTrackUrls    []string                   `cbor:"3,keyasint,omitempty"`
	Headers          []msgHttpHeader            `cbor:"4,keyasint,omitempty"`
	Controls         *msgRemotePlaybackControls `cbor:"5,keyasint,omitempty"`
	Remoting         *struct {
		msgStreamingSessionStartRequestParams
	} `cbor:"6,keyasint,omitempty"`
}

// (cddlc) Ident: remote-playback-source
type msgRemotePlaybackSource struct {
	Url              string `cbor:"0,keyasint"`
	ExtendedMimeType string `cbor:"1,keyasint"`
}

// (cddlc) Ident: remote-playback-start-response
type msgRemotePlaybackStartResponse struct {
	msgResponse
	State    *msgRemotePlaybackState `cbor:"1,keyasint,omitempty"`
	Remoting *struct {
		msgStreamingSessionStartResponseParams
	} `cbor:"2,keyasint,omitempty"`
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
	RemotePlaybackId msgRemotePlaybackId                       `cbor:"1,keyasint"`
	Reason           msgRemotePlaybackTerminationRequestReason `cbor:"2,keyasint"`
}

// (cddlc) Ident: remote-playback-termination-response
type msgRemotePlaybackTerminationResponse struct {
	msgResponse
	Result msgResult `cbor:"1,keyasint"`
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
	RemotePlaybackId msgRemotePlaybackId                     `cbor:"0,keyasint"`
	Reason           msgRemotePlaybackTerminationEventReason `cbor:"1,keyasint"`
}

// (cddlc) Ident: remote-playback-modify-request
type msgRemotePlaybackModifyRequest struct {
	msgRequest
	RemotePlaybackId msgRemotePlaybackId       `cbor:"1,keyasint"`
	Controls         msgRemotePlaybackControls `cbor:"2,keyasint"`
}

// (cddlc) Ident: remote-playback-modify-response
type msgRemotePlaybackModifyResponse struct {
	msgResponse
	Result msgResult               `cbor:"1,keyasint"`
	State  *msgRemotePlaybackState `cbor:"2,keyasint,omitempty"`
}

// (cddlc) Ident: remote-playback-state-event
type msgRemotePlaybackStateEvent struct {
	RemotePlaybackId msgRemotePlaybackId    `cbor:"0,keyasint"`
	State            msgRemotePlaybackState `cbor:"1,keyasint"`
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
	Source               *msgRemotePlaybackSource          `cbor:"0,keyasint,omitempty"`
	Preload              *msgRemotePlaybackControlsPreload `cbor:"1,keyasint,omitempty"`
	Loop                 *bool                             `cbor:"2,keyasint,omitempty"`
	Paused               *bool                             `cbor:"3,keyasint,omitempty"`
	Muted                *bool                             `cbor:"4,keyasint,omitempty"`
	Volume               *float64                          `cbor:"5,keyasint,omitempty"`
	Seek                 *msgMediaTimeline                 `cbor:"6,keyasint,omitempty"`
	FastSeek             *msgMediaTimeline                 `cbor:"7,keyasint,omitempty"`
	PlaybackRate         *float64                          `cbor:"8,keyasint,omitempty"`
	Poster               *string                           `cbor:"9,keyasint,omitempty"`
	EnabledAudioTrackIds []string                          `cbor:"10,keyasint,omitempty"`
	SelectedVideoTrackId *string                           `cbor:"11,keyasint,omitempty"`
	AddedTextTracks      []msgAddedTextTrack               `cbor:"12,keyasint,omitempty"`
	ChangedTextTracks    []msgChangedTextTrack             `cbor:"13,keyasint,omitempty"`
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
		Rate           bool `cbor:"0,keyasint"`
		Preload        bool `cbor:"1,keyasint"`
		Poster         bool `cbor:"2,keyasint"`
		AddedTextTrack bool `cbor:"3,keyasint"`
		AddedCues      bool `cbor:"4,keyasint"`
	} `cbor:"0,keyasint,omitempty"`
	Source             *msgRemotePlaybackSource       `cbor:"1,keyasint,omitempty"`
	Loading            *msgRemotePlaybackStateLoading `cbor:"2,keyasint,omitempty"`
	Loaded             *msgRemotePlaybackStateLoaded  `cbor:"3,keyasint,omitempty"`
	Error              *msgMediaError                 `cbor:"4,keyasint,omitempty"`
	Epoch              *msgEpochTime                  `cbor:"5,keyasint,omitempty"`
	Duration           *msgMediaTimeline              `cbor:"6,keyasint,omitempty"`
	BufferedTimeRanges []msgMediaTimelineRange        `cbor:"7,keyasint,omitempty"`
	SeekableTimeRanges []msgMediaTimelineRange        `cbor:"8,keyasint,omitempty"`
	PlayedTimeRanges   []msgMediaTimelineRange        `cbor:"9,keyasint,omitempty"`
	Position           *msgMediaTimeline              `cbor:"10,keyasint,omitempty"`
	PlaybackRate       *float64                       `cbor:"11,keyasint,omitempty"`
	Paused             *bool                          `cbor:"12,keyasint,omitempty"`
	Seeking            *bool                          `cbor:"13,keyasint,omitempty"`
	Stalled            *bool                          `cbor:"14,keyasint,omitempty"`
	Ended              *bool                          `cbor:"15,keyasint,omitempty"`
	Volume             *float64                       `cbor:"16,keyasint,omitempty"`
	Muted              *bool                          `cbor:"17,keyasint,omitempty"`
	Resolution         *msgVideoResolution            `cbor:"18,keyasint,omitempty"`
	AudioTracks        []msgAudioTrackState           `cbor:"19,keyasint,omitempty"`
	VideoTracks        []msgVideoTrackState           `cbor:"20,keyasint,omitempty"`
	TextTracks         []msgTextTrackState            `cbor:"21,keyasint,omitempty"`
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
	Kind     msgAddedTextTrackKind `cbor:"0,keyasint"`
	Label    *string               `cbor:"1,keyasint,omitempty"`
	Language *string               `cbor:"2,keyasint,omitempty"`
}

// (cddlc) Ident: changed-text-track
type msgChangedTextTrack struct {
	Id            string            `cbor:"0,keyasint"`
	Mode          msgTextTrackMode  `cbor:"1,keyasint"`
	AddedCues     []msgTextTrackCue `cbor:"2,keyasint,omitempty"`
	RemovedCueIds []string          `cbor:"3,keyasint,omitempty"`
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
	Id    string                `cbor:"0,keyasint"`
	Range msgMediaTimelineRange `cbor:"1,keyasint"`
	Text  string                `cbor:"2,keyasint"`
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
	Id       string `cbor:"0,keyasint"`
	Label    string `cbor:"1,keyasint"`
	Language string `cbor:"2,keyasint"`
}

// (cddlc) Ident: audio-track-state
type msgAudioTrackState struct {
	msgTrackState
	Enabled bool `cbor:"3,keyasint"`
}

// (cddlc) Ident: video-track-state
type msgVideoTrackState struct {
	msgTrackState
	Selected bool `cbor:"3,keyasint"`
}

// (cddlc) Ident: text-track-state
type msgTextTrackState struct {
	msgTrackState
	Mode msgTextTrackMode `cbor:"3,keyasint"`
}

// (cddlc) Ident: audio-frame
type msgAudioFrame struct {
	EncodingId uint64
	StartTime  uint64
	Payload    []byte
	Optional   *struct {
		Duration *uint64           `cbor:"0,keyasint,omitempty"`
		SyncTime *msgMediaSyncTime `cbor:"1,keyasint,omitempty"`
	}
}

// (cddlc) Ident: video-frame
type msgVideoFrame struct {
	EncodingId     uint64            `cbor:"0,keyasint"`
	SequenceNumber uint64            `cbor:"1,keyasint"`
	DependsOn      []int64           `cbor:"2,keyasint,omitempty"`
	StartTime      uint64            `cbor:"3,keyasint"`
	Duration       *uint64           `cbor:"4,keyasint,omitempty"`
	Payload        []byte            `cbor:"5,keyasint"`
	VideoRotation  *uint64           `cbor:"6,keyasint,omitempty"`
	SyncTime       *msgMediaSyncTime `cbor:"7,keyasint,omitempty"`
}

// (cddlc) Ident: data-frame
type msgDataFrame struct {
	EncodingId     uint64            `cbor:"0,keyasint"`
	SequenceNumber *uint64           `cbor:"1,keyasint,omitempty"`
	StartTime      *uint64           `cbor:"2,keyasint,omitempty"`
	Duration       *uint64           `cbor:"3,keyasint,omitempty"`
	Payload        []byte            `cbor:"4,keyasint"`
	SyncTime       *msgMediaSyncTime `cbor:"5,keyasint,omitempty"`
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
	StreamingCapabilities msgStreamingCapabilities `cbor:"1,keyasint"`
}

// (cddlc) Ident: streaming-capabilities
type msgStreamingCapabilities struct {
	ReceiveAudio []msgReceiveAudioCapability `cbor:"0,keyasint"`
	ReceiveVideo []msgReceiveVideoCapability `cbor:"1,keyasint"`
	ReceiveData  []msgReceiveDataCapability  `cbor:"2,keyasint"`
}

// (cddlc) Ident: format
type msgFormat struct {
	CodecName string `cbor:"0,keyasint"`
}

// (cddlc) Ident: receive-audio-capability
type msgReceiveAudioCapability struct {
	Codec            msgFormat `cbor:"0,keyasint"`
	MaxAudioChannels *uint64   `cbor:"1,keyasint,omitempty"`
	MinBitRate       *uint64   `cbor:"2,keyasint,omitempty"`
}

// (cddlc) Ident: video-resolution
type msgVideoResolution struct {
	Height uint64 `cbor:"0,keyasint"`
	Width  uint64 `cbor:"1,keyasint"`
}

// (cddlc) Ident: video-hdr-format
type msgVideoHdrFormat struct {
	TransferFunction string  `cbor:"0,keyasint"`
	HdrMetadata      *string `cbor:"1,keyasint,omitempty"`
}

// (cddlc) Ident: receive-video-capability
type msgReceiveVideoCapability struct {
	Codec              msgFormat            `cbor:"0,keyasint"`
	MaxResolution      *msgVideoResolution  `cbor:"1,keyasint,omitempty"`
	MaxFramesPerSecond *msgRatio            `cbor:"2,keyasint,omitempty"`
	MaxPixelsPerSecond *uint64              `cbor:"3,keyasint,omitempty"`
	MinBitRate         *uint64              `cbor:"4,keyasint,omitempty"`
	AspectRatio        *msgRatio            `cbor:"5,keyasint,omitempty"`
	ColorGamut         *string              `cbor:"6,keyasint,omitempty"`
	NativeResolutions  []msgVideoResolution `cbor:"7,keyasint,omitempty"`
	SupportsScaling    *bool                `cbor:"8,keyasint,omitempty"`
	SupportsRotation   *bool                `cbor:"9,keyasint,omitempty"`
	HdrFormats         []msgVideoHdrFormat  `cbor:"10,keyasint,omitempty"`
}

// (cddlc) Ident: receive-data-capability
type msgReceiveDataCapability struct {
	DataType msgFormat `cbor:"0,keyasint"`
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
	StreamingSessionId   uint64                `cbor:"1,keyasint"`
	StreamOffers         []msgMediaStreamOffer `cbor:"2,keyasint"`
	DesiredStatsInterval msgMicroseconds       `cbor:"3,keyasint"`
}

// (cddlc) Ident: streaming-session-modify-request
type msgStreamingSessionModifyRequest struct {
	msgRequest
	msgStreamingSessionModifyRequestParams
}

// (cddlc) Ident: streaming-session-start-response-params
type msgStreamingSessionStartResponseParams struct {
	Result               msgResult               `cbor:"1,keyasint"`
	StreamRequests       []msgMediaStreamRequest `cbor:"2,keyasint"`
	DesiredStatsInterval msgMicroseconds         `cbor:"3,keyasint"`
}

// (cddlc) Ident: streaming-session-modify-request-params
type msgStreamingSessionModifyRequestParams struct {
	StreamingSessionId uint64                  `cbor:"1,keyasint"`
	StreamRequests     []msgMediaStreamRequest `cbor:"2,keyasint"`
}

// (cddlc) Ident: streaming-session-modify-response
type msgStreamingSessionModifyResponse struct {
	msgResponse
	Result msgResult `cbor:"1,keyasint"`
}

// (cddlc) Ident: streaming-session-terminate-request
type msgStreamingSessionTerminateRequest struct {
	msgRequest
	StreamingSessionId uint64 `cbor:"1,keyasint"`
}

// (cddlc) Ident: streaming-session-terminate-response
type msgStreamingSessionTerminateResponse struct {
	msgResponse
}

// (cddlc) Ident: streaming-session-terminate-event
type msgStreamingSessionTerminateEvent struct {
	StreamingSessionId uint64 `cbor:"0,keyasint"`
}

// (cddlc) Ident: media-stream-offer
type msgMediaStreamOffer struct {
	MediaStreamId uint64                  `cbor:"0,keyasint"`
	DisplayName   *string                 `cbor:"1,keyasint,omitempty"`
	Audio         []msgAudioEncodingOffer `cbor:"2,keyasint,omitempty"`
	Video         []msgVideoEncodingOffer `cbor:"3,keyasint,omitempty"`
	Data          []msgDataEncodingOffer  `cbor:"4,keyasint,omitempty"`
}

// (cddlc) Ident: media-stream-request
type msgMediaStreamRequest struct {
	MediaStreamId uint64                   `cbor:"0,keyasint"`
	Audio         *msgAudioEncodingRequest `cbor:"1,keyasint,omitempty"`
	Video         *msgVideoEncodingRequest `cbor:"2,keyasint,omitempty"`
	Data          *msgDataEncodingRequest  `cbor:"3,keyasint,omitempty"`
}

// (cddlc) Ident: audio-encoding-offer
type msgAudioEncodingOffer struct {
	EncodingId      uint64  `cbor:"0,keyasint"`
	CodecName       string  `cbor:"1,keyasint"`
	TimeScale       uint64  `cbor:"2,keyasint"`
	DefaultDuration *uint64 `cbor:"3,keyasint,omitempty"`
}

// (cddlc) Ident: video-encoding-offer
type msgVideoEncodingOffer struct {
	EncodingId      uint64            `cbor:"0,keyasint"`
	CodecName       string            `cbor:"1,keyasint"`
	TimeScale       uint64            `cbor:"2,keyasint"`
	DefaultDuration *uint64           `cbor:"3,keyasint,omitempty"`
	DefaultRotation *msgVideoRotation `cbor:"4,keyasint,omitempty"`
}

// (cddlc) Ident: data-encoding-offer
type msgDataEncodingOffer struct {
	EncodingId      uint64  `cbor:"0,keyasint"`
	DataTypeName    string  `cbor:"1,keyasint"`
	TimeScale       uint64  `cbor:"2,keyasint"`
	DefaultDuration *uint64 `cbor:"3,keyasint,omitempty"`
}

// (cddlc) Ident: audio-encoding-request
type msgAudioEncodingRequest struct {
	EncodingId uint64 `cbor:"0,keyasint"`
}

// (cddlc) Ident: video-encoding-request
type msgVideoEncodingRequest struct {
	EncodingId         uint64              `cbor:"0,keyasint"`
	TargetResolution   *msgVideoResolution `cbor:"1,keyasint,omitempty"`
	MaxFramesPerSecond *msgRatio           `cbor:"2,keyasint,omitempty"`
}

// (cddlc) Ident: data-encoding-request
type msgDataEncodingRequest struct {
	EncodingId uint64 `cbor:"0,keyasint"`
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
	EncodingId            uint64           `cbor:"0,keyasint"`
	CumulativeSentFrames  *uint64          `cbor:"1,keyasint,omitempty"`
	CumulativeEncodeDelay *msgMicroseconds `cbor:"2,keyasint,omitempty"`
}

// (cddlc) Ident: sender-stats-video
type msgSenderStatsVideo struct {
	EncodingId              uint64           `cbor:"0,keyasint"`
	CumulativeSentDuration  *msgMicroseconds `cbor:"1,keyasint,omitempty"`
	CumulativeEncodeDelay   *msgMicroseconds `cbor:"2,keyasint,omitempty"`
	CumulativeDroppedFrames *uint64          `cbor:"3,keyasint,omitempty"`
}

// (cddlc) Ident: streaming-session-sender-stats-event
type msgStreamingSessionSenderStatsEvent struct {
	StreamingSessionId uint64                `cbor:"0,keyasint"`
	SystemTime         msgMicroseconds       `cbor:"1,keyasint"`
	Audio              []msgSenderStatsAudio `cbor:"2,keyasint,omitempty"`
	Video              []msgSenderStatsVideo `cbor:"3,keyasint,omitempty"`
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
	EncodingId                 uint64                    `cbor:"0,keyasint"`
	CumulativeReceivedDuration *msgMicroseconds          `cbor:"1,keyasint,omitempty"`
	CumulativeLostDuration     *msgMicroseconds          `cbor:"2,keyasint,omitempty"`
	CumulativeBufferDelay      *msgMicroseconds          `cbor:"3,keyasint,omitempty"`
	CumulativeDecodeDelay      *msgMicroseconds          `cbor:"4,keyasint,omitempty"`
	RemoteBufferStatus         *msgStreamingBufferStatus `cbor:"5,keyasint,omitempty"`
}

// (cddlc) Ident: receiver-stats-video
type msgReceiverStatsVideo struct {
	EncodingId              uint64                    `cbor:"0,keyasint"`
	CumulativeDecodedFrames *uint64                   `cbor:"1,keyasint,omitempty"`
	CumulativeLostFrames    *uint64                   `cbor:"2,keyasint,omitempty"`
	CumulativeBufferDelay   *msgMicroseconds          `cbor:"3,keyasint,omitempty"`
	CumulativeDecodeDelay   *msgMicroseconds          `cbor:"4,keyasint,omitempty"`
	RemoteBufferStatus      *msgStreamingBufferStatus `cbor:"5,keyasint,omitempty"`
}

// (cddlc) Ident: streaming-session-receiver-stats-event
type msgStreamingSessionReceiverStatsEvent struct {
	StreamingSessionId uint64                  `cbor:"0,keyasint"`
	SystemTime         msgMicroseconds         `cbor:"1,keyasint"`
	Audio              []msgReceiverStatsAudio `cbor:"2,keyasint,omitempty"`
	Video              []msgReceiverStatsVideo `cbor:"3,keyasint,omitempty"`
}

//
// Exported types for application protocol use
//

// RequestID is the request identifier type
type RequestID = msgRequestId

// AgentCapability represents an agent capability
type AgentCapability = msgAgentCapability

// Exported capability constants
const (
	CapabilityReceiveAudio          = AgentCapabilityReceiveAudio
	CapabilityReceiveVideo          = AgentCapabilityReceiveVideo
	CapabilityReceivePresentation   = AgentCapabilityReceivePresentation
	CapabilityControlPresentation   = AgentCapabilityControlPresentation
	CapabilityReceiveRemotePlayback = AgentCapabilityReceiveRemotePlayback
	CapabilityControlRemotePlayback = AgentCapabilityControlRemotePlayback
	CapabilityReceiveStreaming      = AgentCapabilityReceiveStreaming
	CapabilitySendStreaming         = AgentCapabilitySendStreaming
)

// MsgAgentInfo is the exported agent-info structure
type MsgAgentInfo struct {
	DisplayName  string            `cbor:"0,keyasint"`
	ModelName    string            `cbor:"1,keyasint,omitempty"`
	Capabilities []AgentCapability `cbor:"2,keyasint"`
	StateToken   string            `cbor:"3,keyasint"`
	Locales      []string          `cbor:"4,keyasint"`
}

// MsgAgentInfoRequest is the exported agent-info-request message
type MsgAgentInfoRequest struct {
	RequestID RequestID `cbor:"0,keyasint"`
}

// MsgAgentInfoResponse is the exported agent-info-response message
type MsgAgentInfoResponse struct {
	RequestID RequestID    `cbor:"0,keyasint"`
	AgentInfo MsgAgentInfo `cbor:"1,keyasint"`
}

// MsgAgentStatusRequest is the exported agent-status-request message
type MsgAgentStatusRequest struct {
	RequestID RequestID `cbor:"0,keyasint"`
	Status    *string   `cbor:"1,keyasint,omitempty"`
}

// MsgAgentStatusResponse is the exported agent-status-response message
type MsgAgentStatusResponse struct {
	RequestID RequestID `cbor:"0,keyasint"`
	Status    *string   `cbor:"1,keyasint,omitempty"`
}

// MsgAgentInfoEvent is the exported agent-info-event message
type MsgAgentInfoEvent struct {
	AgentInfo MsgAgentInfo `cbor:"0,keyasint"`
}
