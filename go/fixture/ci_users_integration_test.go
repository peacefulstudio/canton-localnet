// Copyright (c) 2026 Peaceful Studio OÜ
// SPDX-License-Identifier: Apache-2.0

//go:build integration

package fixture

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"testing"
	"time"
)

// ciUser pins one a-validator-1 CI client's identity to its generated
// literals, so this test fails the moment the compose fixtures and this
// assertion drift apart.
type ciUser struct {
	label        string
	clientID     string
	clientSecret string
	userID       string
}

func aValidator1CIUsers() []ciUser {
	return []ciUser{
		{label: "ci-1", clientID: "a-validator-1-ci-1", clientSecret: "VSLk2bzpSPKIY8qGsQuY4uYdkdoccUG6", userID: "cfc83af9-a97e-4e5b-85a1-b0c4cc49323c"},
		{label: "ci-2", clientID: "a-validator-1-ci-2", clientSecret: "TdlQDk2xcnLKOcRguN6GyqWUYwa2X0Wn", userID: "33f30684-7067-4cd6-8dea-4c3230a69108"},
		{label: "ci-3", clientID: "a-validator-1-ci-3", clientSecret: "Ar8crPYkvmVLgnUM2Jek2wo4UyLBdTLA", userID: "432a9ded-d191-4472-acfc-7af32e4124b3"},
		{label: "ci-4", clientID: "a-validator-1-ci-4", clientSecret: "WZmCWc4IWmIEKwWsZ97i7QRIMOUtZWkO", userID: "69752f9d-ff24-4f23-a9ec-54fe687e1c1c"},
		{label: "ci-provider", clientID: "a-validator-1-ci-provider", clientSecret: "TtfzDHtIDi9ZE6j6rtIwMFXdVEWxfoPk", userID: "926625a4-2c3c-4bda-8d47-b1e16ac6fd2a"},
		{label: "ci-app", clientID: "a-validator-1-ci-app", clientSecret: "reBRG5CicGVyv80DXph5QpJRCoTJe4iT", userID: "1fa377f9-7153-49fc-8ab4-2582df27be87"},
	}
}

// aValidator1ValidatorUserID is the fixed Keycloak service-account user id
// of the a-validator-1-validator client (AUTH_A_VALIDATOR_1_VALIDATOR_USER_ID
// in compose/modules/keycloak/env/a-validator-1/on/oauth2.env) — the
// participant-admin user whose primary party is the operator.
const aValidator1ValidatorUserID = "c87743ab-80e0-4b83-935a-4c0582226691"

// TestCIUsers_HaveOperatorAsPrimaryPartyAndExactlyBaseRights is the L3
// acceptance check from the shared CI LocalNet spec (§5): each CI client's
// service-account user carries the operator as its primary party, and
// holds exactly the base-rights bundle (ParticipantAdmin, CanActAs and
// CanReadAs on the operator) — no more, no less.
func TestCIUsers_HaveOperatorAsPrimaryPartyAndExactlyBaseRights(t *testing.T) {
	skipIfStackUnreachable(t, RoleAValidator1)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	operator, err := New(Config{Role: RoleAValidator1})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if err := operator.Setup(ctx); err != nil {
		t.Fatalf("Setup: %v", err)
	}
	t.Cleanup(func() { _ = operator.Teardown(context.Background()) })

	operatorParty, err := operator.Admin().primaryParty(ctx, aValidator1ValidatorUserID)
	if err != nil {
		t.Fatalf("resolve operator primary party: %v", err)
	}
	if operatorParty == "" {
		t.Fatal("operator primary party is empty")
	}

	baseEndpoints := operator.Endpoints()
	for _, u := range aValidator1CIUsers() {
		u := u
		t.Run(u.label, func(t *testing.T) {
			tokens, err := NewOAuth2TokenProvider(OAuth2TokenProviderConfig{
				TokenURL:     baseEndpoints.TokenURL,
				ClientID:     u.clientID,
				ClientSecret: u.clientSecret,
				Audience:     baseEndpoints.Audience,
				Scope:        baseEndpoints.Scope,
			})
			if err != nil {
				t.Fatalf("NewOAuth2TokenProvider(%s): %v", u.clientID, err)
			}
			admin, err := NewJsonLedgerAdminClient(baseEndpoints.JSONLedgerAPIURL, tokens)
			if err != nil {
				t.Fatalf("NewJsonLedgerAdminClient(%s): %v", u.clientID, err)
			}

			gotParty, err := admin.primaryParty(ctx, u.userID)
			if err != nil {
				t.Fatalf("%s: primaryParty: %v", u.clientID, err)
			}
			if gotParty != operatorParty {
				t.Fatalf("%s: primary party = %q, want operator party %q", u.clientID, gotParty, operatorParty)
			}

			rights, err := admin.listRights(ctx, u.userID)
			if err != nil {
				t.Fatalf("%s: listRights: %v", u.clientID, err)
			}
			gotKinds := rightKindsAgainst(rights, operatorParty)
			wantKinds := []string{"ParticipantAdmin", "CanActAs:" + operatorParty, "CanReadAs:" + operatorParty}
			sort.Strings(gotKinds)
			sort.Strings(wantKinds)
			if fmt.Sprint(gotKinds) != fmt.Sprint(wantKinds) {
				t.Fatalf("%s: rights = %v, want exactly %v", u.clientID, gotKinds, wantKinds)
			}

			if _, err := admin.GetParticipantId(ctx); err != nil {
				t.Fatalf("%s: token was rejected calling GET /v2/parties/participant-id: %v", u.clientID, err)
			}
		})
	}
}

func rightKindsAgainst(rights []rawRight, operatorParty string) []string {
	kinds := make([]string, 0, len(rights))
	for _, r := range rights {
		if r.party != "" {
			kinds = append(kinds, r.kind+":"+r.party)
			continue
		}
		kinds = append(kinds, r.kind)
	}
	return kinds
}

type rawRight struct {
	kind  string
	party string
}

func (c *JsonLedgerAdminClient) primaryParty(ctx context.Context, userID string) (string, error) {
	var out struct {
		User struct {
			PrimaryParty string `json:"primaryParty"`
		} `json:"user"`
	}
	if err := c.doJSON(ctx, "GET", "/v2/users/"+userID, nil, &out); err != nil {
		return "", err
	}
	return out.User.PrimaryParty, nil
}

func (c *JsonLedgerAdminClient) listRights(ctx context.Context, userID string) ([]rawRight, error) {
	var out struct {
		Rights []struct {
			Kind map[string]json.RawMessage `json:"kind"`
		} `json:"rights"`
	}
	if err := c.doJSON(ctx, "GET", "/v2/users/"+userID+"/rights", nil, &out); err != nil {
		return nil, err
	}
	rights := make([]rawRight, 0, len(out.Rights))
	for _, r := range out.Rights {
		for kind, raw := range r.Kind {
			right := rawRight{kind: kind}
			var withParty struct {
				Value struct {
					Party string `json:"party"`
				} `json:"value"`
			}
			if json.Unmarshal(raw, &withParty) == nil {
				right.party = withParty.Value.Party
			}
			rights = append(rights, right)
		}
	}
	return rights, nil
}
