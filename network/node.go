// Copyright (c) 2017 The Alvalor Authors
//
// This file is part of Alvalor.
//
// Alvalor is free software: you can redistribute it and/or modify
// it under the terms of the GNU Affero General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// Alvalor is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
// GNU Affero General Public License for more details.
//
// You should have received a copy of the GNU Affero General Public License
// along with Alvalor.  If not, see <http://www.gnu.org/licenses/>.

package network

import (
	"bytes"
	"fmt"
	"math/rand"
	"net"
	"reflect"
	"sync/atomic"
	"time"

	"github.com/pierrec/lz4"
	"github.com/pkg/errors"
	uuid "github.com/satori/go.uuid"
	"go.uber.org/zap"
)

// Node represents our own node on the peer-to-peer network. It manages the peers we are connected
// to, as well as all the network parameters. Finally, it allows a subscriber to receive & process
// all messages that are not internally handled by the network library.
type Node struct {
	nonce      []byte
	network    []byte
	log        *zap.Logger
	book       Book
	codec      Codec
	subscriber chan<- interface{}
	server     bool
	address    string
	minPeers   uint
	maxPeers   uint
	balance    time.Duration
	heartbeat  time.Duration
	timeout    time.Duration
	discovery  *time.Ticker
	count      int32
	peers      *registry
}

// NewNode creates a new node to connect to the peer-to-peer network. Without parameters, it will
// use the default configuration, but it takes a variadic list of configuration functions to
// punctually change desired parameters and dependencies. It launches a go routine for balancing the
// number of peers, for managing incoming messages and (if enabled) for the server listening for
// incoming connections.
func NewNode(options ...func(*Config)) *Node {
	cfg := DefaultConfig
	for _, option := range options {
		option(&cfg)
	}
	nonce := uuid.NewV4().Bytes()
	node := &Node{
		nonce:      nonce,
		network:    cfg.network,
		log:        cfg.log,
		book:       cfg.book,
		codec:      cfg.codec,
		subscriber: cfg.subscriber,
		server:     cfg.server,
		address:    cfg.address,
		minPeers:   cfg.minPeers,
		maxPeers:   cfg.maxPeers,
		balance:    cfg.balance,
		heartbeat:  cfg.heartbeat,
		timeout:    cfg.timeout,
		discovery:  time.NewTicker(cfg.discovery),
		peers:      &registry{peers: make(map[string]*peer)},
	}

	incoming := &Incoming{
		address:   cfg.address,
		codec:     cfg.codec,
		heartbeat: cfg.heartbeat,
		log:       cfg.log,
		network:   cfg.network,
		nonce:     nonce,
		timeout:   cfg.timeout,
		onConnected: func(p peer) { node.onConnected(p) },
        onConnecting: func() { node.onConnecting() },
        acceptConnection: func(nonce []byte) bool { return node.peers.count() > int(node.maxPeers) && !node.known(nonce) },
        onError: func(conn net.Conn) { node.drop(conn) },
	}
	node.book.Blacklist(cfg.address)
	if cfg.server {
		go incoming.listen()
	}
	go node.check()
	go node.manage()
	return node
}

func (node *Node) onConnected(p peer) {
	node.peers.add(p.addr, &p)
	node.book.Connected(p.addr)
	go p.receive()
	if node.server {
		err := node.share(p.addr, []string{node.address})
		if err != nil {
			node.log.Error("could not share initial address", zap.Error(err))
		}
	}
	e := Connected{
		Address: p.addr,
	}
	node.event(&e)
}

func (node *Node) onConnecting() {
	atomic.AddInt32(&node.count, 1)
}

// manage will build a list of two incoming channels per peer: one for the heartbeating and one for
// incoming messages. It will then keep receiving these messages and processing them accordingly,
// unless a channel is closed and we need to remove a peer from the list of cases.
func (node *Node) manage() {
Outer:
	for {
		peers := node.peers.slice()
		var cases []reflect.SelectCase
		for _, peer := range peers {
			peers = append(peers, peer)
			submitter := reflect.SelectCase{Dir: reflect.SelectRecv, Chan: reflect.ValueOf(peer.incoming)}
			cases = append(cases, submitter)
		}
		for _, peer := range peers {
			heartbeater := reflect.SelectCase{Dir: reflect.SelectRecv, Chan: reflect.ValueOf(peer.hb.C)}
			cases = append(cases, heartbeater)
		}
		if len(cases) == 0 {
			time.Sleep(time.Millisecond * 100)
			continue
		}
		for {

			// if a channel was closed, we should disconnect that peer
			i, val, ok := reflect.Select(cases)
			if !ok {
				node.disconnect(peers[i])
				continue Outer
			}

			// if we received a time struct, we should send a heartbeat
			_, ok = val.Interface().(time.Time)
			if ok {
				node.ping(peers[i%len(peers)])
				continue
			}

			// otherwise, we should process a received network message
			msg := Message{
				Address: peers[i].addr,
				Value:   i,
			}
			node.process(&msg)
		}
	}
}

// ping will send a ping message to the given peer.
func (node *Node) ping(peer *peer) {
	node.log.Debug("pinging peer", zap.String("address", peer.addr))
	ping := Ping{
		Nonce: rand.Uint32(),
	}
	err := node.Send(peer.addr, &ping)
	if err != nil {
		node.log.Error("could not send ping", zap.String("address", peer.addr), zap.Error(err))
	}
}

// disconnect will disconnect from the given peer and notify the subscriber that we are no longer
// connected to it.
func (node *Node) disconnect(peer *peer) {
	node.log.Info("disconnecting peer", zap.String("address", peer.addr))
	node.peers.remove(peer.addr)
	peer.close()
	node.book.Dropped(peer.addr)
	atomic.AddInt32(&node.count, -1)
	e := Disconnected{
		Address: peer.addr,
	}
	node.event(&e)
}

// process will process a given incoming message according to the message type. If it is not handled
// explicitly by our library, we send it up the stack to the subscriber.
func (node *Node) process(msg *Message) {
	node.log.Debug("processing message", zap.String("type", fmt.Sprintf("%T", msg.Value)), zap.String("address", msg.Address))
	var err error
	switch msg.Value.(type) {
	case *Ping:
		err = node.processPing(msg)
	case *Pong:
		err = node.processPong(msg)
	case *Discover:
		err = node.processDiscover(msg)
	case *Peers:
		err = node.processPeers(msg)
	default:
		node.event(msg)
	}
	if err != nil {
		node.log.Error("could not process message", zap.String("type", fmt.Sprintf("%T", msg.Value)), zap.Error(err))
	}
}

// processPing will process a ping message received on the network by replying with a pong.
func (node *Node) processPing(msg *Message) error {
	ping := msg.Value.(*Ping)
	pong := Pong{
		Nonce: ping.Nonce,
	}
	err := node.Send(msg.Address, &pong)
	if err != nil {
		return errors.Wrap(err, "could not send ping reply")
	}
	return nil
}

// processPong does nothing, as it signals a successfully completed heartbeat.
func (node *Node) processPong(msg *Message) error {
	return nil
}

// processDiscover responds to a discover message by sending a sample of peers that are known to us.
func (node *Node) processDiscover(msg *Message) error {
	addrs, err := node.book.Sample(10, Any(), RandomSort())
	if err != nil {
		return errors.Wrap(err, "could not get address sample")
	}
	err = node.share(msg.Address, addrs)
	if err != nil {
		return errors.Wrap(err, "could not share address sample")
	}
	return nil
}

// processPeers will process a received list of peer addresses by adding them to our address book.
func (node *Node) processPeers(msg *Message) error {
	peers := msg.Value.(*Peers)
	for _, addr := range peers.Addresses {
		node.book.Add(addr)
		if node.peers.has(addr) {
			node.book.Connected(addr)
		}
	}
	return nil
}

// event is called when something happens that is not processed by our network library. It will
// send the message to the subscriber to handle on a higher stack.
func (node *Node) event(e interface{}) {
	select {
	case node.subscriber <- e:
	default:
		node.log.Error("subscriber stalling", zap.String("event", fmt.Sprintf("%T", e)))
	}
}

// check will see if we are below minimum or above maximum peer count and add or remove peers as
// needed.
func (node *Node) check() {
	for {
		count := uint(atomic.LoadInt32(&node.count))
		if count < node.minPeers {
			node.add()
		}
		if count > node.maxPeers {
			node.remove()
		}
		time.Sleep(node.balance)
	}
}

// add will try to initialize a new outgoing connection and hand over to the outgoing handshake
// function on success.
func (node *Node) add() {
	entries, err := node.book.Sample(1, IsActive(false), ByPrioritySort())
	if err != nil {
		node.discover()
		return
	}
	addr := entries[0]
	if node.peers.has(addr) {
		node.log.Error("already connected to peer", zap.String("address", addr))
		return
	}
	conn, err := net.Dial("tcp", addr)
	if err != nil {
		node.log.Error("could not dial peer", zap.String("address", addr), zap.Error(err))
		return
	}
	go node.handshake(conn)
}

// discover will launch an attempt to discover new peers on the network, with a build-in timeout.
func (node *Node) discover() {
	select {
	case <-node.discovery.C:
	default:
		return
	}
	node.log.Info("launching peer discovery")
	discover := Discover{}
	err := node.Broadcast(&discover)
	if err != nil {
		node.log.Error("could not launch discovery", zap.Error(err))
	}
}

// remove will drop one of the current peers.
func (node *Node) remove() {
	index := 0
	goal := rand.Int() % node.peers.count()
	for _, peer := range node.peers.slice() {
		if index != goal {
			index++
			continue
		}
		node.disconnect(peer)
		return
	}
}

// known checks whether we already know a peer with the given nonce.
func (node *Node) known(nonce []byte) bool {
	for _, peer := range node.peers.slice() {
		if bytes.Equal(nonce, peer.nonce) {
			return true
		}
	}
	return false
}

// handshake starts an outgoing handshake by sending the network ID and our node nonce, then
// comparing the reply against our initial message.
func (node *Node) handshake(conn net.Conn) {
	addr := conn.RemoteAddr().String()
	node.log.Info("adding outgoing peer", zap.String("address", addr))
	atomic.AddInt32(&node.count, 1)
	syn := append(node.network, node.nonce...)
	_, err := conn.Write(syn)
	if err != nil {
		node.drop(conn)
		return
	}
	ack := make([]byte, len(syn))
	_, err = conn.Read(ack)
	if err != nil {
		node.drop(conn)
		return
	}
	code := ack[:len(node.network)]
	nonce := ack[len(node.network):]
	if !bytes.Equal(code, node.network) || bytes.Equal(nonce, node.nonce) || node.known(nonce) {
		node.log.Warn("dropping invalid outgoing connection", zap.String("address", addr))
		node.drop(conn)
		return
	}
	node.init(conn, nonce)
}

// init will initialize a new peer and add it to our registry after a successful handshake. It
// launches the required receiving go routine and does the initial sharing of our own peer address.
// Finally, it notifies the subscriber that a new connection was established.
func (node *Node) init(conn net.Conn, nonce []byte) {
	addr := conn.RemoteAddr().String()
	node.log.Info("finalizing handshake", zap.String("address", addr))
	r := lz4.NewReader(conn)
	w := lz4.NewWriter(conn)
	outgoing := make(chan interface{}, 16)
	incoming := make(chan interface{}, 16)
	p := peer{
		conn:      conn,
		addr:      addr,
		nonce:     nonce,
		r:         r,
		w:         w,
		outgoing:  outgoing,
		incoming:  incoming,
		codec:     node.codec,
		heartbeat: node.heartbeat,
		timeout:   node.timeout,
		hb:        time.NewTimer(node.heartbeat),
	}
	node.peers.add(addr, &p)
	node.book.Connected(addr)
	go p.receive()
	if node.server {
		err := node.share(addr, []string{node.address})
		if err != nil {
			node.log.Error("could not share initial address", zap.Error(err))
		}
	}
	e := Connected{
		Address: p.addr,
	}
	node.event(&e)
}

// share will share the given addresses with the peer of the given address.
func (node *Node) share(addr string, addrs []string) error {
	peers := Peers{
		Addresses: addrs,
	}
	err := node.Send(addr, &peers)
	if err != nil {
		return errors.Wrap(err, "could not send peers message")
	}
	return nil
}

// drop will disconnect a peer by closing the connection.
func (node *Node) drop(conn net.Conn) {
	addr := conn.RemoteAddr().String()
	node.log.Info("dropping connection", zap.String("address", addr))
	atomic.AddInt32(&node.count, -1)
	err := conn.Close()
	if err != nil {
		node.log.Error("could not close dropped connection", zap.Error(err))
	}
	node.book.Dropped(addr)
	node.book.Blacklist(addr)
}

// Send is used to send the given message to the peer with the given address.
func (node *Node) Send(addr string, msg interface{}) error {
	node.log.Debug("sending message", zap.String("type", fmt.Sprintf("%T", msg)), zap.String("address", addr))
	if !node.peers.has(addr) {
		return errors.New("could not find peer with given address")
	}
	peer := node.peers.get(addr)
	select {
	case peer.outgoing <- msg:
		return nil
	default:
		node.book.Failed(addr)
		node.disconnect(peer)
		return errors.New("could not send message, peer stalling")
	}
}

// Broadcast is used to broadcast a message to all peers we are connected to.
func (node *Node) Broadcast(msg interface{}) error {
	node.log.Debug("broadcasting message", zap.String("type", fmt.Sprintf("%T", msg)))
	for _, peer := range node.peers.slice() {
		select {
		case peer.outgoing <- msg:
			continue
		default:
			node.book.Failed(peer.addr)
			node.disconnect(peer)
			return errors.Errorf("could not broadcast message to %v, peer stalling", peer.addr)
		}
	}
	return nil
}

// Peers returns a list of peer addresses that we are connected to.
func (node *Node) Peers() []string {
	peers := node.peers.slice()
	addrs := make([]string, 0, len(peers))
	for _, peer := range peers {
		addrs = append(addrs, peer.addr)
	}
	return addrs
}
