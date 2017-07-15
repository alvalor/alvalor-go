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
    "testing"
    "time"
    "go.uber.org/zap"
    "io"
    "github.com/stretchr/testify/assert"
)

func TestSetAddress(t *testing.T) {
    config := DefaultConfig
    addr := "192.168.4.62"
    
    setFunc := SetAddress("192.168.4.62")
    setFunc(&config)

    assert.Equal(t, addr, config.address)
}

func TestSetBalance(t *testing.T) {
    config := DefaultConfig
    balance := time.Duration(5)
    
    setFunc := SetBalance(balance)
    setFunc(&config)

    assert.Equal(t, balance, config.balance)
}

func TestSetBook(t *testing.T) {
    config := DefaultConfig
    book := NewSimpleBook()
    
    setFunc := SetBook(book)
    setFunc(&config)

    assert.Equal(t, book, config.book)
}

func TestSetCodec(t *testing.T) {
    config := DefaultConfig
    codec := DummyCodec{}
    
    setFunc := SetCodec(codec)
    setFunc(&config)

    assert.Equal(t, codec, config.codec)
}

func TestSetDiscovery(t *testing.T) {
    config := DefaultConfig
    discovery := time.Duration(5)
    
    setFunc := SetDiscovery(discovery)
    setFunc(&config)

    assert.Equal(t, discovery, config.discovery)
}

func TestSetHeartbeat(t *testing.T) {
    config := DefaultConfig
    heartbeat := time.Duration(5)
    
    setFunc := SetHeartbeat(heartbeat)
    setFunc(&config)

    assert.Equal(t, heartbeat, config.heartbeat)
}

func TestSetLog(t *testing.T) {
    config := DefaultConfig
    log, _ := zap.NewDevelopment()
    
    setFunc := SetLog(log)
    setFunc(&config)

    assert.Equal(t, log, config.log)
}

func TestSetMaxPeers(t *testing.T) {
    config := DefaultConfig
    maxPeers := uint(15)
    
    setFunc := SetMaxPeers(maxPeers)
    setFunc(&config)

    assert.Equal(t, maxPeers, config.maxPeers)
}

func TestSetMinPeers(t *testing.T) {
    config := DefaultConfig
    minPeers := uint(5)
    
    setFunc := SetMinPeers(minPeers)
    setFunc(&config)

    assert.Equal(t, minPeers, config.minPeers)
}

func TestSetNetwork(t *testing.T) {
    config := DefaultConfig
    network := make([]byte, 2)
    network[0] = 5
    network[1] = 10
    
    setFunc := SetNetwork(network)
    setFunc(&config)

    assert.EqualValues(t, network, config.network)
}

func TestSetServer(t *testing.T) {
    config := DefaultConfig
    server := true
    
    setFunc := SetServer(server)
    setFunc(&config)

    assert.Equal(t, server, config.server)
}

func TestSetSubscriber(t *testing.T) {
    config := DefaultConfig
    subscriber := make(chan interface{})
    
    setFunc := SetSubscriber(subscriber)
    setFunc(&config)

    assert.ObjectsAreEqual(subscriber, config.subscriber)
}

func TestSetTimeout(t *testing.T) {
    config := DefaultConfig
    timeout := time.Duration(5)
    
    setFunc := SetTimeout(timeout)
    setFunc(&config)

    assert.Equal(t, timeout, config.timeout)
}

type DummyCodec struct{}

func (s DummyCodec) Encode(w io.Writer, i interface{}) error {
	return nil
}

func (s DummyCodec) Decode(r io.Reader) (interface{}, error) {
	return 1, nil
}