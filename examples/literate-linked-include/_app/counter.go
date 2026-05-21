package main

import "github.com/livetemplate/livetemplate"

// Counter is the state — pure data, cloned per session by livetemplate.
type Counter struct {
	Count int
}

// CounterController holds dependencies (none in this demo).
type CounterController struct{}

// Mount opts this connection in to peer fan-out for the session via
// ctx.Subscribe(ctx.SelfTopic()). Without this opt-in, an Increment in
// one tab's Publish would have no peer subscribers and the embeds would
// drift. SelfTopic() is ACL-exempt; Subscribe always succeeds for it.
func (c *CounterController) Mount(s Counter, ctx *livetemplate.Context) (Counter, error) {
	_ = ctx.Subscribe(ctx.SelfTopic())
	return s, nil
}

// Increment is the action handler invoked when the user clicks the
// "+" button. Mutate the cloned state, then publish the same action
// to every *other* subscribed connection so multiple embeds (and tabs)
// stay in sync. Publishes triggered by a dispatched action are no-ops
// (the framework's recursion guard), so no infinite loop.
func (c *CounterController) Increment(s Counter, ctx *livetemplate.Context) (Counter, error) {
	s.Count++
	ctx.Publish(ctx.SelfTopic(), "Increment", nil)
	return s, nil
}

// Decrement and Reset follow the same pattern: mutate, publish.
func (c *CounterController) Decrement(s Counter, ctx *livetemplate.Context) (Counter, error) {
	s.Count--
	ctx.Publish(ctx.SelfTopic(), "Decrement", nil)
	return s, nil
}

func (c *CounterController) Reset(s Counter, ctx *livetemplate.Context) (Counter, error) {
	s.Count = 0
	ctx.Publish(ctx.SelfTopic(), "Reset", nil)
	return s, nil
}
