// Copyright (c) 2026 Peaceful Studio OÜ
// SPDX-License-Identifier: Apache-2.0

package main

import "errors"

const exitNotAuthorised = 2

type exitError struct {
	code    int
	message string
}

func (e exitError) Error() string { return e.message }

func notAuthorised(message string) error {
	return exitError{code: exitNotAuthorised, message: message}
}

func exitCode(err error) int {
	var wanted exitError
	if errors.As(err, &wanted) {
		return wanted.code
	}
	return 1
}
