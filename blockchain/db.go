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

package blockchain

import (
	"bytes"

	"github.com/dgraph-io/badger/badger"
	"github.com/pkg/errors"

	"github.com/alvalor/alvalor-go/hasher"
	"github.com/alvalor/alvalor-go/trie"
)

// DB is a blockchain database that syncs the trie with the persistent key-value store on disk.
type DB struct {
	kv *badger.KV
	tr *trie.Trie
	cd Codec
}

// NewDB creates a new blockchain DB on the disk.
func NewDB(kv *badger.KV) (*DB, error) {
	tr := trie.New()
	itr := kv.NewIterator(badger.DefaultIteratorOptions)
	for itr.Rewind(); itr.Valid(); itr.Next() {
		item := itr.Item()
		key := item.Key()
		val := item.Value()
		ok := tr.Put(key, val, false)
		if !ok {
			return nil, errors.Errorf("could not insert key %x", key)
		}
	}
	itr.Close()
	db := &DB{kv: kv, tr: tr}
	return db, nil
}

// Insert will insert a new key and hash into the trie after storing the related hash and data on
// disk.
func (db *DB) Insert(id []byte, entity interface{}) error {
	buf := &bytes.Buffer{}
	err := db.cd.Encode(buf, entity)
	if err != nil {
		return errors.Wrap(err, "could not serialize entity")
	}
	data := buf.Bytes()
	hash := hasher.Sum256(data)
	err = db.kv.Set(hash, data)
	if err != nil {
		return errors.Wrap(err, "could not save entity on disk")
	}
	ok := db.tr.Put(id, hash, false)
	if !ok {
		return errors.Errorf("could not insert entity %x into trie", id)
	}
	return nil
}

// Retrieve will retrieve an entity from the key-value store by looking up the associated hash in
// the trie.
func (db *DB) Retrieve(id []byte) (interface{}, error) {
	hash, ok := db.tr.Get(id)
	if !ok {
		return nil, errors.Errorf("could not find entity %x in trie", id)
	}
	var kv badger.KVItem
	err := db.kv.Get(hash, &kv)
	if err != nil {
		return nil, errors.Wrap(err, "could not retrieve entity from disk")
	}
	buf := bytes.NewBuffer(kv.Value())
	entity, err := db.cd.Decode(buf)
	if err != nil {
		return nil, errors.Wrap(err, "could not deserialize entity")
	}
	return entity, nil
}
