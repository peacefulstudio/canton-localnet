// Copyright (c) 2026 Peaceful Studio OÜ
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

const (
	minDockerMajorForLoopbackOnlyPublishing = 28
	dockerVersionProbeTimeout               = 10 * time.Second
)

var dockerServerVersion = func(ctx context.Context) (string, error) {
	output, err := restartGuardRunner(ctx, "docker", "version", "--format", "{{.Server.Version}}")
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(output)), nil
}

func warnIfDockerPublishesLoopbackToTheLan(ctx context.Context, cmd *cobra.Command) {
	probeCtx, cancel := context.WithTimeout(ctx, dockerVersionProbeTimeout)
	defer cancel()
	version, major, err := readDockerServerMajor(probeCtx)
	switch {
	case err != nil:
		printWarning(cmd, fmt.Sprintf("could not read the Docker server version (%v): ports published to 127.0.0.1 are reachable from the local network segment on Docker servers below %d.0.0", err, minDockerMajorForLoopbackOnlyPublishing))
	case major < minDockerMajorForLoopbackOnlyPublishing:
		printWarning(cmd, fmt.Sprintf("Docker server version %s is below %d.0.0: a port published to 127.0.0.1 may still be reachable from other hosts on the same network segment on this version", version, minDockerMajorForLoopbackOnlyPublishing))
	}
}

func readDockerServerMajor(ctx context.Context) (version string, major int, err error) {
	version, err = dockerServerVersion(ctx)
	if err != nil {
		return "", 0, err
	}
	major, err = dockerMajorVersion(version)
	return version, major, err
}

func dockerMajorVersion(version string) (int, error) {
	majorText, _, _ := strings.Cut(version, ".")
	major, err := strconv.Atoi(majorText)
	if err != nil || major < 0 {
		return 0, fmt.Errorf("cannot parse a major version from %q", version)
	}
	return major, nil
}
