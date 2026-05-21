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
// drift. Subscribe is idempotent per-connection, so re-Mounts (which
// happen on every HTTP request and WS connect) are no-ops.
//
// We propagate the error rather than silently discarding it: SelfTopic()
// is ACL-exempt in livetemplate v0.10.0, but a controller that copies
// this pattern with a developer topic name MUST propagate the error to
// trigger the keep-open lvt:error envelope path. Keeping the propagation
// here makes the example safe-by-default for readers.
func (c *CounterController) Mount(s Counter, ctx *livetemplate.Context) (Counter, error) {
	if err := ctx.Subscribe(ctx.SelfTopic()); err != nil {
		return s, err
	}
	return s, nil
}

// Increment is the action handler invoked when the user clicks the
// "+" button. Mutate the cloned state, then publish the same action
// to every *other* subscribed connection so multiple embeds (and tabs)
// stay in sync. Publishes triggered by a dispatched action are no-ops
// (the framework's recursion guard), so no infinite loop.
//
// Mutate-first ordering is safe even when Publish errors: the
// livetemplate dispatcher only assigns the returned newState to the
// connection's persisted state when the action returns (state, nil).
// On (state, err), newState is discarded — both this connection AND
// the peer connections that never received the failed Publish stay at
// the pre-action state. No divergence by construction.
func (c *CounterController) Increment(s Counter, ctx *livetemplate.Context) (Counter, error) {
	s.Count++
	if err := ctx.Publish(ctx.SelfTopic(), "Increment", nil); err != nil {
		return s, err
	}
	return s, nil
}

// Decrement and Reset follow the same pattern: mutate, publish.
func (c *CounterController) Decrement(s Counter, ctx *livetemplate.Context) (Counter, error) {
	s.Count--
	if err := ctx.Publish(ctx.SelfTopic(), "Decrement", nil); err != nil {
		return s, err
	}
	return s, nil
}

func (c *CounterController) Reset(s Counter, ctx *livetemplate.Context) (Counter, error) {
	s.Count = 0
	if err := ctx.Publish(ctx.SelfTopic(), "Reset", nil); err != nil {
		return s, err
	}
	return s, nil
}
