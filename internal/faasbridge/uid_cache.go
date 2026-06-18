// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package faasbridge

import (
	"sync"
	"time"
)

// uidCacheTTL bounds how long an open_id → uid mapping is reused. Lark uids are
// stable per user, but caching forever would mask the rare ID swap during
// migrations. 30 min matches the design doc.
const uidCacheTTL = 30 * time.Minute

type uidCache struct {
	m sync.Map // string → uidEntry
}

type uidEntry struct {
	uid       int64
	expiresAt time.Time
}

// globalUIDCache is process-wide; a CLI invocation is one process, so this is
// effectively per-invocation but cheap to share across calls within the same
// run.
var globalUIDCache = &uidCache{}

func (c *uidCache) Get(openID string) (int64, bool) {
	v, ok := c.m.Load(openID)
	if !ok {
		return 0, false
	}
	entry := v.(uidEntry)
	if time.Now().After(entry.expiresAt) {
		c.m.Delete(openID)
		return 0, false
	}
	return entry.uid, true
}

func (c *uidCache) Set(openID string, uid int64) {
	c.m.Store(openID, uidEntry{uid: uid, expiresAt: time.Now().Add(uidCacheTTL)})
}
