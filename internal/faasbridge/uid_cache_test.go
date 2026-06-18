// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package faasbridge

import (
	"testing"
	"time"
)

func TestUIDCacheSetGet(t *testing.T) {
	t.Parallel()
	c := &uidCache{}
	if _, ok := c.Get("o1"); ok {
		t.Error("empty cache should miss")
	}
	c.Set("o1", 42)
	if uid, ok := c.Get("o1"); !ok || uid != 42 {
		t.Errorf("Get after Set = (%d, %v), want (42, true)", uid, ok)
	}
}

func TestUIDCacheExpiry(t *testing.T) {
	t.Parallel()
	c := &uidCache{}
	c.m.Store("o1", uidEntry{uid: 42, expiresAt: time.Now().Add(-time.Minute)})
	if _, ok := c.Get("o1"); ok {
		t.Error("expired entry should miss")
	}
}
