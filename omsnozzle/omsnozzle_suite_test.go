// Copyright 2020 VMware, Inc.
// SPDX-License-Identifier: Apache-2.0

package omsnozzle_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"testing"
)

func TestOmsnozzle(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Omsnozzle Suite")
}
