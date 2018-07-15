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

package node

import (
	"io"
	"sync"

	"github.com/rs/zerolog"

	"github.com/Workiva/go-datastructures/queue"
	"github.com/alvalor/alvalor-go/trie"
	"github.com/alvalor/alvalor-go/types"
)

// Node defines the exposed API of the Alvalor node package.
type Node interface {
	Submit(tx *types.Transaction)
	Stats()
}

// Network defines what we need from the network module.
type Network interface {
	Send(address string, msg interface{}) error
	Broadcast(msg interface{}, exclude ...string) error
}

// Codec is responsible for serializing and deserializing data for disk storage.
type Codec interface {
	Encode(w io.Writer, i interface{}) error
	Decode(r io.Reader) (interface{}, error)
}

type simpleNode struct {
	log                 zerolog.Logger
	wg                  *sync.WaitGroup
	net                 Network
	chain               Blockchain
	finder              Finder
	peers               peerManager
	pool                poolManager
	events              eventManager
	stream              chan interface{}
	subscribers         []subscriber
	headerReceivers     map[string]*queue.Queue
	headerReceiversLock sync.Mutex
	stop                chan struct{}
}

type subscriber struct {
	channel chan<- interface{}
	buffer  chan interface{}
	filters []func(interface{}) bool
}

// New creates a new node to manage the Alvalor blockchain.
func New(log zerolog.Logger, net Network, chain Blockchain, finder Finder, codec Codec, input <-chan interface{}) Node {

	// initialize the node
	n := &simpleNode{}

	// configure the logger
	log = log.With().Str("package", "node").Logger()
	n.log = log

	// initialize the wait group
	wg := &sync.WaitGroup{}
	n.wg = wg

	// store dependency references
	n.net = net
	n.chain = chain
	n.finder = finder

	// initialize peer state manager
	peers := newPeers()
	n.peers = peers

	// initialize simple transaction pool
	store := trie.NewBin()
	pool := newPool(codec, store)
	n.pool = pool

	// create the subscriber channel
	n.stream = make(chan interface{}, 128)

	// create the channel for shutdown
	n.stop = make(chan struct{})

	// create map of queues where each key is an address of the header receiver
	n.headerReceivers = make(map[string]*queue.Queue)
	n.headerReceiversLock = sync.Mutex{}

	// handle all input messages we get
	n.Input(input)
	n.events = &simpleEventManager{stream: n.stream}

	n.Stream()
	n.SendHeaders()

	return n
}

func (n *simpleNode) Subscribe(channel chan<- interface{}, filters ...func(interface{}) bool) {
	sub := subscriber{filters: filters, channel: channel, buffer: make(chan interface{}, 128)}
	n.subscribers = append(n.subscribers, sub)
	n.Subscriber(sub)
}

func (n *simpleNode) Submit(tx *types.Transaction) {
	n.Entity(tx)
}

func (n *simpleNode) Stats() {
	numActive := uint(len(n.peers.Actives()))
	numTxs := n.pool.Count()
	n.log.Info().Uint("num_active", numActive).Uint("num_txs", numTxs).Msg("stats")
}

func (n *simpleNode) Input(input <-chan interface{}) {
	n.wg.Add(1)
	go handleInput(n.log, n.wg, n, input)
}

func (n *simpleNode) Event(event interface{}) {
	n.wg.Add(1)
	go handleEvent(n.log, n.wg, n.net, n.chain, n.peers, n, event)
}

func (n *simpleNode) Message(address string, message interface{}) {
	n.wg.Add(1)
	go handleMessage(n.log, n.wg, n.net, n.chain, n.finder, n.peers, n.pool, n, address, message)
}

func (n *simpleNode) Entity(entity Entity) {
	n.wg.Add(1)
	go handleEntity(n.log, n.wg, n.net, n.peers, n.pool, entity, n.events)
}

func (n *simpleNode) Collect(path []types.Hash) {
}

func (n *simpleNode) Stream() {
	n.wg.Add(1)
	go handleStream(n.log, n.wg, n.stream, n.subscribers)
}

func (n *simpleNode) Subscriber(sub subscriber) {
	n.wg.Add(1)
	go handleSubscriber(n.log, n.wg, sub)
}

func (n *simpleNode) Stop() {
	n.wg.Wait()
	close(n.stop)
	close(n.stream)
	for _, sub := range n.subscribers {
		close(sub.channel)
		close(sub.buffer)
	}
}

func (n *simpleNode) SendHeaders() {
	n.wg.Add(1)
	go handleHeaders(n.log, n.wg, n.net, n.headerReceivers, n.stop)
}

func (n *simpleNode) Header(address string, header *types.Header) {
	q := n.getQueue(address)
	q.Put(header)
}

func (n *simpleNode) getQueue(address string) *queue.Queue {
	n.headerReceiversLock.Lock()
	defer n.headerReceiversLock.Unlock()

	if q, ok := n.headerReceivers[address]; ok {
		return q
	}

	q := queue.New(1024)
	n.headerReceivers[address] = q
	return q
}
