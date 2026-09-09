// Copyright (c) 2026 Peaceful Studio OÜ
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

const (
	rightsTestToken  = "bearer-that-must-never-be-printed"
	rightsTestSecret = "client-secret-that-must-never-be-printed"
	rightsTestUserID = "validator-user-1"
	validatorParty   = "a-validator-1::1220abcd"
	svValidatorParty = "sv::1220abcd"
	adminRight       = `{"kind":{"ParticipantAdmin":{"value":{}}}}`
	noPrimaryParty   = ""
)

func actAs(party string) string {
	return `{"kind":{"CanActAs":{"value":{"party":"` + party + `"}}}}`
}

func readAs(party string) string {
	return `{"kind":{"CanReadAs":{"value":{"party":"` + party + `"}}}}`
}

type participantCall struct {
	method string
	path   string
	body   string
}

type fakeParticipant struct {
	mu            sync.Mutex
	primaryParty  string
	rightsPerGet  [][]string
	gets          int
	calls         []participantCall
	revokesAtMost int
}

func participantHolding(primaryParty string, held ...string) *fakeParticipant {
	return &fakeParticipant{primaryParty: primaryParty, rightsPerGet: [][]string{held}}
}

func (p *fakeParticipant) thenHolding(held ...string) *fakeParticipant {
	p.rightsPerGet = append(p.rightsPerGet, held)
	return p
}

func (p *fakeParticipant) revokingAtMost(n int) *fakeParticipant {
	p.revokesAtMost = n
	return p
}

func (p *fakeParticipant) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	body, _ := io.ReadAll(r.Body)
	p.mu.Lock()
	defer p.mu.Unlock()
	p.calls = append(p.calls, participantCall{method: r.Method, path: r.URL.Path, body: string(body)})

	if !strings.HasSuffix(r.URL.Path, "/rights") {
		_, _ = w.Write([]byte(`{"user":{"id":"` + rightsTestUserID + `","primaryParty":"` + p.primaryParty + `"}}`))
		return
	}
	if r.Method == http.MethodPatch {
		var request struct {
			Rights []json.RawMessage `json:"rights"`
		}
		_ = json.Unmarshal(body, &request)
		revoked := request.Rights
		if p.revokesAtMost > 0 && p.revokesAtMost < len(revoked) {
			revoked = revoked[:p.revokesAtMost]
		}
		p.rightsPerGet = [][]string{nil}
		p.gets = 0
		response, _ := json.Marshal(map[string]any{"newlyRevokedRights": revoked})
		_, _ = w.Write(response)
		return
	}
	index := min(p.gets, len(p.rightsPerGet)-1)
	p.gets++
	_, _ = w.Write([]byte(`{"rights":[` + strings.Join(p.rightsPerGet[index], ",") + `]}`))
}

func (p *fakeParticipant) recorded() []participantCall {
	p.mu.Lock()
	defer p.mu.Unlock()
	return append([]participantCall(nil), p.calls...)
}

func (p *fakeParticipant) rightsCalls() []string {
	var methods []string
	for _, call := range p.recorded() {
		if strings.HasSuffix(call.path, "/rights") {
			methods = append(methods, call.method)
		}
	}
	return methods
}

func (p *fakeParticipant) patches() []participantCall {
	var out []participantCall
	for _, call := range p.recorded() {
		if call.method == http.MethodPatch {
			out = append(out, call)
		}
	}
	return out
}

func startFakeSlot(t *testing.T, slotName string, participant *fakeParticipant) {
	t.Helper()
	keycloak := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"access_token":"` + rightsTestToken + `","token_type":"Bearer","expires_in":300}`))
	}))
	t.Cleanup(keycloak.Close)
	ledger := httptest.NewServer(participant)
	t.Cleanup(ledger.Close)

	keycloakHost, keycloakPort := splitHostPort(t, keycloak.URL)
	ledgerHost, ledgerPort := splitHostPort(t, ledger.URL)
	if keycloakHost != ledgerHost {
		t.Fatalf("httptest invariant broken: keycloak=%s ledger=%s — both must share a host", keycloakHost, ledgerHost)
	}
	prefix := "CANTON_LOCALNET_" + strings.ToUpper(strings.ReplaceAll(slotName, "-", "_")) + "_"
	t.Setenv("CANTON_LOCALNET_HOST", keycloakHost)
	t.Setenv("CANTON_LOCALNET_KEYCLOAK_PORT", keycloakPort)
	t.Setenv(prefix+"JSON_PORT", ledgerPort)
	t.Setenv(prefix+"CLIENT_SECRET", rightsTestSecret)
	t.Setenv(prefix+"USER_ID", rightsTestUserID)
}

func startFakeSlotA(t *testing.T, participant *fakeParticipant) {
	t.Helper()
	startFakeSlot(t, "a-validator-1", participant)
}

type rightsRun struct {
	stdout string
	stderr string
	err    error
	code   int
}

func runRightsOnTTY(t *testing.T, isTTY bool, stdin string, args ...string) rightsRun {
	t.Helper()
	_, factory := newFakeRunnerFactory()
	root := newRootCommandWithDeps(factory, defaultVMDeps(), rightsDeps{isTerminal: func() bool { return isTTY }})
	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	root.SetOut(stdout)
	root.SetErr(stderr)
	root.SetIn(strings.NewReader(stdin))
	root.SetArgs(args)
	err := root.Execute()
	run := rightsRun{stdout: stdout.String(), stderr: stderr.String(), err: err}
	if err != nil {
		run.code = exitCode(err)
	}
	return run
}

func runRights(t *testing.T, args ...string) rightsRun {
	t.Helper()
	return runRightsOnTTY(t, false, "", args...)
}

func TestRightsListGroupsEphemeralParties(t *testing.T) {
	participant := participantHolding(validatorParty,
		adminRight,
		actAs(validatorParty),
		actAs("grpc-writer-parity-4f2a9c31::1220abcd"),
		actAs("grpc-writer-parity-9b1e07d4::1220abcd"),
		readAs("alice::1220abcd"),
	)
	startFakeSlotA(t, participant)

	run := runRights(t, "rights", "list", "--slot", "a")

	if run.err != nil {
		t.Fatalf("execute: %v", run.err)
	}
	for _, want := range []string{
		"user             " + rightsTestUserID,
		"preserving       ParticipantAdmin, and CanActAs on " + validatorParty + "*",
		"preserve source  the user's primary party, read from the participant",
		"total rights     5",
		"preserved        2",
		"keep    CanActAs                 " + validatorParty,
		"to revoke        3",
		"revoke     2  CanActAs                 grpc-writer-parity-*",
		"revoke     1  CanReadAs                alice",
	} {
		if !strings.Contains(run.stdout, want) {
			t.Errorf("stdout missing %q:\n%s", want, run.stdout)
		}
	}
	if len(participant.patches()) != 0 {
		t.Errorf("list must not write: %+v", participant.patches())
	}
}

func TestRightsListFullNamesEveryRevocableParty(t *testing.T) {
	participant := participantHolding(validatorParty,
		adminRight,
		actAs(validatorParty),
		actAs("grpc-writer-parity-4f2a9c31::1220abcd"),
		actAs("grpc-writer-parity-9b1e07d4::1220abcd"),
	)
	startFakeSlotA(t, participant)

	run := runRights(t, "rights", "list", "--slot", "a", "--full")

	if run.err != nil {
		t.Fatalf("execute: %v", run.err)
	}
	for _, want := range []string{
		"revoke  CanActAs                 grpc-writer-parity-4f2a9c31::1220abcd",
		"revoke  CanActAs                 grpc-writer-parity-9b1e07d4::1220abcd",
	} {
		if !strings.Contains(run.stdout, want) {
			t.Errorf("--full must name the party ids, missing %q:\n%s", want, run.stdout)
		}
	}
	if strings.Contains(run.stdout, "grpc-writer-parity-*") {
		t.Errorf("--full must not also print the grouped summary:\n%s", run.stdout)
	}
}

func TestRightsPruneWithoutYesPrintsThePlanAndExitsDeclined(t *testing.T) {
	participant := participantHolding(validatorParty, adminRight, actAs(validatorParty), actAs("alice-4f2a9c31::1220abcd"))
	startFakeSlotA(t, participant)

	run := runRights(t, "rights", "prune", "--slot", "a")

	if run.err == nil {
		t.Fatal("a non-interactive run that changed nothing must not report success")
	}
	if run.code != 2 {
		t.Errorf("exit code = %d, want 2", run.code)
	}
	if !strings.Contains(run.err.Error(), "--yes") {
		t.Errorf("error should say how to proceed, got %v", run.err)
	}
	if len(participant.patches()) != 0 {
		t.Fatalf("the plan-only path must issue no PATCH: %+v", participant.patches())
	}
}

func TestRightsPruneDryRunExitsZeroWithoutWriting(t *testing.T) {
	participant := participantHolding(validatorParty, adminRight, actAs(validatorParty), actAs("alice-4f2a9c31::1220abcd"))
	startFakeSlotA(t, participant)

	run := runRights(t, "rights", "prune", "--slot", "a", "--dry-run")

	if run.err != nil {
		t.Fatalf("execute: %v", run.err)
	}
	if !strings.Contains(run.stdout, "passes every check") {
		t.Errorf("stdout should report the plan is executable:\n%s", run.stdout)
	}
	if len(participant.patches()) != 0 {
		t.Fatalf("--dry-run must issue no PATCH: %+v", participant.patches())
	}
}

func TestRightsPruneDryRunFailsOnAPlanThatWouldBeRefused(t *testing.T) {
	participant := participantHolding(validatorParty, actAs(validatorParty), actAs("alice-4f2a9c31::1220abcd"))
	startFakeSlotA(t, participant)

	run := runRights(t, "rights", "prune", "--slot", "a", "--dry-run")

	if run.err == nil {
		t.Fatal("--dry-run must exit non-zero when the plan would be refused")
	}
	if run.code != 1 {
		t.Errorf("exit code = %d, want 1", run.code)
	}
}

func TestRightsPruneRejectsYesWithDryRun(t *testing.T) {
	participant := participantHolding(validatorParty, adminRight, actAs(validatorParty))
	startFakeSlotA(t, participant)

	run := runRights(t, "rights", "prune", "--slot", "a", "--yes", "--dry-run")

	if run.err == nil || !strings.Contains(run.err.Error(), "none of the others can be") {
		t.Fatalf("error = %v, want the mutually-exclusive refusal", run.err)
	}
	if len(participant.patches()) != 0 {
		t.Errorf("a rejected flag combination must issue no PATCH: %+v", participant.patches())
	}
}

func TestRightsPruneRevokesOnlyTheUnpreservedRights(t *testing.T) {
	participant := participantHolding(validatorParty,
		adminRight,
		actAs(validatorParty),
		actAs("grpc-writer-parity-4f2a9c31::1220abcd"),
		readAs(validatorParty),
	)
	startFakeSlotA(t, participant)

	run := runRights(t, "rights", "prune", "--slot", "a", "--yes")

	if run.err != nil {
		t.Fatalf("execute: %v", run.err)
	}
	patches := participant.patches()
	if len(patches) != 1 {
		t.Fatalf("expected exactly one PATCH, got %d", len(patches))
	}
	body := patches[0].body
	if !strings.Contains(body, `"userId":"`+rightsTestUserID+`"`) {
		t.Errorf("PATCH body missing the user id: %s", body)
	}
	if !strings.Contains(body, "grpc-writer-parity-4f2a9c31::1220abcd") {
		t.Errorf("PATCH body should revoke the ephemeral party: %s", body)
	}
	if !strings.Contains(body, `"CanReadAs"`) {
		t.Errorf("PATCH body should revoke other right kinds on the validator's own party: %s", body)
	}
	if strings.Contains(body, `"ParticipantAdmin"`) {
		t.Errorf("PATCH body must preserve the admin right: %s", body)
	}
	if strings.Count(body, `"CanActAs"`) != 1 {
		t.Errorf("PATCH body must preserve the validator's own act-as right: %s", body)
	}
	if !strings.Contains(run.stdout, "done. rights revoked: 2.") {
		t.Errorf("stdout should report the revoke count:\n%s", run.stdout)
	}
}

func TestRightsPruneRereadsTheRightsBeforeRevoking(t *testing.T) {
	participant := participantHolding(validatorParty, adminRight, actAs(validatorParty), actAs("alice-4f2a9c31::1220abcd"))
	startFakeSlotA(t, participant)

	if run := runRights(t, "rights", "prune", "--slot", "a", "--yes"); run.err != nil {
		t.Fatalf("execute: %v", run.err)
	}

	want := []string{http.MethodGet, http.MethodGet, http.MethodPatch, http.MethodGet}
	if got := participant.rightsCalls(); strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("rights call sequence = %v, want a live re-read immediately before the PATCH (%v)", got, want)
	}
}

func TestRightsPruneAbortsWhenNewRightsAppearAfterConsent(t *testing.T) {
	participant := participantHolding(validatorParty, adminRight, actAs(validatorParty), actAs("alice-4f2a9c31::1220abcd")).
		thenHolding(adminRight, actAs(validatorParty), actAs("alice-4f2a9c31::1220abcd"), actAs("bob-9b1e07d4::1220abcd"))
	startFakeSlotA(t, participant)

	run := runRights(t, "rights", "prune", "--slot", "a", "--yes")

	if run.err == nil || !strings.Contains(run.err.Error(), "was not consented to") {
		t.Fatalf("error = %v, want the unconsented-growth refusal", run.err)
	}
	if len(participant.patches()) != 0 {
		t.Errorf("a superset must issue no PATCH: %+v", participant.patches())
	}
}

func TestRightsPruneAbortsOnASameSizedButDifferentLiveSet(t *testing.T) {
	participant := participantHolding(validatorParty, adminRight, actAs(validatorParty), actAs("alice-4f2a9c31::1220abcd")).
		thenHolding(adminRight, actAs(validatorParty), actAs("bob-9b1e07d4::1220abcd"))
	startFakeSlotA(t, participant)

	run := runRights(t, "rights", "prune", "--slot", "a", "--yes")

	if run.err == nil || !strings.Contains(run.err.Error(), "was not consented to") {
		t.Fatalf("error = %v — equal counts with different members must not pass", run.err)
	}
	if len(participant.patches()) != 0 {
		t.Errorf("an unconsented member must issue no PATCH: %+v", participant.patches())
	}
}

func TestRightsPruneProceedsWhenTheLiveSetShrank(t *testing.T) {
	participant := participantHolding(validatorParty, adminRight, actAs(validatorParty), actAs("alice-4f2a9c31::1220abcd"), actAs("bob-9b1e07d4::1220abcd")).
		thenHolding(adminRight, actAs(validatorParty), actAs("alice-4f2a9c31::1220abcd"))
	startFakeSlotA(t, participant)

	run := runRights(t, "rights", "prune", "--slot", "a", "--yes")

	if run.err != nil {
		t.Fatalf("execute: %v", run.err)
	}
	if !strings.Contains(run.stdout, "already gone; revoking the rest") {
		t.Errorf("stdout should report the shrink:\n%s", run.stdout)
	}
	patches := participant.patches()
	if len(patches) != 1 || strings.Contains(patches[0].body, "bob-9b1e07d4") {
		t.Errorf("the PATCH must carry only what is still held: %+v", patches)
	}
}

func TestRightsPruneRefusesAUserWithoutTheAdminRight(t *testing.T) {
	participant := participantHolding(validatorParty, actAs(validatorParty), actAs("alice-4f2a9c31::1220abcd"))
	startFakeSlotA(t, participant)

	run := runRights(t, "rights", "prune", "--slot", "a", "--yes")

	if run.err == nil || !strings.Contains(run.err.Error(), "no ParticipantAdmin right") {
		t.Fatalf("error = %v, want the no-ParticipantAdmin refusal", run.err)
	}
	if len(participant.patches()) != 0 {
		t.Errorf("a refused prune must issue no PATCH: %+v", participant.patches())
	}
}

func TestRightsPruneRefusesWhenThePreservedPartyIsWrongForTheSlot(t *testing.T) {
	participant := participantHolding(noPrimaryParty, adminRight, actAs(svValidatorParty), actAs("alice-4f2a9c31::1220abcd"))
	startFakeSlot(t, "sv-validator-1", participant)

	run := runRights(t, "rights", "prune", "--slot", "sv", "--yes")

	if run.err == nil || !strings.Contains(run.err.Error(), "no CanActAs right on the preserved party") {
		t.Fatalf("error = %v, want the wrong-preserved-party refusal", run.err)
	}
	if !strings.Contains(run.err.Error(), "sv-validator-1::") {
		t.Errorf("error should name the prefix it fell back to, got %v", run.err)
	}
	if len(participant.patches()) != 0 {
		t.Errorf("the sv slot must not have its own party revoked: %+v", participant.patches())
	}
}

func TestRightsPrunePreservesTheSVPartyTheParticipantReports(t *testing.T) {
	participant := participantHolding(svValidatorParty, adminRight, actAs(svValidatorParty), actAs("alice-4f2a9c31::1220abcd"))
	startFakeSlot(t, "sv-validator-1", participant)

	run := runRights(t, "rights", "prune", "--slot", "sv", "--yes")

	if run.err != nil {
		t.Fatalf("execute: %v", run.err)
	}
	patches := participant.patches()
	if len(patches) != 1 {
		t.Fatalf("expected exactly one PATCH, got %d", len(patches))
	}
	if strings.Contains(patches[0].body, svValidatorParty) {
		t.Errorf("the sv validator's own party must be preserved: %s", patches[0].body)
	}
	if !strings.Contains(run.stdout, "preserving       ParticipantAdmin, and CanActAs on "+svValidatorParty) {
		t.Errorf("the plan should name the party read from the participant:\n%s", run.stdout)
	}
}

func TestRightsPrunePreservePartyMustBeAFullPartyID(t *testing.T) {
	participant := participantHolding(validatorParty, adminRight, actAs(validatorParty))
	startFakeSlotA(t, participant)

	run := runRights(t, "rights", "prune", "--slot", "a", "--preserve-party", "a-validator-1", "--yes")

	if run.err == nil || !strings.Contains(run.err.Error(), "must be a full party id") {
		t.Fatalf("error = %v, want the bare-hint refusal", run.err)
	}
}

func TestRightsPrunePreservePartyStillFacesTheRail(t *testing.T) {
	participant := participantHolding(validatorParty, adminRight, actAs(validatorParty), actAs("alice-4f2a9c31::1220abcd"))
	startFakeSlotA(t, participant)

	run := runRights(t, "rights", "prune", "--slot", "a", "--preserve-party", "typo::1220abcd", "--yes")

	if run.err == nil || !strings.Contains(run.err.Error(), "no CanActAs right on the preserved party") {
		t.Fatalf("error = %v — a --preserve-party the user does not hold must refuse", run.err)
	}
	if len(participant.patches()) != 0 {
		t.Errorf("a refused prune must issue no PATCH: %+v", participant.patches())
	}
}

func TestRightsPrunePreservePartyOverridesTheParticipant(t *testing.T) {
	participant := participantHolding(noPrimaryParty, adminRight, actAs("chosen::1220abcd"), actAs("alice-4f2a9c31::1220abcd"))
	startFakeSlotA(t, participant)

	run := runRights(t, "rights", "prune", "--slot", "a", "--preserve-party", "chosen::1220abcd", "--yes")

	if run.err != nil {
		t.Fatalf("execute: %v", run.err)
	}
	if !strings.Contains(run.stdout, "preserve source  --preserve-party") {
		t.Errorf("the plan should name the override as its source:\n%s", run.stdout)
	}
	if strings.Contains(participant.patches()[0].body, "chosen::1220abcd") {
		t.Errorf("the named party must be preserved: %s", participant.patches()[0].body)
	}
}

func TestRightsPruneAbortsAboveMaxRevoke(t *testing.T) {
	participant := participantHolding(validatorParty,
		adminRight,
		actAs(validatorParty),
		actAs("alice-4f2a9c31::1220abcd"),
		actAs("bob-9b1e07d4::1220abcd"),
	)
	startFakeSlotA(t, participant)

	run := runRights(t, "rights", "prune", "--slot", "a", "--max-revoke", "1", "--yes")

	if run.err == nil || !strings.Contains(run.err.Error(), "exceeds the 1 bound") {
		t.Fatalf("error = %v, want the revoke bound", run.err)
	}
	if len(participant.patches()) != 0 {
		t.Errorf("a bounded prune must issue no PATCH: %+v", participant.patches())
	}
}

func TestRightsPruneOnACleanUserSucceedsWithoutWriting(t *testing.T) {
	participant := participantHolding(validatorParty, adminRight, actAs(validatorParty))
	startFakeSlotA(t, participant)

	run := runRights(t, "rights", "prune", "--slot", "a", "--yes")

	if run.err != nil {
		t.Fatalf("execute: %v", run.err)
	}
	if !strings.Contains(run.stdout, "already clean") {
		t.Errorf("stdout should report the user is clean:\n%s", run.stdout)
	}
	if strings.Contains(run.stdout, "warning:") {
		t.Errorf("a genuinely clean user should carry no warning:\n%s", run.stdout)
	}
	if len(participant.patches()) != 0 {
		t.Errorf("nothing to revoke must issue no PATCH: %+v", participant.patches())
	}
}

func TestRightsPruneWarnsWhenACleanUserHoldsNoAdminRight(t *testing.T) {
	participant := participantHolding(validatorParty, actAs(validatorParty))
	startFakeSlotA(t, participant)

	run := runRights(t, "rights", "prune", "--slot", "a", "--yes")

	if run.err != nil {
		t.Fatalf("a pristine but wrongly-targeted user must not be an error: %v", run.err)
	}
	if !strings.Contains(run.stdout, "may not be the validator user") {
		t.Errorf("a user with no admin right must not read as a clean bill of health:\n%s", run.stdout)
	}
	if len(participant.patches()) != 0 {
		t.Errorf("nothing to revoke must issue no PATCH: %+v", participant.patches())
	}
}

func TestRightsPruneWarnsWhenACleanUserHoldsNoOwnPartyRight(t *testing.T) {
	participant := participantHolding(noPrimaryParty, adminRight)
	startFakeSlot(t, "sv-validator-1", participant)

	run := runRights(t, "rights", "prune", "--slot", "sv", "--yes")

	if run.err != nil {
		t.Fatalf("a pristine participant must not be an error: %v", run.err)
	}
	if !strings.Contains(run.stdout, "the preserved party may be wrong for this slot") {
		t.Errorf("an empty user must not read as a clean bill of health:\n%s", run.stdout)
	}
}

func TestRightsPruneFailsLoudlyOnAPartialRevoke(t *testing.T) {
	participant := participantHolding(validatorParty,
		adminRight,
		actAs(validatorParty),
		actAs("alice-4f2a9c31::1220abcd"),
		actAs("bob-9b1e07d4::1220abcd"),
	).revokingAtMost(1)
	startFakeSlotA(t, participant)

	run := runRights(t, "rights", "prune", "--slot", "a", "--yes")

	if run.err == nil || !strings.Contains(run.err.Error(), "revoked 1 of 2") {
		t.Fatalf("error = %v, want the partial-revoke failure", run.err)
	}
}

func TestRightsPruneConfirmsOnATerminal(t *testing.T) {
	participant := participantHolding(validatorParty, adminRight, actAs(validatorParty), actAs("alice-4f2a9c31::1220abcd"))
	startFakeSlotA(t, participant)

	run := runRightsOnTTY(t, true, "revoke\n", "rights", "prune", "--slot", "a")

	if run.err != nil {
		t.Fatalf("execute: %v", run.err)
	}
	if !strings.Contains(run.stdout, "Type 'revoke' to proceed") {
		t.Errorf("a terminal run should prompt:\n%s", run.stdout)
	}
	if len(participant.patches()) != 1 {
		t.Errorf("a confirmed prune must revoke: %+v", participant.patches())
	}
}

func TestRightsPruneAbortsOnAWrongAnswer(t *testing.T) {
	participant := participantHolding(validatorParty, adminRight, actAs(validatorParty), actAs("alice-4f2a9c31::1220abcd"))
	startFakeSlotA(t, participant)

	run := runRightsOnTTY(t, true, "no\n", "rights", "prune", "--slot", "a")

	if run.err == nil {
		t.Fatal("a declined prune must not report success")
	}
	if run.code != 2 {
		t.Errorf("exit code = %d, want 2", run.code)
	}
	if len(participant.patches()) != 0 {
		t.Errorf("a declined prune must issue no PATCH: %+v", participant.patches())
	}
}

func TestRightsPruneAbortsOnAPromptThatSeesNoAnswer(t *testing.T) {
	participant := participantHolding(validatorParty, adminRight, actAs(validatorParty), actAs("alice-4f2a9c31::1220abcd"))
	startFakeSlotA(t, participant)

	run := runRightsOnTTY(t, true, "", "rights", "prune", "--slot", "a")

	if run.err == nil || run.code != 2 {
		t.Fatalf("a PTY with nobody behind it must exit 2, got err=%v code=%d", run.err, run.code)
	}
	if len(participant.patches()) != 0 {
		t.Errorf("an unanswered prompt must issue no PATCH: %+v", participant.patches())
	}
}

func TestRightsPruneYesSkipsThePrompt(t *testing.T) {
	participant := participantHolding(validatorParty, adminRight, actAs(validatorParty), actAs("alice-4f2a9c31::1220abcd"))
	startFakeSlotA(t, participant)

	run := runRightsOnTTY(t, true, "", "rights", "prune", "--slot", "a", "--yes")

	if run.err != nil {
		t.Fatalf("execute: %v", run.err)
	}
	if strings.Contains(run.stdout, "Type 'revoke' to proceed") {
		t.Errorf("--yes must not prompt:\n%s", run.stdout)
	}
	if len(participant.patches()) != 1 {
		t.Errorf("--yes must revoke: %+v", participant.patches())
	}
}

func TestRightsPruneNeverPrintsTheCredentials(t *testing.T) {
	participant := participantHolding(validatorParty, adminRight, actAs(validatorParty), actAs("alice-4f2a9c31::1220abcd"))
	startFakeSlotA(t, participant)

	run := runRights(t, "rights", "prune", "--slot", "a", "--yes")

	if run.err != nil {
		t.Fatalf("execute: %v", run.err)
	}
	for stream, text := range map[string]string{"stdout": run.stdout, "stderr": run.stderr} {
		if strings.Contains(text, rightsTestToken) || strings.Contains(text, rightsTestSecret) {
			t.Errorf("%s leaked a credential:\n%s", stream, text)
		}
	}
}

func TestRightsListReportsAnUnreachableParticipant(t *testing.T) {
	startFakeSlotA(t, participantHolding(validatorParty))
	t.Setenv("CANTON_LOCALNET_A_VALIDATOR_1_JSON_PORT", "1")

	run := runRights(t, "rights", "list", "--slot", "a")

	if run.err == nil {
		t.Fatal("expected an error against a dead port")
	}
	if strings.Contains(run.err.Error(), rightsTestToken) || strings.Contains(run.err.Error(), rightsTestSecret) {
		t.Errorf("error leaked a credential: %v", run.err)
	}
}

func TestRunDrivesTheProcessStatusForAnUnauthorisedSweep(t *testing.T) {
	participant := participantHolding(validatorParty, adminRight, actAs(validatorParty), actAs("alice-4f2a9c31::1220abcd"))
	startFakeSlotA(t, participant)

	code := run(context.Background(), []string{"rights", "prune", "--slot", "a"}, io.Discard, io.Discard)

	if code != 2 {
		t.Errorf("process status = %d, want 2", code)
	}
	if len(participant.patches()) != 0 {
		t.Errorf("an unauthorised sweep must issue no PATCH: %+v", participant.patches())
	}
}

func TestRunDrivesTheProcessStatusForARefusal(t *testing.T) {
	participant := participantHolding(validatorParty, actAs(validatorParty), actAs("alice-4f2a9c31::1220abcd"))
	startFakeSlotA(t, participant)

	code := run(context.Background(), []string{"rights", "prune", "--slot", "a", "--yes"}, io.Discard, io.Discard)

	if code != 1 {
		t.Errorf("process status = %d, want 1", code)
	}
}

func TestRunDrivesTheProcessStatusForASuccessfulSweep(t *testing.T) {
	participant := participantHolding(validatorParty, adminRight, actAs(validatorParty), actAs("alice-4f2a9c31::1220abcd"))
	startFakeSlotA(t, participant)

	code := run(context.Background(), []string{"rights", "prune", "--slot", "a", "--yes"}, io.Discard, io.Discard)

	if code != 0 {
		t.Errorf("process status = %d, want 0", code)
	}
	if len(participant.patches()) != 1 {
		t.Errorf("an authorised sweep must revoke: %+v", participant.patches())
	}
}

func TestRightsPruneDefaultsMaxRevokeToTheParticipantCap(t *testing.T) {
	t.Parallel()
	flag := newRightsPruneCommand(defaultRightsDeps()).Flags().Lookup("max-revoke")

	if flag == nil {
		t.Fatal("prune has no --max-revoke flag")
	}
	if flag.DefValue != "1000" {
		t.Errorf("--max-revoke default = %s, want 1000 — a lower bound would abort the saturated-participant recovery this tool exists for", flag.DefValue)
	}
}
