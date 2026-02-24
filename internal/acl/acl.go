package acl

import (
	"fmt"
	"strings"
)

// Permission represents an access level for a repository path.
type Permission int

const (
	Deny       Permission = 0
	ReadOnly   Permission = 1
	AppendOnly Permission = 2
	FullAccess Permission = 3
)

func (p Permission) String() string {
	switch p {
	case Deny:
		return "deny"
	case ReadOnly:
		return "read-only"
	case AppendOnly:
		return "append-only"
	case FullAccess:
		return "full-access"
	default:
		return fmt.Sprintf("unknown(%d)", int(p))
	}
}

// ParsePermission converts a string to a Permission value.
func ParsePermission(s string) (Permission, error) {
	switch strings.ToLower(s) {
	case "deny":
		return Deny, nil
	case "read-only":
		return ReadOnly, nil
	case "append-only":
		return AppendOnly, nil
	case "full-access":
		return FullAccess, nil
	default:
		return Deny, fmt.Errorf("invalid permission %q: must be deny, read-only, append-only, or full-access", s)
	}
}

// OperationType represents the kind of operation being performed.
type OperationType int

const (
	OpRead   OperationType = iota
	OpWrite                // includes lock deletion and blob creation
	OpDelete               // blob deletion (blocked in append-only)
)

// Rule defines an ACL rule matching identities to paths with a permission level.
type Rule struct {
	Paths      []string
	Identities []string
	Permission Permission
}

// Engine evaluates ACL rules to determine whether an operation is allowed.
type Engine struct {
	defaultPerm Permission
	rules       []Rule
}

// New creates an ACL engine with the given default permission and rules.
// Rule paths are normalized to always start with "/".
func New(defaultPerm Permission, rules []Rule) (*Engine, error) {
	for i, r := range rules {
		if len(r.Paths) == 0 {
			return nil, fmt.Errorf("acl rule %d: paths must not be empty", i)
		}
		if len(r.Identities) == 0 {
			return nil, fmt.Errorf("acl rule %d: identities must not be empty", i)
		}
		// Normalize paths to start with /
		for j, p := range r.Paths {
			rules[i].Paths[j] = normalizePath(p)
		}
	}
	return &Engine{defaultPerm: defaultPerm, rules: rules}, nil
}

// Resolve returns the effective permission for the given identity and repo path.
//
// Cascade logic:
//  1. Collect all rules matching both identity and path (segment-boundary prefix match).
//  2. Group by matched path depth (number of segments). Deepest match wins.
//  3. At the deepest level: if any rule is Deny, result is Deny.
//     Otherwise, the highest permission wins.
//  4. No match → default permission.
func (e *Engine) Resolve(identity, repoPath string) Permission {
	repoPath = normalizePath(repoPath)

	bestDepth := -1
	var permsAtBest []Permission

	for _, rule := range e.rules {
		if !matchIdentity(rule.Identities, identity) {
			continue
		}
		for _, rulePath := range rule.Paths {
			depth, ok := matchPath(rulePath, repoPath)
			if !ok {
				continue
			}
			if depth > bestDepth {
				bestDepth = depth
				permsAtBest = permsAtBest[:0]
				permsAtBest = append(permsAtBest, rule.Permission)
			} else if depth == bestDepth {
				permsAtBest = append(permsAtBest, rule.Permission)
			}
		}
	}

	if bestDepth < 0 {
		return e.defaultPerm
	}

	// Deny is absolute at the deepest matching level
	highest := permsAtBest[0]
	for _, p := range permsAtBest[1:] {
		if p == Deny {
			return Deny
		}
		if p > highest {
			highest = p
		}
	}
	if highest == Deny {
		return Deny
	}
	return highest
}

// Allowed checks whether the given operation is permitted for the identity and path.
func (e *Engine) Allowed(identity, repoPath string, op OperationType) bool {
	perm := e.Resolve(identity, repoPath)
	switch op {
	case OpRead:
		return perm >= ReadOnly
	case OpWrite:
		return perm >= AppendOnly
	case OpDelete:
		return perm >= FullAccess
	default:
		return false
	}
}

// normalizePath ensures a path starts with "/" and has no trailing slash (except root).
func normalizePath(p string) string {
	if p == "" || p == "/" {
		return "/"
	}
	if !strings.HasPrefix(p, "/") {
		p = "/" + p
	}
	return strings.TrimRight(p, "/")
}

// segmentCount returns the number of path segments (root "/" = 0).
func segmentCount(p string) int {
	p = strings.Trim(p, "/")
	if p == "" {
		return 0
	}
	return strings.Count(p, "/") + 1
}

// matchPath checks if rulePath is a segment-boundary prefix of reqPath.
// Returns the depth (segment count) of the rule path and whether it matched.
// "/server-a" matches "/server-a" and "/server-a/sub" but NOT "/server-ab".
func matchPath(rulePath, reqPath string) (int, bool) {
	if rulePath == "/" {
		return 0, true
	}
	if reqPath == rulePath {
		return segmentCount(rulePath), true
	}
	if strings.HasPrefix(reqPath, rulePath+"/") {
		return segmentCount(rulePath), true
	}
	return 0, false
}

// matchIdentity checks if any of the rule's identities match the request identity.
// "*" matches any identity.
func matchIdentity(ruleIdentities []string, identity string) bool {
	for _, ri := range ruleIdentities {
		if ri == "*" || ri == identity {
			return true
		}
	}
	return false
}
