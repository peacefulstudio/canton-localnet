// Copyright (c) 2026 Peaceful Studio OÜ
// SPDX-License-Identifier: Apache-2.0

// Package rights decides which of a ledger user's rights are durable and
// which are the residue left behind by integration suites, and applies
// that decision against a participant's JSON Ledger API.
//
// The preserve/revoke contract this implements is stated for users in
// the "rights prune" command's help text and echoed in the plan every
// run prints. Read it there.
package rights

import (
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"
)

// AdminKind is the right kind that marks a user as the participant's
// admin user. Its presence is the only evidence that a rights list
// belongs to the validator user and not to somebody's real account, so a
// prune refuses to touch a user that does not hold it.
const AdminKind = "ParticipantAdmin"

// ActAsKind is the right kind carrying the party a user may submit
// commands as. It is the kind integration suites grant and leak.
const ActAsKind = "CanActAs"

// MaxRevoke is the default sanity bound on the size of a revoke list. It
// sits at the participant's own per-user limit, so on a stock
// participant it cannot fire: it exists to stop a runaway list, not to
// protect the user, and it is deliberately not lower — recovering a
// participant saturated at 999 rights is the case this tool exists for.
const MaxRevoke = 1000

// ErrNoAdminRight reports that the target user holds no ParticipantAdmin
// right, which means the caller is pointed at a user whose rights are
// not disposable.
var ErrNoAdminRight = errors.New("rights: no " + AdminKind + " right found to preserve — refusing to touch this user")

// ErrNoOwnPartyRight reports that the target user holds no CanActAs
// right on the party the policy preserves. Every validator user holds
// one, so its absence means the preserved party is wrong for this slot
// and a prune would revoke the user's own act-as rights.
var ErrNoOwnPartyRight = errors.New("rights: the user holds no " + ActAsKind + " right on the preserved party — refusing to touch this user")

// Right is one entry of a user's rights list. The participant's own JSON
// is kept verbatim so a revoke request hands back exactly the
// representation the participant served.
type Right struct {
	raw   json.RawMessage
	kind  string
	party string
}

// Kind returns the right's discriminator, such as "CanActAs" or
// "ParticipantAdmin".
func (r Right) Kind() string { return r.kind }

// Party returns the party the right is scoped to, or "" for kinds that
// carry no party.
func (r Right) Party() string { return r.party }

// UnmarshalJSON decodes one entry of the participant's rights list.
func (r *Right) UnmarshalJSON(data []byte) error {
	var envelope struct {
		Kind map[string]json.RawMessage `json:"kind"`
	}
	if err := json.Unmarshal(data, &envelope); err != nil {
		return fmt.Errorf("rights: decode right: %w", err)
	}
	if len(envelope.Kind) != 1 {
		return fmt.Errorf("rights: expected exactly one kind per right, got %d", len(envelope.Kind))
	}
	r.raw = append(json.RawMessage(nil), data...)
	for kind, body := range envelope.Kind {
		party, err := partyOf(body)
		if kind == ActAsKind && err != nil {
			return fmt.Errorf("rights: could not read the party of a %s right: %w", ActAsKind, err)
		}
		r.kind = kind
		r.party = party
	}
	return nil
}

// MarshalJSON returns the participant's original document for the right.
func (r Right) MarshalJSON() ([]byte, error) {
	if len(r.raw) == 0 {
		return nil, errors.New("rights: right was not decoded from a participant response")
	}
	return r.raw, nil
}

func partyOf(body json.RawMessage) (string, error) {
	var scoped struct {
		Value struct {
			Party string `json:"party"`
		} `json:"value"`
	}
	if err := json.Unmarshal(body, &scoped); err != nil {
		return "", err
	}
	if scoped.Value.Party == "" {
		return "", errors.New("no party at value.party")
	}
	return scoped.Value.Party, nil
}

// Policy names what a prune preserves and the bound on how much it may
// revoke at once. PartyPrefix is the validator's own party, or a prefix
// of it, such as "a-validator-1::1220ab" or "a-validator-1::".
type Policy struct {
	PartyPrefix string
	MaxRevoke   int
}

// NewPolicy builds a Policy, rejecting a non-empty partyPrefix that lacks
// "::" — a bare hint would also match parties that merely start with it.
// An empty partyPrefix is valid: it preserves nothing but the admin right.
func NewPolicy(partyPrefix string, maxRevoke int) (Policy, error) {
	if partyPrefix != "" && !strings.Contains(partyPrefix, "::") {
		return Policy{}, fmt.Errorf("rights: party prefix %q must contain '::' — a bare hint also matches parties that merely start with it", partyPrefix)
	}
	return Policy{PartyPrefix: partyPrefix, MaxRevoke: maxRevoke}, nil
}

func (p Policy) preserves(r Right) bool {
	if r.kind == AdminKind {
		return true
	}
	return r.kind == ActAsKind && p.PartyPrefix != "" && strings.HasPrefix(r.party, p.PartyPrefix)
}

// Plan is the split Classify reached over a user's rights.
type Plan struct {
	Keep   []Right
	Revoke []Right
}

// Classify splits held into the rights the policy preserves and
// everything else, which a prune revokes.
func Classify(held []Right, policy Policy) Plan {
	plan := Plan{}
	for _, right := range held {
		if policy.preserves(right) {
			plan.Keep = append(plan.Keep, right)
			continue
		}
		plan.Revoke = append(plan.Revoke, right)
	}
	return plan
}

// Validate reports whether the plan is safe to execute. The user must
// hold the admin right, must hold an act-as right on the preserved party
// — its absence means the preserved party is wrong for this slot — and
// the revoke list must stay within the policy's bound.
func (p Plan) Validate(policy Policy) error {
	if !p.Keeps(AdminKind) {
		return ErrNoAdminRight
	}
	if !p.Keeps(ActAsKind) {
		return fmt.Errorf("%w: %s*", ErrNoOwnPartyRight, policy.PartyPrefix)
	}
	if len(p.Revoke) > policy.MaxRevoke {
		return fmt.Errorf("rights: %d rights to revoke exceeds the %d bound", len(p.Revoke), policy.MaxRevoke)
	}
	return nil
}

// Keeps reports whether the plan preserves a right of the given kind.
func (p Plan) Keeps(kind string) bool {
	for _, right := range p.Keep {
		if right.kind == kind {
			return true
		}
	}
	return false
}

type rightIdentity struct {
	kind  string
	party string
}

func (r Right) identity() rightIdentity {
	return rightIdentity{kind: r.kind, party: r.party}
}

// NewSince returns the rights p would revoke that consented did not
// list, identified by kind and party. A prune revokes only what the
// operator agreed to, so a non-empty result means consent no longer
// covers the live plan.
func (p Plan) NewSince(consented Plan) []Right {
	agreed := make(map[rightIdentity]bool, len(consented.Revoke))
	for _, right := range consented.Revoke {
		agreed[right.identity()] = true
	}
	var added []Right
	for _, right := range p.Revoke {
		if !agreed[right.identity()] {
			added = append(added, right)
		}
	}
	return added
}

// Group counts the rights sharing one kind and one party family.
type Group struct {
	Kind   string
	Family string
	Count  int
}

// RevokeGroups counts the plan's revocable rights by kind and party
// family, most numerous first, so a list of hundreds of ephemeral
// parties reads as a handful of lines.
func (p Plan) RevokeGroups() []Group {
	counts := map[Group]int{}
	for _, right := range p.Revoke {
		counts[Group{Kind: right.kind, Family: Family(right.party)}]++
	}
	groups := make([]Group, 0, len(counts))
	for group, count := range counts {
		group.Count = count
		groups = append(groups, group)
	}
	sort.Slice(groups, func(i, j int) bool {
		if groups[i].Count != groups[j].Count {
			return groups[i].Count > groups[j].Count
		}
		if groups[i].Kind != groups[j].Kind {
			return groups[i].Kind < groups[j].Kind
		}
		return groups[i].Family < groups[j].Family
	})
	return groups
}

var ephemeralPartySuffix = regexp.MustCompile(`-[0-9a-f]{8,}::.*$`)

// Family reduces a party to the hint its allocator used, so the
// per-test parties of one suite — grpc-writer-parity-4f2a9c31::1220ab…,
// grpc-writer-parity-9b1e07d4::1220ab… — collapse onto
// "grpc-writer-parity-*". Parties without a random suffix keep their
// bare hint.
func Family(party string) string {
	if party == "" {
		return ""
	}
	if collapsed := ephemeralPartySuffix.ReplaceAllString(party, ""); collapsed != party {
		return collapsed + "-*"
	}
	hint, _, found := strings.Cut(party, "::")
	if !found {
		return party
	}
	return hint
}
