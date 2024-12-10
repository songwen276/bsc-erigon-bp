// Copyright 2014 The go-ethereum Authors
// This file is part of the go-ethereum library.
//
// The go-ethereum library is free software: you can redistribute it and/or modify
// it under the terms of the GNU Lesser General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// The go-ethereum library is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
// GNU Lesser General Public License for more details.
//
// You should have received a copy of the GNU Lesser General Public License
// along with the go-ethereum library. If not, see <http://www.gnu.org/licenses/>.

package state

import (
	"github.com/VictoriaMetrics/fastcache"
	"github.com/ethereum/go-ethereum/common"
	"sync"
)

var storageCacheMap = NewStorageCacheMap()

type storageCache struct {
	lock     sync.RWMutex
	cacheMap map[common.Hash]map[common.Hash]common.Hash
}

// NewStorageCacheMap 初始化一个空的 storageCacheMap
func NewStorageCacheMap() *storageCache {
	return &storageCache{
		cacheMap: make(map[common.Hash]map[common.Hash]common.Hash),
	}
}

// Get 获取指定地址和槽位的值
func (s *storageCache) Get(addr common.Hash, slot common.Hash) (common.Hash, bool) {
	s.lock.RLock()         // 加读锁
	defer s.lock.RUnlock() // 函数结束释放读锁

	if slotMap, exists := s.cacheMap[addr]; exists {
		value, found := slotMap[slot]
		return value, found
	}
	return common.Hash{}, false
}

func (s *storageCache) GetSlotMap(addr common.Hash) (map[common.Hash]common.Hash, bool) {
	s.lock.RLock()         // 加读锁
	defer s.lock.RUnlock() // 函数结束释放读锁

	if slotMap, exists := s.cacheMap[addr]; exists {
		return slotMap, true
	}
	return nil, false
}

// Set 设置指定地址和槽位的值
func (s *storageCache) Set(addr common.Hash, slot common.Hash, value common.Hash) {
	s.lock.Lock()         // 加写锁
	defer s.lock.Unlock() // 函数结束释放写锁

	// 如果地址不存在，则创建新的槽位映射
	if _, exists := s.cacheMap[addr]; !exists {
		s.cacheMap[addr] = make(map[common.Hash]common.Hash)
	}
	s.cacheMap[addr][slot] = value
}

// Delete 删除指定地址和槽位的值
func (s *storageCache) Delete(addr common.Hash, slot common.Hash) {
	s.lock.Lock()
	defer s.lock.Unlock()

	if slotMap, exists := s.cacheMap[addr]; exists {
		delete(slotMap, slot)
		if len(slotMap) == 0 { // 如果地址的槽位为空，删除地址映射
			delete(s.cacheMap, addr)
		}
	}
}

// Delete 删除指定地址的所有槽位
func (s *storageCache) DeleteAll(addr common.Hash) {
	s.lock.Lock()
	defer s.lock.Unlock()

	// 删除整个地址映射，清除该地址下的所有槽位
	delete(s.cacheMap, addr)
}

var storageFastCache = fastcache.New(10 * 1024 * 1024 * 1024) // 10GB cache
