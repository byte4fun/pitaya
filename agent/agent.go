// Copyright (c) nano Author and TFG Co. All Rights Reserved.
//
// Permission is hereby granted, free of charge, to any person obtaining a copy
// of this software and associated documentation files (the "Software"), to deal
// in the Software without restriction, including without limitation the rights
// to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
// copies of the Software, and to permit persons to whom the Software is
// furnished to do so, subject to the following conditions:
//
// The above copyright notice and this permission notice shall be included in all
// copies or substantial portions of the Software.
//
// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
// FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
// AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
// LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
// OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
// SOFTWARE.

package agent

import (
	"context"
	gojson "encoding/json"
	e "errors"
	"fmt"
	"net"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	opentracing "github.com/opentracing/opentracing-go"

	"github.com/topfreegames/pitaya/v2/conn/codec"
	"github.com/topfreegames/pitaya/v2/conn/message"
	"github.com/topfreegames/pitaya/v2/conn/packet"
	"github.com/topfreegames/pitaya/v2/constants"
	pcontext "github.com/topfreegames/pitaya/v2/context"
	"github.com/topfreegames/pitaya/v2/errors"
	"github.com/topfreegames/pitaya/v2/logger"
	"github.com/topfreegames/pitaya/v2/metrics"
	"github.com/topfreegames/pitaya/v2/protos"
	"github.com/topfreegames/pitaya/v2/serialize"
	"github.com/topfreegames/pitaya/v2/session"
	"github.com/topfreegames/pitaya/v2/tracing"
	"github.com/topfreegames/pitaya/v2/util"
	"github.com/topfreegames/pitaya/v2/util/compression"
)

var (
	// hbd contains the heartbeat packet data
	hbd []byte
	// hrd contains the handshake response data
	hrd  []byte
	once sync.Once
)

const handlerType = "handler"

type (
	agentImpl struct {
		Session            session.Session // session
		sessionPool        session.SessionPool
		appDieChan         chan bool         // app die channel
		chDie              chan struct{}     // wait for close
		chOrder            chan pendingWrite // order message queue
		chSend             chan pendingWrite // push message queue
		chStopHeartbeat    chan struct{}     // stop heartbeats
		chStopWrite        chan struct{}     // stop writing messages
		chStopOrder        chan struct{}     // stop ordering messages
		closeMutex         sync.Mutex
		conn               net.Conn            // low-level conn fd
		decoder            codec.PacketDecoder // binary decoder
		encoder            codec.PacketEncoder // binary encoder
		heartbeatTimeout   time.Duration
		lastAt             int64 // last heartbeat unix time stamp
		messageEncoder     message.Encoder
		messagesBufferSize int // size of the pending messages buffer
		metricsReporters   []metrics.Reporter
		serializer         serialize.Serializer    // message serializer
		state              int32                   // current agent state
		curMsgID           uint                    // cur request msg id
		pushDelay          map[uint][]pendingWrite // push message delay
		pushDelayMID       uint                    // push message msg id
	}

	pendingMessage struct {
		ctx     context.Context
		typ     message.Type // message type
		route   string       // message route (push)
		mid     uint         // response message id (response)
		payload interface{}  // payload
		err     bool         // if its an error message
	}

	pendingWrite struct {
		ctx  context.Context
		data []byte
		err  error
		msg  *message.Message
	}

	// Agent corresponds to a user and is used for storing raw Conn information
	Agent interface {
		GetSession() session.Session
		Push(ctx context.Context, route string, v interface{}) error
		ResponseMID(ctx context.Context, mid uint, v interface{}, isError ...bool) error
		Close() error
		RemoteAddr() net.Addr
		String() string
		GetStatus() int32
		Kick(ctx context.Context) error
		SetLastAt()
		SetStatus(state int32)
		Handle()
		IPVersion() string
		SendHandshakeResponse() error
		SendRequest(ctx context.Context, serverID, route string, v interface{}) (*protos.Response, error)
		AnswerWithError(ctx context.Context, mid uint, err error)
	}

	// AgentFactory factory for creating Agent instances
	AgentFactory interface {
		CreateAgent(conn net.Conn) Agent
	}

	agentFactoryImpl struct {
		sessionPool        session.SessionPool
		appDieChan         chan bool           // app die channel
		decoder            codec.PacketDecoder // binary decoder
		encoder            codec.PacketEncoder // binary encoder
		heartbeatTimeout   time.Duration
		messageEncoder     message.Encoder
		messagesBufferSize int // size of the pending messages buffer
		metricsReporters   []metrics.Reporter
		serializer         serialize.Serializer // message serializer
	}
)

// NewAgentFactory ctor
func NewAgentFactory(
	appDieChan chan bool,
	decoder codec.PacketDecoder,
	encoder codec.PacketEncoder,
	serializer serialize.Serializer,
	heartbeatTimeout time.Duration,
	messageEncoder message.Encoder,
	messagesBufferSize int,
	sessionPool session.SessionPool,
	metricsReporters []metrics.Reporter,
) AgentFactory {
	return &agentFactoryImpl{
		appDieChan:         appDieChan,
		decoder:            decoder,
		encoder:            encoder,
		heartbeatTimeout:   heartbeatTimeout,
		messageEncoder:     messageEncoder,
		messagesBufferSize: messagesBufferSize,
		sessionPool:        sessionPool,
		metricsReporters:   metricsReporters,
		serializer:         serializer,
	}
}

// CreateAgent returns a new agent
func (f *agentFactoryImpl) CreateAgent(conn net.Conn) Agent {
	return newAgent(conn, f.decoder, f.encoder, f.serializer, f.heartbeatTimeout, f.messagesBufferSize, f.appDieChan, f.messageEncoder, f.metricsReporters, f.sessionPool)
}

// NewAgent create new agent instance
func newAgent(
	conn net.Conn,
	packetDecoder codec.PacketDecoder,
	packetEncoder codec.PacketEncoder,
	serializer serialize.Serializer,
	heartbeatTime time.Duration,
	messagesBufferSize int,
	dieChan chan bool,
	messageEncoder message.Encoder,
	metricsReporters []metrics.Reporter,
	sessionPool session.SessionPool,
) Agent {
	// initialize heartbeat and handshake data on first user connection
	serializerName := serializer.GetName()

	once.Do(func() {
		hbdEncode(heartbeatTime, packetEncoder, messageEncoder.IsCompressionEnabled(), serializerName)
	})

	a := &agentImpl{
		appDieChan:         dieChan,
		chDie:              make(chan struct{}),
		chSend:             make(chan pendingWrite, messagesBufferSize),
		chOrder:            make(chan pendingWrite, messagesBufferSize*10),
		chStopHeartbeat:    make(chan struct{}),
		chStopWrite:        make(chan struct{}),
		chStopOrder:        make(chan struct{}),
		pushDelay:          make(map[uint][]pendingWrite),
		messagesBufferSize: messagesBufferSize,
		conn:               conn,
		decoder:            packetDecoder,
		encoder:            packetEncoder,
		heartbeatTimeout:   heartbeatTime,
		lastAt:             time.Now().Unix(),
		serializer:         serializer,
		state:              constants.StatusStart,
		messageEncoder:     messageEncoder,
		metricsReporters:   metricsReporters,
		sessionPool:        sessionPool,
	}

	// binding session
	s := sessionPool.NewSession(a, true)
	metrics.ReportNumberOfConnectedClients(metricsReporters, sessionPool.GetSessionCount())
	a.Session = s
	return a
}

func (a *agentImpl) getMessageFromPendingMessage(pm pendingMessage) (*message.Message, error) {
	payload, err := util.SerializeOrRaw(a.serializer, pm.payload)
	if err != nil {
		payload, err = util.GetErrorPayload(a.serializer, err)
		if err != nil {
			return nil, err
		}
	}

	// construct message and encode
	m := &message.Message{
		Type:  pm.typ,
		Data:  payload,
		Route: pm.route,
		ID:    pm.mid,
		Err:   pm.err,
	}

	return m, nil
}

func (a *agentImpl) packetEncodeMessage(m *message.Message) ([]byte, error) {
	em, err := a.messageEncoder.Encode(m)
	if err != nil {
		return nil, err
	}

	// packet encode
	p, err := a.encoder.Encode(packet.Data, em)
	if err != nil {
		return nil, err
	}
	return p, nil
}

func (a *agentImpl) send(pendingMsg pendingMessage) (err error) {
	defer func() {
		if e := recover(); e != nil {
			err = errors.NewError(constants.ErrBrokenPipe, errors.ErrClientClosedRequest)
		}
	}()
	a.reportChannelSize()

	if chSendCapacity := a.messagesBufferSize - len(a.chSend); chSendCapacity == 0 {
		logger.Log.Warnf("the agent is will close as its network is busy, ID=%d, UID=%s",
			a.Session.ID(), a.Session.UID())
		a.Close()
		return nil
	}

	m, err := a.getMessageFromPendingMessage(pendingMsg)
	if err != nil {
		return err
	}

	// packet encode
	p, err := a.packetEncodeMessage(m)
	if err != nil {
		return err
	}

	pWrite := pendingWrite{
		ctx:  pendingMsg.ctx,
		data: p,
		msg:  m,
	}

	if pendingMsg.err {
		pWrite.err = util.GetErrorFromPayload(a.serializer, m.Data)
	}

	// chSend is never closed so we need this to don't block if agent is already closed
	select {
	case a.chOrder <- pWrite:
	case <-a.chDie:
	}
	return
}

// GetSession returns the agent session
func (a *agentImpl) GetSession() session.Session {
	return a.Session
}

// Push implementation for NetworkEntity interface
func (a *agentImpl) Push(ctx context.Context, route string, v interface{}) error {
	if a.GetStatus() == constants.StatusClosed {
		return errors.NewError(constants.ErrBrokenPipe, errors.ErrClientClosedRequest)
	}

	// switch d := v.(type) {
	// case []byte:
	// 	logger.Log.Debugf("Type=Push, ID=%d, UID=%s, Route=%s, Data=%dbytes",
	// 		a.Session.ID(), a.Session.UID(), route, len(d))
	// default:
	// 	logger.Log.Debugf("Type=Push, ID=%d, UID=%s, Route=%s, Data=%+v",
	// 		a.Session.ID(), a.Session.UID(), route, v)
	// }

	midVar := pcontext.GetRelationMsgIdFromContext(ctx, a.Session.UID())
	mid := uint(midVar)

	return a.send(pendingMessage{ctx: ctx, typ: message.Push, route: route, payload: v, mid: mid})
}

// ResponseMID implementation for NetworkEntity interface
// Respond message to session
func (a *agentImpl) ResponseMID(ctx context.Context, mid uint, v interface{}, isError ...bool) error {
	err := false
	if len(isError) > 0 {
		err = isError[0]
	}
	if a.GetStatus() == constants.StatusClosed {
		return errors.NewError(constants.ErrBrokenPipe, errors.ErrClientClosedRequest)
	}

	if mid <= 0 {
		return constants.ErrSessionOnNotify
	}

	route := pcontext.GetFromPropagateCtx(ctx, constants.RouteKey)

	// switch d := v.(type) {
	// case []byte:
	// 	logger.Log.Debugf("Type=Response, ID=%d, UID=%s, MID=%d, Route=%s, Data=%dbytes",
	// 		a.Session.ID(), a.Session.UID(), mid, route, len(d))
	// default:
	// 	logger.Log.Infof("Type=Response, ID=%d, UID=%s, MID=%d, Route=%s, Data=%+v",
	// 		a.Session.ID(), a.Session.UID(), mid, v)
	// }

	return a.send(pendingMessage{ctx: ctx, typ: message.Response, route: route.(string), mid: mid, payload: v, err: err})
}

// Close closes the agent, cleans inner state and closes low-level connection.
// Any blocked Read or Write operations will be unblocked and return errors.
func (a *agentImpl) Close() error {
	a.closeMutex.Lock()
	defer a.closeMutex.Unlock()
	if a.GetStatus() == constants.StatusClosed {
		return constants.ErrCloseClosedSession
	}
	a.SetStatus(constants.StatusClosed)

	logger.Log.Debugf("Session closed, ID=%d, UID=%s, IP=%s",
		a.Session.ID(), a.Session.UID(), a.conn.RemoteAddr())

	// prevent closing closed channel
	select {
	case <-a.chDie:
		// expect
	default:
		close(a.chStopWrite)
		close(a.chStopOrder)
		close(a.chStopHeartbeat)
		close(a.chDie)
		a.onSessionClosed(a.Session)
	}

	metrics.ReportNumberOfConnectedClients(a.metricsReporters, a.sessionPool.GetSessionCount())

	return a.conn.Close()
}

// RemoteAddr implementation for NetworkEntity interface
// returns the remote network address.
func (a *agentImpl) RemoteAddr() net.Addr {
	return a.conn.RemoteAddr()
}

// String, implementation for Stringer interface
func (a *agentImpl) String() string {
	return fmt.Sprintf("Remote=%s, LastTime=%d", a.conn.RemoteAddr().String(), atomic.LoadInt64(&a.lastAt))
}

// GetStatus gets the status
func (a *agentImpl) GetStatus() int32 {
	return atomic.LoadInt32(&a.state)
}

// Kick sends a kick packet to a client
func (a *agentImpl) Kick(ctx context.Context) error {
	midVar := pcontext.GetRelationMsgIdFromContext(ctx, a.Session.UID())
	mid := uint(midVar)

	if a.GetStatus() == constants.StatusClosed {
		logger.Log.Debugf("can't send kick, session has closed, SessionID=%d, UID=%s", a.Session.ID(), a.Session.UID())
		return nil
	}

	fn := func() error {
		// packet encode
		p, err := a.encoder.Encode(packet.Kick, nil)
		if err != nil {
			return err
		}
		_, err = a.conn.Write(p)
		return err
	}

	if mid > 0 {
		t := time.NewTicker(time.Millisecond)
		for a.curMsgID <= mid {
			select {
			case <-t.C:
			}
		}
		t.Stop()
		return fn()
	}

	return fn()
}

// SetLastAt sets the last at to now
func (a *agentImpl) SetLastAt() {
	atomic.StoreInt64(&a.lastAt, time.Now().Unix())
}

// SetStatus sets the agent status
func (a *agentImpl) SetStatus(state int32) {
	atomic.StoreInt32(&a.state, state)
}

// Handle handles the messages from and to a client
func (a *agentImpl) Handle() {
	defer func() {
		a.Close()
		logger.Log.Debugf("Session handle goroutine exit, SessionID=%d, UID=%s", a.Session.ID(), a.Session.UID())
	}()

	go a.write()
	go a.ordered()
	go a.heartbeat()
	<-a.chDie // agent closed signal
}

// IPVersion returns the remote address ip version.
// net.TCPAddr and net.UDPAddr implementations of String()
// always construct result as <ip>:<port> on both
// ipv4 and ipv6. Also, to see if the ip is ipv6 they both
// check if there is a colon on the string.
// So checking if there are more than one colon here is safe.
func (a *agentImpl) IPVersion() string {
	version := constants.IPv4

	ipPort := a.RemoteAddr().String()
	if strings.Count(ipPort, ":") > 1 {
		version = constants.IPv6
	}

	return version
}

func (a *agentImpl) heartbeat() {
	ticker := time.NewTicker(a.heartbeatTimeout)

	defer func() {
		ticker.Stop()
		a.Close()
	}()

	for {
		select {
		case <-ticker.C:
			deadline := time.Now().Add(-2 * a.heartbeatTimeout).Unix()
			if atomic.LoadInt64(&a.lastAt) < deadline {
				logger.Log.Debugf("Session heartbeat timeout, LastTime=%d, Deadline=%d", atomic.LoadInt64(&a.lastAt), deadline)
				return
			}

			// chSend is never closed so we need this to don't block if agent is already closed
			select {
			case a.chSend <- pendingWrite{data: hbd}:
			case <-a.chDie:
				return
			case <-a.chStopHeartbeat:
				return
			}
		case <-a.chDie:
			return
		case <-a.chStopHeartbeat:
			return
		}
	}
}

func (a *agentImpl) onSessionClosed(s session.Session) {
	defer func() {
		if err := recover(); err != nil {
			logger.Log.Errorf("pitaya/onSessionClosed: %v", err)
		}
	}()

	for _, fn1 := range s.GetOnCloseCallbacks() {
		fn1()
	}

	for _, fn2 := range a.sessionPool.GetSessionCloseCallbacks() {
		fn2(s)
	}
}

// SendHandshakeResponse sends a handshake response
func (a *agentImpl) SendHandshakeResponse() error {
	_, err := a.conn.Write(hrd)
	return err
}

func (a *agentImpl) write() {
	// clean func
	defer func() {
		a.Close()
	}()

	for {
		select {
		case pWrite := <-a.chSend:
			// close agent if low-level Conn broken
			if _, err := a.conn.Write(pWrite.data); err != nil {
				tracing.FinishSpan(pWrite.ctx, err)
				metrics.ReportTimingFromCtx(pWrite.ctx, a.metricsReporters, handlerType, err)
				logger.Log.Errorf("Failed to write in conn: %s", err.Error())
				return
			}
			var e error
			tracing.FinishSpan(pWrite.ctx, e)
			metrics.ReportTimingFromCtx(pWrite.ctx, a.metricsReporters, handlerType, pWrite.err)
		case <-a.chStopWrite:
			return
		}
	}
}

// SendRequest sends a request to a server
func (a *agentImpl) SendRequest(ctx context.Context, serverID, route string, v interface{}) (*protos.Response, error) {
	return nil, e.New("not implemented")
}

// AnswerWithError answers with an error
func (a *agentImpl) AnswerWithError(ctx context.Context, mid uint, err error) {
	var e error
	defer func() {
		if e != nil {
			tracing.FinishSpan(ctx, e)
			metrics.ReportTimingFromCtx(ctx, a.metricsReporters, handlerType, e)
		}
	}()
	if ctx != nil && err != nil {
		s := opentracing.SpanFromContext(ctx)
		if s != nil {
			tracing.LogError(s, err.Error())
		}
	}
	p, e := util.GetErrorPayload(a.serializer, err)
	if e != nil {
		logger.Log.Errorf("error answering the user with an error: %s", e.Error())
		return
	}
	e = a.Session.ResponseMID(ctx, mid, p, true)
	if e != nil {
		logger.Log.Errorf("error answering the user with an error: %s", e.Error())
	}
}

func hbdEncode(heartbeatTimeout time.Duration, packetEncoder codec.PacketEncoder, dataCompression bool, serializerName string) {
	hData := map[string]interface{}{
		"code": 200,
		"sys": map[string]interface{}{
			"heartbeat":  heartbeatTimeout.Seconds(),
			"dict":       message.GetDictionary(),
			"serializer": serializerName,
		},
	}
	data, err := gojson.Marshal(hData)
	if err != nil {
		panic(err)
	}

	if dataCompression {
		compressedData, err := compression.DeflateData(data)
		if err != nil {
			panic(err)
		}

		if len(compressedData) < len(data) {
			data = compressedData
		}
	}

	hrd, err = packetEncoder.Encode(packet.Handshake, data)
	if err != nil {
		panic(err)
	}

	hbd, err = packetEncoder.Encode(packet.Heartbeat, nil)
	if err != nil {
		panic(err)
	}
}

func (a *agentImpl) reportChannelSize() {
	chSendCapacity := a.messagesBufferSize - len(a.chSend)
	if chSendCapacity == 0 {
		logger.Log.Warnf("chSend is at maximum capacity, ID=%d, UID=%s", a.Session.ID(), a.Session.UID())
	}
	for _, mr := range a.metricsReporters {
		if err := mr.ReportGauge(metrics.ChannelCapacity, map[string]string{"channel": "agent_chsend"}, float64(chSendCapacity)); err != nil {
			logger.Log.Warnf("failed to report chSend channel capaacity: %s", err.Error())
		}
	}
}

func (a *agentImpl) ordered() {
	// clean func
	defer func() {
		a.Close()
	}()

	send := func(pWrite pendingWrite) {
		select {
		case a.chSend <- pWrite:
		case <-a.chDie:
		}

		msg := pWrite.msg
		if msg.Type == message.Push {
			logger.Log.Debugf("Type=RPush, ID=%d, UID=%s, RID=%d Route=%s, Data=%dbytes",
				a.Session.ID(), a.Session.UID(), msg.ID, msg.Route, len(msg.Data))

		} else if msg.Type == message.Response {
			logger.Log.Debugf("Type=RResponse, ID=%d, UID=%s, MID=%d, Route=%s, Data=%dbytes",
				a.Session.ID(), a.Session.UID(), msg.ID, msg.Route, len(msg.Data))
		}
	}
	for {
		select {
		case pWrite := <-a.chOrder:
			m := pWrite.msg

			if m.Type == message.Push && m.ID > 0 && m.ID > a.curMsgID {
				tID := m.ID
				a.pushDelay[tID] = append(a.pushDelay[tID], pWrite)
				a.pushDelayMID = tID
				logger.Log.Debugf("freeze push msg, ID=%d, UID=%s, relation.id=%v route=%s",
					a.Session.ID(), a.Session.UID(), tID, m.Route)
				continue
			}

			if m.Type == message.Push && a.pushDelayMID > 0 {
				tID := a.pushDelayMID
				a.pushDelay[tID] = append(a.pushDelay[tID], pWrite)
				logger.Log.Debugf("freeze push msg, ID=%d, UID=%s, relation.id=%v route=%s len=%d",
					a.Session.ID(), a.Session.UID(), tID, m.Route, len(a.pushDelay[tID]))
				continue
			}

			send(pWrite)

			if m.Type == message.Response {
				a.curMsgID = m.ID
				if val, ok := a.pushDelay[m.ID]; ok {
					logger.Log.Debugf("restore push msg, ID=%d, UID=%s, relation.id=%v len=%d",
						a.Session.ID(), a.Session.UID(), a.pushDelayMID, len(a.pushDelay[m.ID]))
					for _, v := range val {
						logger.Log.Debugf("restore push msg, ID=%d, UID=%s, relation.id=%v route=%s",
							a.Session.ID(), a.Session.UID(), m.ID, v.msg.Route)
						send(v)
					}
					delete(a.pushDelay, m.ID)
					a.pushDelayMID = 0
				}

				if len(a.pushDelay) > 0 {
					keys := make([]uint, 0, len(a.pushDelay))
					for key := range a.pushDelay {
						keys = append(keys, key)
					}
					sort.Slice(keys, func(i, j int) bool {
						return keys[i] < keys[j]
					})
					for _, id := range keys {
						if id >= a.curMsgID {
							break
						}

						if val, ok := a.pushDelay[id]; ok {
							logger.Log.Debugf("restore old push msg, ID=%d, UID=%s, relation.id=%v len=%d",
								a.Session.ID(), a.Session.UID(), a.pushDelayMID, len(a.pushDelay[id]))
							for _, v := range val {
								logger.Log.Debugf("restore old push msg, ID=%d, UID=%s, relation.id=%v route=%s",
									a.Session.ID(), a.Session.UID(), id, v.msg.Route)
								send(v)
							}
							delete(a.pushDelay, id)
							a.pushDelayMID = 0
						}
					}
				}
			}
		case <-a.chStopOrder:
			return
		}
	}
}
