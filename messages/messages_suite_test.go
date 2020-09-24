// Copyright 2020 VMware, Inc.
// SPDX-License-Identifier: Apache-2.0

package messages_test

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"testing"
)

func TestMessages(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Messages Suite")
}
