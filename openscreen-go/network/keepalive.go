package ospc

import (
	"fmt"
	"sync"
	"sync/atomic"
	"time"
)

const (
	// DefaultIdleTimeout is the recommended QUIC idle_timeout transport
	// parameter.
	DefaultIdleTimeout = 25 * time.Second
	// DefaultKeepAliveInterval is the default time without sending any message
	// after which an agent-status-request is sent. It has to be less than the
	// idle timeout; half of it leaves room for loss and round trips.
	DefaultKeepAliveInterval = DefaultIdleTimeout / 2
)

// KeepAliveConfig configures the OSP keep-alive mechanism: an agent behaves
// as though a timer, shorter than the idle timeout, is reset every time a
// message is sent. When the timer expires an agent-status-request is sent.
// QUIC PING frames are not used.
type KeepAliveConfig struct {
	// Disabled stops this agent from sending agent-status-request messages.
	// Incoming agent-status-request messages are still answered.
	Disabled bool
	// Interval overrides DefaultKeepAliveInterval when non-zero.
	Interval time.Duration
}

func (c KeepAliveConfig) interval() time.Duration {
	if c.Disabled {
		return 0
	}
	if c.Interval > 0 {
		return c.Interval
	}
	return DefaultKeepAliveInterval
}

// keepAlive sends agent-status-request messages when the connection has been
// idle for too long and answers incoming agent-status-request messages. It is
// owned by the transport connection so that it outlives the hand-off from the
// network to the application protocol.
type keepAlive struct {
	interval time.Duration // Zero when sending keep-alives is disabled.
	send     func(msg interface{}) error

	mu        sync.Mutex // Protects state below
	timer     *time.Timer
	stopped   bool
	requestID uint64
	nextID    func() uint64 // Optional request ID source, see setRequestIDSource.
	pending   map[msgRequestId]struct{}

	// Counters of handled keep-alive traffic.
	requestsReceived  atomic.Uint64
	responsesReceived atomic.Uint64
}

func newKeepAlive(conf KeepAliveConfig, send func(msg interface{}) error) *keepAlive {
	return &keepAlive{
		interval: conf.interval(),
		send:     send,
		pending:  make(map[msgRequestId]struct{}),
	}
}

// setRequestIDSource makes keep-alive requests draw their IDs from next, so
// they don't collide with other requests sent to the same peer.
func (k *keepAlive) setRequestIDSource(next func() uint64) {
	k.mu.Lock()
	defer k.mu.Unlock()

	k.nextID = next
}

// start arms the keep-alive timer.
func (k *keepAlive) start() {
	k.mu.Lock()
	defer k.mu.Unlock()

	if k.interval == 0 || k.stopped || k.timer != nil {
		return
	}
	k.timer = time.AfterFunc(k.interval, k.fire)
}

// reset should be called every time a message is sent.
func (k *keepAlive) reset() {
	k.mu.Lock()
	defer k.mu.Unlock()

	if k.timer == nil || k.stopped {
		return
	}
	k.timer.Reset(k.interval)
}

func (k *keepAlive) stop() {
	k.mu.Lock()
	defer k.mu.Unlock()

	k.stopped = true
	if k.timer != nil {
		k.timer.Stop()
	}
}

func (k *keepAlive) fire() {
	k.mu.Lock()
	if k.stopped {
		k.mu.Unlock()
		return
	}
	var id msgRequestId
	if k.nextID != nil {
		id = msgRequestId(k.nextID())
	} else {
		k.requestID++
		id = msgRequestId(k.requestID)
	}
	k.pending[id] = struct{}{}
	k.mu.Unlock()

	// Sending a message resets the timer.
	err := k.send(&msgAgentStatusRequest{
		msgRequest: msgRequest{RequestId: id},
	})
	if err != nil {
		fmt.Printf("keep-alive: failed to send agent-status-request: %v\n", err)
		k.stop()
	}
}

// handleMessage processes agent-status messages. It reports whether msg was
// consumed.
func (k *keepAlive) handleMessage(msg interface{}) bool {
	switch typedMsg := msg.(type) {
	case *msgAgentStatusRequest:
		k.requestsReceived.Add(1)
		err := k.send(&msgAgentStatusResponse{
			msgResponse: msgResponse{RequestId: typedMsg.RequestId},
		})
		if err != nil {
			fmt.Printf("keep-alive: failed to send agent-status-response: %v\n", err)
		}
		return true

	case *msgAgentStatusResponse:
		k.mu.Lock()
		_, ok := k.pending[typedMsg.RequestId]
		delete(k.pending, typedMsg.RequestId)
		k.mu.Unlock()
		if !ok {
			fmt.Printf("keep-alive: ignoring unsolicited agent-status-response %d\n", typedMsg.RequestId)
		}
		k.responsesReceived.Add(1)
		return true
	}
	return false
}
