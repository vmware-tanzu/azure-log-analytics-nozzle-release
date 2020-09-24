// Copyright 2020 VMware, Inc.
// SPDX-License-Identifier: Apache-2.0

package mocks

import "github.com/pivotal/azure-oms-log-analytics-firehose-nozzle/caching"

type MockCaching struct {
	MockGetAppInfo  func(string) caching.AppInfo
	InstanceName    string
	EnvironmentName string
}

func (c *MockCaching) GetAppInfo(appGuid string) caching.AppInfo {
	return c.MockGetAppInfo(appGuid)
}

func (c *MockCaching) GetInstanceName() string {
	return c.InstanceName
}

func (c *MockCaching) GetEnvironmentName() string {
	return c.EnvironmentName
}

func (c *MockCaching) Initialize(loadApps bool) {
	return
}
