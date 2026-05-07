package main

import "github.com/livetemplate/livetemplate"

// Counter is the state — pure data, cloned per session by livetemplate.
type Counter struct {
	Count int
}

// CounterController holds dependencies (none in this demo).
type CounterController struct{}

// Increment is the action handler invoked when the user clicks the
// "+" button. Mutate the cloned state, then broadcast the same action
// to every *other* connected client so multiple embeds (and tabs)
// stay in sync. Broadcasts triggered by a broadcast are no-ops, so
// no infinite loop.
func (c *CounterController) Increment(s Counter, ctx *livetemplate.Context) (Counter, error) {
	s.Count++
	ctx.BroadcastAction("Increment", nil)
	return s, nil
}

// Decrement and Reset follow the same pattern: mutate, broadcast.
func (c *CounterController) Decrement(s Counter, ctx *livetemplate.Context) (Counter, error) {
	s.Count--
	ctx.BroadcastAction("Decrement", nil)
	return s, nil
}

func (c *CounterController) Reset(s Counter, ctx *livetemplate.Context) (Counter, error) {
	s.Count = 0
	ctx.BroadcastAction("Reset", nil)
	return s, nil
}
