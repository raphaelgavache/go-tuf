package client

import (
	"github.com/theupdateframework/go-tuf/data"
	"github.com/theupdateframework/go-tuf/verify"
)

// getTargetFileMeta searches for a verified TargetFileMeta matching a target
// Requires a local snapshot to be loaded and is locked to the snapshot versions.
// Searches through delegated targets following TUF spec 1.0.19 section 5.6.
func (c *Client) getTargetFileMeta(target string) (data.TargetFileMeta, error) {
	snapshot, err := c.loadLocalSnapshot()
	if err != nil {
		return data.TargetFileMeta{}, err
	}

	// delegationsIterator covers 5.6.7
	// - pre-order depth-first search starting with the top targets
	// - filter delegations with paths or path_hash_prefixes matching searched target
	// - 5.6.7.1 cycles protection
	// - 5.6.7.2 terminations
	delegations := newDelegationsIterator(target)
	for i := 0; i < c.MaxDelegations; i++ {
		d, ok := delegations.next()
		if !ok {
			return data.TargetFileMeta{}, ErrUnknownTarget{target, snapshot.Version}
		}

		// covers 5.6.{1,2,3,4,5,6}
		targets, err := c.loadDelegatedTargets(snapshot, d.delegatee.Name, d.verifier)
		if err != nil {
			return data.TargetFileMeta{}, err
		}

		// stop when the searched TargetFileMeta is found
		if m, ok := targets.Targets[target]; ok {
			return m, nil
		}

		if targets.Delegations != nil {
			delegationsVerifier, err := verify.NewDelegationsVerifier(targets.Delegations)
			if err != nil {
				return data.TargetFileMeta{}, err
			}
			err = delegations.add(targets.Delegations.Roles, d.delegatee.Name, delegationsVerifier)
			if err != nil {
				return data.TargetFileMeta{}, err
			}
		}
	}

	return data.TargetFileMeta{}, ErrMaxDelegations{
		Target:          target,
		MaxDelegations:  c.MaxDelegations,
		SnapshotVersion: snapshot.Version,
	}
}

func (c *Client) loadLocalSnapshot() (*data.Snapshot, error) {
	if err := c.getLocalMeta(); err != nil {
		return nil, err
	}

	rawS, ok := c.localMeta["snapshot.json"]
	if !ok {
		return nil, ErrNoLocalSnapshot
	}

	snapshot := &data.Snapshot{}
	if err := c.db.Unmarshal(rawS, snapshot, "snapshot", c.snapshotVer); err != nil {
		return nil, ErrDecodeFailed{"snapshot.json", err}
	}
	return snapshot, nil
}

// loadDelegatedTargets downloads, decodes, verifies and stores targets
func (c *Client) loadDelegatedTargets(snapshot *data.Snapshot, role string, verifier verify.DelegationsVerifier) (*data.Targets, error) {
	var err error
	fileName := role + ".json"
	fileMeta, ok := snapshot.Meta[fileName]
	if !ok {
		return nil, ErrRoleNotInSnapshot{role, snapshot.Version}
	}

	// 5.6.1 download target if not in the local store
	// 5.6.2 check against snapshot hash
	// 5.6.4 check against snapshot version
	raw, alreadyStored := c.localMetaFromSnapshot(fileName, fileMeta)
	if !alreadyStored {
		raw, err = c.downloadMetaFromSnapshot(fileName, fileMeta)
		if err != nil {
			return nil, err
		}
	}

	targets := &data.Targets{}
	// 5.6.3 verify signature with parent public keys
	// 5.6.5 verify that the targets is not expired
	// role "targets" is a top role verified by root keys loaded in the client db
	if role == "targets" {
		err = c.db.Unmarshal(raw, targets, role, fileMeta.Version)
	} else {
		err = verifier.Unmarshal(raw, targets, role, fileMeta.Version)
	}
	if err != nil {
		return nil, ErrDecodeFailed{fileName, err}
	}

	// 5.6.6 persist
	if !alreadyStored {
		if err := c.local.SetMeta(fileName, raw); err != nil {
			return nil, err
		}
	}
	return targets, nil
}

type delegation struct {
	delegator string
	verifier  verify.DelegationsVerifier
	delegatee data.DelegatedRole
}

type delegationsIterator struct {
	stack        []delegation
	target       string
	visitedRoles map[string]struct{}
}

// newDelegationsIterator initialises an iterator with a first step
// on top level targets
func newDelegationsIterator(target string) *delegationsIterator {
	i := &delegationsIterator{
		target: target,
		stack: []delegation{
			{
				delegatee: data.DelegatedRole{Name: "targets"},
			},
		},
		visitedRoles: make(map[string]struct{}),
	}
	return i
}

func (d *delegationsIterator) next() (value delegation, ok bool) {
	if len(d.stack) == 0 {
		return delegation{}, false
	}
	delegation := d.stack[len(d.stack)-1]
	d.stack = d.stack[:len(d.stack)-1]

	// 5.6.7.1: If this role has been visited before, then skip this role (so
	// that cycles in the delegation graph are avoided).
	roleName := delegation.delegatee.Name
	if _, ok := d.visitedRoles[roleName]; ok {
		return d.next()
	}
	d.visitedRoles[roleName] = struct{}{}

	// 5.6.7.2 trim delegations to visit, only the current role and its delegations
	// will be considered
	// https://github.com/theupdateframework/specification/issues/168
	if delegation.delegatee.Terminating {
		// Empty the stack.
		d.stack = d.stack[0:0]
	}
	return delegation, true
}

func (d *delegationsIterator) add(roles []data.DelegatedRole, delegator string, verifier verify.DelegationsVerifier) error {
	for i := len(roles) - 1; i >= 0; i-- {
		// Push the roles onto the stack in reverse so we get an preorder traversal
		// of the delegations graph.
		r := roles[i]
		matchesPath, err := r.MatchesPath(d.target)
		if err != nil {
			return err
		}
		if matchesPath {
			delegation := delegation{
				delegator: delegator,
				delegatee: r,
				verifier:  verifier,
			}
			d.stack = append(d.stack, delegation)
		}
	}

	return nil
}
