package service

import (
	"sync"

	apimodel "github.com/anyproto/anytype-heart/core/api/model"
)

// cacheManager handles thread-safe caching of properties, types, and tags per space
type cacheManager struct {
	mu sync.RWMutex

	// Caches organized by spaceId -> key -> object
	// For properties: key can be id, relationKey, or apiObjectKey
	// For types: key can be id, uniqueKey, or apiObjectKey
	// For tags: key is just id
	properties map[string]map[string]*apimodel.Property
	types      map[string]map[string]*apimodel.Type
	tags       map[string]map[string]*apimodel.Tag
}

func newCacheManager() *cacheManager {
	return &cacheManager{
		properties: make(map[string]map[string]*apimodel.Property),
		types:      make(map[string]map[string]*apimodel.Type),
		tags:       make(map[string]map[string]*apimodel.Tag),
	}
}

// Property cache methods
func (c *cacheManager) cacheProperty(spaceId string, prop *apimodel.Property) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if _, exists := c.properties[spaceId]; !exists {
		c.properties[spaceId] = make(map[string]*apimodel.Property)
	}

	c.properties[spaceId][prop.Id] = prop
	c.properties[spaceId][prop.RelationKey] = prop
	c.properties[spaceId][prop.Key] = prop
}

func (c *cacheManager) getProperties(spaceId string) map[string]*apimodel.Property {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if spaceCache, exists := c.properties[spaceId]; exists {
		return spaceCache
	}

	return make(map[string]*apimodel.Property)
}

// Type cache methods
func (c *cacheManager) cacheType(spaceId string, t *apimodel.Type) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if _, exists := c.types[spaceId]; !exists {
		c.types[spaceId] = make(map[string]*apimodel.Type)
	}

	c.types[spaceId][t.Id] = t
	c.types[spaceId][t.UniqueKey] = t
	c.types[spaceId][t.Key] = t
}

func (c *cacheManager) getTypes(spaceId string) map[string]*apimodel.Type {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if spaceCache, exists := c.types[spaceId]; exists {
		return spaceCache
	}

	return make(map[string]*apimodel.Type)
}

// Tag cache methods
func (c *cacheManager) cacheTag(spaceId string, tag *apimodel.Tag) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if _, exists := c.tags[spaceId]; !exists {
		c.tags[spaceId] = make(map[string]*apimodel.Tag)
	}

	c.tags[spaceId][tag.Id] = tag
}

func (c *cacheManager) getTags(spaceId string) map[string]*apimodel.Tag {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if spaceCache, exists := c.tags[spaceId]; exists {
		return spaceCache
	}

	return make(map[string]*apimodel.Tag)
}

func (c *cacheManager) removeProperty(spaceId, id, relationKey, key string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if spaceCache, exists := c.properties[spaceId]; exists {
		delete(spaceCache, id)
		delete(spaceCache, relationKey)
		delete(spaceCache, key)
	}
}

func (c *cacheManager) removeType(spaceId, id, uniqueKey, key string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if spaceCache, exists := c.types[spaceId]; exists {
		delete(spaceCache, id)
		delete(spaceCache, uniqueKey)
		delete(spaceCache, key)
	}
}

func (c *cacheManager) removeTag(spaceId, id string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if spaceCache, exists := c.tags[spaceId]; exists {
		delete(spaceCache, id)
	}
}

func (c *cacheManager) clear() {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.properties = nil
	c.types = nil
	c.tags = nil
}
