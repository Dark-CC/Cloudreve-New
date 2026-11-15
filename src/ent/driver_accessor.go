package ent

import "entgo.io/ent/dialect"

// Driver returns the underlying dialect driver of the client.
//
// It is used by inventory-level additive schema reconciliation that needs to
// execute raw DDL without going through ent's schema diff.
func (c *Client) Driver() dialect.Driver {
	return c.driver
}
