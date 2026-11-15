// Copyright (c) 2026 Peaceful Studio OÜ
// SPDX-License-Identifier: Apache-2.0

package restartguard

import (
	"context"
	"strings"
	"testing"
)

const spliceTwice = "/canton always 0 2026-11-11T11:50:01Z\n/splice always 2 2026-11-11T11:50:02Z\n"

func classifySplice(inspect, events, log string) Finding {
	docker := &scriptedDocker{psOutput: "canton\nsplice\n", inspectOutput: inspect, eventsOutput: events, logs: map[string]string{"splice": log}}
	return Classify(context.Background(), docker.run, "localnet", until)
}

func TestClassifyAcceptsOneJvmCrashThatEveryValidatorBackendLogged(t *testing.T) {
	log := banner + causedBy + initLine + initLine + initLine + banner + "ready\n"

	finding := classifySplice(spliceOnce, "die splice 1\n", log)

	if finding.Classification != SelfHealedSpliceInit {
		t.Fatalf("want SelfHealedSpliceInit, got %+v", finding)
	}
}

func TestClassifyRejectsALaterOutOfMemoryCrashHiddenBehindOneCrashsManyInitializationLines(t *testing.T) {
	log := banner + initLine + initLine + initLine + banner + "Caused by: java.lang.OutOfMemoryError: Java heap space\n" + banner + "ready\n"

	finding := classifySplice(spliceTwice, "die splice 1\ndie splice 137\n", log)

	if finding.Classification != Degraded {
		t.Fatalf("want Degraded, got %+v", finding)
	}
}

func TestClassifyRejectsAnOutOfMemoryErrorInTheSegmentThatIsRunningNow(t *testing.T) {
	log := banner + initLine + banner + "java.lang.OutOfMemoryError: GC overhead limit exceeded\nready\n"

	finding := classifySplice(spliceOnce, "die splice 1\n", log)

	if finding.Classification != Degraded {
		t.Fatalf("want Degraded, got %+v", finding)
	}
}

func TestClassifyRejectsAnExitedSegmentThatNeverLoggedTheInitializationFailure(t *testing.T) {
	log := banner + initLine + banner + "boot\n" + banner + "ready\n"

	finding := classifySplice(spliceTwice, "die splice 1\ndie splice 1\n", log)

	if finding.Classification != Degraded {
		t.Fatalf("want Degraded, got %+v", finding)
	}
}

func TestClassifyRejectsALogWithMoreJvmStartsThanRestarts(t *testing.T) {
	log := banner + initLine + banner + initLine + banner + "ready\n"

	finding := classifySplice(spliceOnce, "die splice 1\n", log)

	if finding.Classification != Degraded {
		t.Fatalf("want Degraded, got %+v", finding)
	}
}

func TestClassifyRejectsALogWithFewerJvmStartsThanRestarts(t *testing.T) {
	log := banner + initLine + "ready\n"

	finding := classifySplice(spliceTwice, "die splice 1\ndie splice 1\n", log)

	if finding.Classification != Degraded {
		t.Fatalf("want Degraded, got %+v", finding)
	}
}

func TestClassifyAcceptsARestartThatHasNotLoggedItsNewJvmStartYet(t *testing.T) {
	log := banner + causedBy + initLine

	finding := classifySplice(spliceOnce, "die splice 1\n", log)

	if finding.Classification != SelfHealedSpliceInit {
		t.Fatalf("want SelfHealedSpliceInit, got %+v", finding)
	}
}

func TestClassifyReportsAnUnrecordedExitCodeAndOomKillWhenTheEventBufferExpired(t *testing.T) {
	finding := classifySplice(spliceOnce, "", initFailure)

	want := "splice restarted 1 time during boot (OOM kill and exit code not recorded (Docker event buffer expired), " +
		"last \"Caused by\": io.grpc.StatusRuntimeException: DEADLINE_EXCEEDED: deadline exceeded after 59.9s): "
	if !strings.HasPrefix(finding.Reason, want) || strings.Contains(finding.Reason, "no OOM kill") {
		t.Fatalf("want a reason starting\n%s\ngot\n%s", want, finding.Reason)
	}
}

func TestClassifyReportsAnUnrecordedExitCodeWhenOnlySomeRestartsLeftADieEvent(t *testing.T) {
	log := banner + initLine + banner + initLine + banner + "ready\n"

	finding := classifySplice(spliceTwice, "die splice 1\n", log)

	if !strings.Contains(finding.Reason, "(OOM kill and exit code not recorded (Docker event buffer expired)") {
		t.Fatalf("want the unrecorded wording, got %q", finding.Reason)
	}
}

func TestClassifyReportsAnUnrecordedOomKillOnTheDegradedPathToo(t *testing.T) {
	docker := &scriptedDocker{psOutput: "canton\n", inspectOutput: cantonOnce, logs: map[string]string{"canton": "boot\n"}}

	finding := Classify(context.Background(), docker.run, "localnet", until)

	if finding.Classification != Degraded || !strings.HasPrefix(finding.Reason, "canton restarted 1 time during boot (OOM kill and exit code not recorded (Docker event buffer expired)") {
		t.Fatalf("want the unrecorded wording, got %+v", finding)
	}
}

func TestClassifyToleratesAnUnrecordedOomKillByDefault(t *testing.T) {
	finding := classifySplice(spliceOnce, "", initFailure)

	if finding.Classification != SelfHealedSpliceInit {
		t.Fatalf("want SelfHealedSpliceInit, got %+v", finding)
	}
}

func TestClassifyRejectsAnUnrecordedOomKillWhenTheSwitchIsOff(t *testing.T) {
	original := toleratesUnrecordedOOM
	toleratesUnrecordedOOM = func() bool { return false }
	t.Cleanup(func() { toleratesUnrecordedOOM = original })

	finding := classifySplice(spliceOnce, "", initFailure)

	if finding.Classification != Degraded {
		t.Fatalf("want Degraded, got %+v", finding)
	}
}

func TestClassifyAcceptsARecordedDieEventWhateverTheSwitchSays(t *testing.T) {
	original := toleratesUnrecordedOOM
	toleratesUnrecordedOOM = func() bool { return false }
	t.Cleanup(func() { toleratesUnrecordedOOM = original })

	finding := classifySplice(spliceOnce, "die splice 1\n", initFailure)

	if finding.Classification != SelfHealedSpliceInit {
		t.Fatalf("want SelfHealedSpliceInit, got %+v", finding)
	}
}
