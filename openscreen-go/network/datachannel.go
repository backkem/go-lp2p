package ospc

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"
)

// MaxDataChannelID is the largest channel-id (LP2P "Data channel extension":
// "between 0 and 65,534").
const MaxDataChannelID = 65534

// DataChannelOpenTimeout bounds how long OpenDataChannel waits for the
// data-channel-open-response when the context has no earlier deadline.
var DataChannelOpenTimeout = 10 * time.Second

var (
	// ErrDataChannelIDOutOfRange is returned when an explicit channel ID is
	// above MaxDataChannelID, or no ID is free.
	ErrDataChannelIDOutOfRange = errors.New("data channel ID out of range")
	// ErrDataChannelIDInUse is returned when an explicit channel ID is used
	// by another open data channel of the connection.
	ErrDataChannelIDInUse = errors.New("data channel ID in use")
	// ErrDataChannelIDParity is returned when an explicit channel ID has
	// the parity the peer allocates from.
	ErrDataChannelIDParity = errors.New("data channel ID has the peer's parity")
	// ErrDataChannelRejected is returned when the peer answers the
	// data-channel-open-request with a result other than success.
	ErrDataChannelRejected = errors.New("data channel rejected by the peer")
)

// DataChannelParameters describes an open data channel.
type DataChannelParameters struct {
	Label    string
	Protocol string
	ID       uint64
}

// DataChannelInit configures a data channel to open.
type DataChannelInit struct {
	Label    string
	Protocol string
	// ID is the channel-id to use. If nil, one is allocated: the QUIC
	// client allocates even IDs and the QUIC server odd IDs, taking the
	// lowest one of its parity not used by an open data channel of the
	// connection. An explicit ID must be free and of the local parity, so
	// the two sides never pick the same ID. IDs only label channels;
	// channels are told apart by their QUIC stream, so IDs from the peer
	// are accepted whatever their parity, duplicates included.
	ID *uint64
}

// OpenDataChannel opens a data channel. It returns once the peer accepted
// it with a data-channel-open-response.
func (c *Connection) OpenDataChannel(ctx context.Context, init DataChannelInit) (*DataChannel, error) {
	return c.base.OpenDataChannel(ctx, init)
}

// AcceptDataChannel accepts a data channel opened by the peer.
func (c *Connection) AcceptDataChannel(ctx context.Context) (*DataChannel, error) {
	return c.base.AcceptDataChannel(ctx)
}

// dataChannelIDs tracks the channel IDs in use on a connection.
type dataChannelIDs struct {
	mu   sync.Mutex
	used map[uint64]int // ID -> number of open channels using it
}

// reserve takes id, or the lowest free ID of the local parity if id is nil.
// parity is 0 (even) on the QUIC client and 1 (odd) on the QUIC server.
func (d *dataChannelIDs) reserve(id *uint64, parity uint64) (uint64, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.used == nil {
		d.used = make(map[uint64]int)
	}
	if id != nil {
		if *id > MaxDataChannelID {
			return 0, fmt.Errorf("%w: %d", ErrDataChannelIDOutOfRange, *id)
		}
		if *id%2 != parity {
			return 0, fmt.Errorf("%w: %d", ErrDataChannelIDParity, *id)
		}
		if d.used[*id] > 0 {
			return 0, fmt.Errorf("%w: %d", ErrDataChannelIDInUse, *id)
		}
		d.used[*id]++
		return *id, nil
	}
	for i := parity; i <= MaxDataChannelID; i += 2 {
		if d.used[i] == 0 {
			d.used[i]++
			return i, nil
		}
	}
	return 0, fmt.Errorf("%w: no free ID", ErrDataChannelIDOutOfRange)
}

// add records a channel opened by the peer.
func (d *dataChannelIDs) add(id uint64) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.used == nil {
		d.used = make(map[uint64]int)
	}
	d.used[id]++
}

func (d *dataChannelIDs) release(id uint64) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.used[id] <= 1 {
		delete(d.used, id)
		return
	}
	d.used[id]--
}

func (c *baseConnection) OpenDataChannel(ctx context.Context, init DataChannelInit) (*DataChannel, error) {
	ids := &c.app.Load().dataChannelIDs
	parity := uint64(0)
	if c.agentRole == AgentRoleServer {
		parity = 1
	}
	id, err := ids.reserve(init.ID, parity)
	if err != nil {
		return nil, err
	}
	dc, err := c.openDataChannel(ctx, id, init)
	if err != nil {
		ids.release(id)
		return nil, err
	}
	return dc, nil
}

func (c *baseConnection) openDataChannel(ctx context.Context, id uint64, init DataChannelInit) (*DataChannel, error) {
	ctx, cancel := context.WithTimeout(ctx, DataChannelOpenTimeout)
	defer cancel()
	// Stop waiting when the connection closes, too.
	defer context.AfterFunc(c.ctx, cancel)()

	req := &msgDataChannelOpenRequest{
		msgRequest: msgRequest{
			RequestId: msgRequestId(c.nextRequestID()),
		},
		ChannelId: id,
		Label:     init.Label,
		Protocol:  init.Protocol,
	}
	stream, err := c.openStream(ctx, req)
	if err != nil {
		return nil, err
	}

	if err := awaitDataChannelOpenResponse(ctx, stream, req.RequestId); err != nil {
		abortStream(stream)
		if c.ctx.Err() != nil {
			return nil, c.err()
		}
		return nil, err
	}
	c.log.Debug("receive", "msg", "msgDataChannelOpenResponse")

	return &DataChannel{
		DataChannelParameters: DataChannelParameters{
			Label:    init.Label,
			Protocol: init.Protocol,
			ID:       id,
		},
		stream: stream,
		ids:    &c.app.Load().dataChannelIDs,
	}, nil
}

// awaitDataChannelOpenResponse waits for the data-channel-open-response to
// request reqID. The peer sends it on the request's (bidirectional) stream
// before any data-frame. The caller aborts the stream on error, which also
// ends the read left behind when ctx is done.
func awaitDataChannelOpenResponse(ctx context.Context, stream ApplicationStream, reqID msgRequestId) error {
	type result struct {
		msg interface{}
		err error
	}
	ch := make(chan result, 1)
	go func() {
		msg, err := readMessage(stream)
		ch <- result{msg, err}
	}()
	var res result
	select {
	case res = <-ch:
	case <-ctx.Done():
		return fmt.Errorf("awaiting data-channel-open-response: %w", ctx.Err())
	}
	if res.err != nil {
		return fmt.Errorf("awaiting data-channel-open-response: %w", res.err)
	}
	resp, ok := res.msg.(*msgDataChannelOpenResponse)
	if !ok {
		return fmt.Errorf("awaiting data-channel-open-response: got %s", msgName(res.msg))
	}
	if resp.RequestId != reqID {
		return fmt.Errorf("data-channel-open-response to request %d, want %d", resp.RequestId, reqID)
	}
	if resp.Result != ResultSuccess {
		// The spec defines no failure codes for data channels; any result
		// other than success fails the open.
		return fmt.Errorf("%w: result %d", ErrDataChannelRejected, resp.Result)
	}
	return nil
}

// acceptDataChannelRequest answers a data-channel-open-request received on
// stream s and hands the channel to AcceptDataChannel.
func (c *baseConnection) acceptDataChannelRequest(req *msgDataChannelOpenRequest, s ApplicationStream) {
	app := c.app.Load()
	resp := &msgDataChannelOpenResponse{
		msgResponse: msgResponse{RequestId: req.RequestId},
		Result:      ResultSuccess,
	}
	if req.ChannelId > MaxDataChannelID {
		// The spec defines no failure codes; use permanent-error, as
		// openscreen does.
		resp.Result = ResultPermanentError
	}
	c.log.Debug("send", "msg", msgName(resp), "result", resp.Result)
	// The response goes out before the channel is handed over, so it
	// precedes any data-frame we send.
	if err := writeMessage(resp, s); err != nil || resp.Result != ResultSuccess {
		if err != nil {
			c.log.Debug("data channel: failed to send open response", "err", err)
		}
		_ = s.Close()
		return
	}

	app.dataChannelIDs.add(req.ChannelId)
	dc := &DataChannel{
		DataChannelParameters: DataChannelParameters{
			Label:    req.Label,
			ID:       req.ChannelId,
			Protocol: req.Protocol,
		},
		stream: s,
		ids:    &app.dataChannelIDs,
	}
	deliver(c, app.acceptDataChannel, dc, dc.Close)
}

// abortStream aborts both directions of a stream, unblocking pending reads.
func abortStream(s ApplicationStream) {
	if a, ok := s.(interface{ cancel() }); ok {
		a.cancel()
		return
	}
	_ = s.Close()
}

func (c *baseConnection) AcceptDataChannel(ctx context.Context) (*DataChannel, error) {
	return accept(ctx, c, c.app.Load().acceptDataChannel)
}

// DataChannel is an open data channel.
type DataChannel struct {
	DataChannelParameters
	stream    ApplicationStream
	ids       *dataChannelIDs
	closeOnce sync.Once
}

// SendMessage
func (c *DataChannel) SendMessage(payload []byte) error {
	return c.SendMessageWithEncoding(payload, DataEncodingBinary)
}

// SendMessageWithEncoding
func (c *DataChannel) SendMessageWithEncoding(payload []byte, enc DataEncoding) error {
	msg := &msgDataFrame{
		EncodingId: uint64(enc),
		Payload:    payload,
	}

	return writeMessage(msg, c.stream)
}

// ReceiveMessage
func (c *DataChannel) ReceiveMessage() ([]byte, error) {
	b, _, err := c.ReceiveMessageWithEncoding()
	return b, err
}

// ReceiveMessageWithEncoding
func (c *DataChannel) ReceiveMessageWithEncoding() ([]byte, DataEncoding, error) {
	msg, err := readMessage(c.stream)
	if err != nil {
		return nil, 0, err
	}

	dataFrame, ok := msg.(*msgDataFrame)
	if !ok {
		return nil, 0, fmt.Errorf("unexpected message type: %T", msg)
	}

	return dataFrame.Payload, DataEncoding(dataFrame.EncodingId), nil
}

// Read reads a packet of len(p) bytes as binary data
func (c *DataChannel) Read(p []byte) (int, error) {
	n, _, err := c.ReadDataChannel(p)
	return n, err
}

// ReadDataChannel reads a packet of len(p) bytes
func (c *DataChannel) ReadDataChannel(p []byte) (int, DataEncoding, error) {
	msg, err := readMessage(c.stream)
	if err != nil {
		return 0, 0, err
	}

	dataFrame, ok := msg.(*msgDataFrame)
	if !ok {
		return 0, 0, fmt.Errorf("unexpected message type")
	}

	payload := dataFrame.Payload

	n := copy(p, payload)
	return n, DataEncoding(dataFrame.EncodingId), nil
}

// Write writes len(p) bytes from p as binary data
func (c *DataChannel) Write(p []byte) (n int, err error) {
	return c.WriteDataChannel(p, DataEncodingBinary)
}

// WriteDataChannel writes len(p) bytes from p
func (c *DataChannel) WriteDataChannel(p []byte, enc DataEncoding) (n int, err error) {
	msg := &msgDataFrame{
		EncodingId: uint64(enc),
		Payload:    p,
	}

	err = writeMessage(msg, c.stream)
	if err != nil {
		return 0, err
	}

	return len(p), nil
}

// Close closes the DataChannel and the underlying Quic stream.
// The channel's ID is free for reuse from then on.
func (c *DataChannel) Close() error {
	c.closeOnce.Do(func() { c.ids.release(c.ID) })
	return c.stream.Close()
}
