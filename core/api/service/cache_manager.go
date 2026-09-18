package service

import (
	"sync"

	apimodel "github.com/anyproto/anytype-heart/core/api/model"
	"github.com/anyproto/anytype-heart/core/api/util"
)

// participantEntry caches the participant fields needed to enrich chat message
// creators without an extra ObjectSearch per request.
type participantEntry struct {
	Id       string
	Identity string
	Name     string
}

// cacheManager handles thread-safe caching of properties, types, tags, and
// participants per space.
// NOTE: Current implementation copies maps on read to prevent concurrent access issues.
// For better performance (especially with many entries), we might consider implementing
// copy-on-write using atomic.Value to make reads lock- and copy-free.
type cacheManager struct {
	mu sync.RWMutex

	// Caches organized by spaceId -> key -> object
	// For properties: key can be id, relationKey, or apiObjectKey
	// For types: key can be id, uniqueKey, or apiObjectKey
	// For tags: key can be id, uniqueKey, or apiObjectKey
	// For participants: key is the raw identity string
	properties   map[string]map[string]*apimodel.Property
	types        map[string]map[string]*apimodel.Type
	tags         map[string]map[string]*apimodel.Tag
	participants map[string]map[string]*participantEntry
}

func newCacheManager() *cacheManager {
	return &cacheManager{
		properties:   make(map[string]map[string]*apimodel.Property),
		types:        make(map[string]map[string]*apimodel.Type),
		tags:         make(map[string]map[string]*apimodel.Tag),
		participants: make(map[string]map[string]*participantEntry),
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
		// Return a copy to prevent concurrent map read/write after lock is released
		copy := make(map[string]*apimodel.Property, len(spaceCache))
		for k, v := range spaceCache {
			copy[k] = v
		}
		return copy
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
	// the key DERIVED from the unique key, always — not only when it happens
	// to equal t.Key. t.Key is the apiObjectKey slug when one is stored, and
	// for a BSON-keyed custom type ("ot-<hex>") the bare "<hex>" is then
	// present in NO other slot: uniqueKey keeps its "ot-" prefix and the id is
	// the object id. Every v1 address the surface has ever served for such a
	// type is that hex — a create's typeKey, a search's `types`, the `key` of
	// every object row it ever returned — so the moment the apiObjectKey
	// backfill stamps a slug, ResolveTypeApiKey stops answering for it and
	// create/update 400/500 while search silently drops the type from its
	// filter. Indexing the derived key alongside the slug keeps both spellings
	// live; properties already get this for free from the RelationKey slot.
	// Written BEFORE t.Key so an explicit slug still wins the slot on a clash.
	if derived := util.ToTypeApiKey(t.UniqueKey); derived != "" {
		c.types[spaceId][derived] = t
	}
	c.types[spaceId][t.Key] = t
}

func (c *cacheManager) getTypes(spaceId string) map[string]*apimodel.Type {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if spaceCache, exists := c.types[spaceId]; exists {
		// Return a copy to prevent concurrent map read/write after lock is released
		copy := make(map[string]*apimodel.Type, len(spaceCache))
		for k, v := range spaceCache {
			copy[k] = v
		}
		return copy
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
	c.tags[spaceId][tag.UniqueKey] = tag
	c.tags[spaceId][tag.Key] = tag
}

func (c *cacheManager) getTags(spaceId string) map[string]*apimodel.Tag {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if spaceCache, exists := c.tags[spaceId]; exists {
		// Return a copy to prevent concurrent map read/write after lock is released
		copy := make(map[string]*apimodel.Tag, len(spaceCache))
		for k, v := range spaceCache {
			copy[k] = v
		}
		return copy
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
		delete(spaceCache, util.ToTypeApiKey(uniqueKey)) // the slot cacheType adds
		delete(spaceCache, key)
	}
}

func (c *cacheManager) removeTag(spaceId, id, uniqueKey, key string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if spaceCache, exists := c.tags[spaceId]; exists {
		delete(spaceCache, id)
		delete(spaceCache, uniqueKey)
		delete(spaceCache, key)
	}
}

// Participant cache methods
func (c *cacheManager) cacheParticipant(spaceId string, p *participantEntry) {
	if spaceId == "" || p == nil || p.Identity == "" {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()

	if _, exists := c.participants[spaceId]; !exists {
		c.participants[spaceId] = make(map[string]*participantEntry)
	}
	c.participants[spaceId][p.Identity] = p
}

// getParticipantByIdentity returns the cached participant for the given
// (spaceId, identity) pair, or nil if it has not yet been observed by the
// cross-space subscription.
func (c *cacheManager) getParticipantByIdentity(spaceId, identity string) *participantEntry {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if spaceCache, exists := c.participants[spaceId]; exists {
		return spaceCache[identity]
	}
	return nil
}

func (c *cacheManager) removeParticipant(spaceId, identity string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if spaceCache, exists := c.participants[spaceId]; exists {
		delete(spaceCache, identity)
	}
}

func (c *cacheManager) clear() {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.properties = nil
	c.types = nil
	c.tags = nil
	c.participants = nil
}
