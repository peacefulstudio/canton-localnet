// Copyright (c) 2026 Peaceful Studio OÜ
// SPDX-License-Identifier: Apache-2.0

package fixture

import (
	"context"
	"errors"
	"slices"
	"sync"
)

type createdUsers struct {
	mu          sync.Mutex
	outstanding []string
	attempts    map[string]int
}

func (c *createdUsers) record(userID string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.outstanding = append(c.outstanding, userID)
}

func (c *createdUsers) forget(userID string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if i := slices.Index(c.outstanding, userID); i >= 0 {
		c.outstanding = slices.Delete(c.outstanding, i, i+1)
	}
}

func (c *createdUsers) clearAttempts(userID string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.attempts, userID)
}

func (c *createdUsers) deleteAll(ctx context.Context, users *UserBuilder) error {
	c.mu.Lock()
	if len(c.outstanding) == 0 {
		c.mu.Unlock()
		return nil
	}
	pending := slices.Clone(c.outstanding)
	if c.attempts == nil {
		c.attempts = make(map[string]int)
	}
	attemptByUser := make(map[string]int, len(pending))
	for _, userID := range pending {
		c.attempts[userID]++
		attemptByUser[userID] = c.attempts[userID]
	}
	c.mu.Unlock()

	var failures []error
	for _, userID := range pending {
		if err := users.deleteUser(ctx, userID, attemptByUser[userID]); err != nil {
			failures = append(failures, err)
			continue
		}
		c.forget(userID)
		c.clearAttempts(userID)
	}
	return errors.Join(failures...)
}
