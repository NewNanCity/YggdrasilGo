// Package blessing_skin LRU缓存实现
package blessing_skin

import (
	"container/list"
	"sync"
)

// LRUCache LRU缓存实现
type LRUCache struct {
	capacity int
	cache    map[string]*list.Element
	list     *list.List
	mutex    sync.RWMutex
}

// cacheItem 缓存项
type cacheItem struct {
	key   string
	value string
}

// NewLRUCache 创建LRU缓存
func NewLRUCache(capacity int) *LRUCache {
	if capacity <= 0 {
		capacity = 1000 // 默认容量
	}

	return &LRUCache{
		capacity: capacity,
		cache:    make(map[string]*list.Element),
		list:     list.New(),
	}
}

// Get 获取缓存值
func (c *LRUCache) Get(key string) (string, bool) {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	if elem, exists := c.cache[key]; exists {
		// 移动到前面（最近使用）
		c.list.MoveToFront(elem)
		return elem.Value.(*cacheItem).value, true
	}
	return "", false
}

// Put 设置缓存值
func (c *LRUCache) Put(key, value string) {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	if elem, exists := c.cache[key]; exists {
		// 更新现有项
		c.list.MoveToFront(elem)
		elem.Value.(*cacheItem).value = value
		return
	}

	// 检查容量限制
	if c.list.Len() >= c.capacity {
		// 移除最久未使用的项
		back := c.list.Back()
		if back != nil {
			c.list.Remove(back)
			delete(c.cache, back.Value.(*cacheItem).key)
		}
	}

	// 添加新项
	item := &cacheItem{key: key, value: value}
	elem := c.list.PushFront(item)
	c.cache[key] = elem
}

// Delete 删除缓存项
func (c *LRUCache) Delete(key string) {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	if elem, exists := c.cache[key]; exists {
		c.list.Remove(elem)
		delete(c.cache, key)
	}
}

// Size 获取当前缓存大小
func (c *LRUCache) Size() int {
	c.mutex.RLock()
	defer c.mutex.RUnlock()
	return len(c.cache)
}

// Clear 清空缓存
func (c *LRUCache) Clear() {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	c.cache = make(map[string]*list.Element)
	c.list = list.New()
}

// UUIDCache UUID双向缓存
type UUIDCache struct {
	nameToUUID *LRUCache
	uuidToName *LRUCache
	maxSize    int
}

// NewUUIDCache 创建UUID缓存
func NewUUIDCache(maxSize int) *UUIDCache {
	if maxSize <= 0 {
		maxSize = 1000 // 默认缓存1000条
	}

	return &UUIDCache{
		nameToUUID: NewLRUCache(maxSize),
		uuidToName: NewLRUCache(maxSize),
		maxSize:    maxSize,
	}
}

// GetUUIDByName 根据角色名获取UUID（从缓存）
func (uc *UUIDCache) GetUUIDByName(name string) (string, bool) {
	return uc.nameToUUID.Get(name)
}

// GetNameByUUID 根据UUID获取角色名（从缓存）
func (uc *UUIDCache) GetNameByUUID(uuid string) (string, bool) {
	return uc.uuidToName.Get(uuid)
}

// PutMapping 添加UUID映射到缓存
func (uc *UUIDCache) PutMapping(name, uuid string) {
	uc.nameToUUID.Put(name, uuid)
	uc.uuidToName.Put(uuid, name)
}

// DeleteMapping 删除UUID映射
func (uc *UUIDCache) DeleteMapping(name, uuid string) {
	uc.nameToUUID.Delete(name)
	uc.uuidToName.Delete(uuid)
}

// Size 获取缓存大小
func (uc *UUIDCache) Size() int {
	return uc.nameToUUID.Size()
}

// Clear 清空缓存
func (uc *UUIDCache) Clear() {
	uc.nameToUUID.Clear()
	uc.uuidToName.Clear()
}

// GetStats 获取缓存统计信息
func (uc *UUIDCache) GetStats() map[string]any {
	return map[string]any{
		"max_size":     uc.maxSize,
		"current_size": uc.Size(),
		"name_to_uuid": uc.nameToUUID.Size(),
		"uuid_to_name": uc.uuidToName.Size(),
	}
}
