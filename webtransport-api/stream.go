package webtransport

import (
	"errors"

	"github.com/backkem/go-lp2p/streams-api"
)

type WebTransportBidirectionalStream struct {
	Readable WebTransportReceiveStream
	Writable WebTransportSendStream
}

func NewWebTransportBidirectionalStream(s Stream) WebTransportBidirectionalStream {
	return WebTransportBidirectionalStream{
		Readable: NewWebTransportReceiveStream(s),
		Writable: NewWebTransportSendStream(s),
	}
}

type WebTransportReceiveStream struct {
	streams.ReadableStream[[]byte]
	inner Stream
}

func NewWebTransportReceiveStream(s Stream) WebTransportReceiveStream {
	return WebTransportReceiveStream{
		ReadableStream: streams.NewDataReadableStream(s),
		inner:          s,
	}
}

type WebTransportReceiveStreamStats struct {
	BytesReceived uint64
	BytesRead     uint64
}

func (s WebTransportReceiveStream) GetStats() (WebTransportReceiveStreamStats, error) {
	return WebTransportReceiveStreamStats{}, errors.New("unimplemented")
}

type WebTransportSendStream struct {
	streams.WritableStream[[]byte]
	inner Stream
}

func NewWebTransportSendStream(s Stream) WebTransportSendStream {
	return WebTransportSendStream{
		WritableStream: streams.NewDataWritableStream(s),
		inner:          s,
	}
}

func (s WebTransportSendStream) GetStats() (WebTransportSendStreamStats, error) {
	return WebTransportSendStreamStats{}, errors.New("unimplemented")
}

type WebTransportSendStreamStats struct {
	BytesWritten      uint64
	BytesSent         uint64
	BytesAcknowledged uint64
}
