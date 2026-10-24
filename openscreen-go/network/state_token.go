package ospc

// MERGE NOTE: branch osp-request-ids provides Agent.StateToken() (and the
// stateToken field) in request_id.go with identical semantics. When merging,
// delete this file and the stateToken field added to Agent on this branch.

// StateToken returns the state token of this (local) agent: 8 characters in
// the range [0-9A-Za-z]. It is created lazily and sent in agent-info.
func (a *Agent) StateToken() string {
	a.mu.Lock()
	defer a.mu.Unlock()

	if a.stateToken == "" {
		a.stateToken = randomAlphaNum(8)
	}
	return a.stateToken
}
