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

package inventories

import (
	"testing"

	"github.com/alvalor/alvalor-go/types"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
)

func TestNewRepo(t *testing.T) {
	repo := NewRepo()
	assert.NotNil(t, repo.inventories)
}

func TestRepoAdd(t *testing.T) {

	// initialize the repository with required maps
	repo := &Repo{
		inventories: make(map[types.Hash]*types.Inventory),
	}

	// create entities and set up state
	hash1 := types.Hash{0x1}
	hash2 := types.Hash{0x2}

	inv1 := &types.Inventory{Hash: hash1}
	inv2 := &types.Inventory{Hash: hash2}

	repo.inventories[hash2] = &types.Inventory{}

	// execute add
	err1 := repo.Add(inv1)
	err2 := repo.Add(inv2)

	assert.Nil(t, err1)
	if assert.NotNil(t, err2) {
		assert.Equal(t, ErrExist, errors.Cause(err2))
	}
	if assert.Len(t, repo.inventories, 2) {
		assert.Equal(t, inv1, repo.inventories[hash1])
		assert.NotEqual(t, inv2, repo.inventories[hash2])
	}
}

func TestRepoHas(t *testing.T) {

	// initialize the repository with required maps
	repo := &Repo{
		inventories: make(map[types.Hash]*types.Inventory),
	}

	// create entities and set up state
	hash1 := types.Hash{0x1}
	hash2 := types.Hash{0x2}

	repo.inventories[hash1] = &types.Inventory{}

	// execute has
	ok1 := repo.Has(hash1)
	ok2 := repo.Has(hash2)

	// check conditions
	assert.True(t, ok1)
	assert.False(t, ok2)
}

func TestRepoGet(t *testing.T) {

	// initialize the repository with required maps
	repo := &Repo{
		inventories: make(map[types.Hash]*types.Inventory),
	}

	// create entities and set up state
	hash1 := types.Hash{0x1}
	hash2 := types.Hash{0x2}

	inv1 := &types.Inventory{Hash: hash1}

	repo.inventories[hash1] = inv1

	// execute get
	out1, err1 := repo.Get(hash1)
	_, err2 := repo.Get(hash2)

	// check conditions
	assert.Nil(t, err1)
	assert.Equal(t, inv1, out1)

	if assert.NotNil(t, err2) {
		assert.Equal(t, ErrNotExist, errors.Cause(err2))
	}
}
