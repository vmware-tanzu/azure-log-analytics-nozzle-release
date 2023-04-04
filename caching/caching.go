// Copyright 2020 VMware, Inc.
// SPDX-License-Identifier: Apache-2.0

package caching

import (
	"fmt"
	"math/rand"
	"os"
	"strings"
	"sync"
	"time"

	"code.cloudfoundry.org/lager"
	cfclient "github.com/cloudfoundry-community/go-cfclient"
)

type AppInfo struct {
	Name      string `json:"name"`
	Org       string `json:"org"`
	OrgID     string `json:"orgId"`
	Space     string `json:"space"`
	SpaceID   string `json:"spaceId"`
	Monitored bool   `json:"monitored"`
}

type Caching struct {
	cfClientConfig  *cfclient.Config
	appInfosByGuid  map[string]AppInfo
	spaceWhiteList  map[string]bool
	appInfoLock     sync.RWMutex
	logger          lager.Logger
	instanceName    string
	environment     string
	cachingInterval time.Duration
}

type CachingClient interface {
	GetAppInfo(string) AppInfo
	GetInstanceName() string
	GetEnvironmentName() string
	Initialize()
}

func NewCaching(config *cfclient.Config, logger lager.Logger, environment string, spaceFilter string, cachingInterval time.Duration) CachingClient {
	var spaceWhiteList map[string]bool
	if len(spaceFilter) > 0 {
		logger.Info("config", lager.Data{"SPACE_FILTER": spaceFilter})
		spaceWhiteList = make(map[string]bool)
		spaceFilters := strings.Split(spaceFilter, ",")
		for _, v := range spaceFilters {
			v = strings.TrimSuffix(strings.Trim(v, " "), ".*")
			spaceWhiteList[v] = true
			logger.Debug("adding app space filter", lager.Data{"filter-content": v})
		}
	} else {
		logger.Info("config SPACE_FILTER is nil, all apps will be monitored")
	}
	return &Caching{
		cfClientConfig:  config,
		appInfosByGuid:  make(map[string]AppInfo),
		spaceWhiteList:  spaceWhiteList,
		logger:          logger,
		environment:     environment,
		cachingInterval: cachingInterval,
	}
}

func (c *Caching) addAppinfoRecord(app cfclient.App) {
	var appInfo = AppInfo{
		Name:    app.Name,
		Org:     app.SpaceData.Entity.OrgData.Entity.Name,
		OrgID:   app.SpaceData.Entity.OrgData.Entity.Guid,
		Space:   app.SpaceData.Entity.Name,
		SpaceID: app.SpaceData.Entity.Guid,
	}
	appInfo.Monitored = c.spaceWhiteList == nil || c.spaceWhiteList[app.SpaceData.Entity.OrgData.Entity.Name] ||
		c.spaceWhiteList[app.SpaceData.Entity.OrgData.Entity.Name+"."+app.SpaceData.Entity.Name] ||
		c.spaceWhiteList[app.SpaceData.Entity.OrgData.Entity.Name+"."+app.SpaceData.Entity.Name+"."+app.Name]
	c.appInfoLock.Lock()
	c.appInfosByGuid[app.Guid] = appInfo
	c.appInfoLock.Unlock()
	c.logger.Debug("adding to app info cache",
		lager.Data{"guid": app.Guid},
		lager.Data{"info": appInfo},
	)
}

func (c *Caching) Initialize() {
	c.setInstanceName() //nolint:errcheck

	c.refreshCache()

	c.logger.Info("Cache initialize completed",
		lager.Data{"cache size": len(c.appInfosByGuid)})
	go func() {
		time.Sleep(time.Duration(float64(c.cachingInterval) * rand.Float64()))
		ticker := time.NewTicker(c.cachingInterval)
		for range ticker.C {
			c.refreshCache()
		}
	}()
}

func (c *Caching) refreshCache() {
	c.logger.Debug("Refreshing Cache")
	cfClient, err := cfclient.NewClient(c.cfClientConfig)
	if err != nil {
		c.logger.Error("error creating cfclient", err)
		return
	}

	apps, err := cfClient.ListAppsByQuery(nil)
	if err != nil {
		c.logger.Error("error getting app list", err)
		return
	}

	spaces, err := cfClient.ListSpaces()
	if err != nil {
		c.logger.Error("error getting spaces list", err)
		return
	}

	spaceMap := make(map[string]cfclient.Space)
	for _, space := range spaces {
		spaceMap[space.Guid] = space
	}

	orgs, err := cfClient.ListOrgs()
	if err != nil {
		c.logger.Error("error getting org list", err)
		return
	}
	orgMap := make(map[string]cfclient.Org)
	for _, org := range orgs {
		orgMap[org.Guid] = org
	}

	newAppInfo := make(map[string]AppInfo)
	for _, app := range apps {
		appInfo := AppInfo{
			Name:    app.Name,
			Org:     orgMap[spaceMap[app.SpaceGuid].OrganizationGuid].Name,
			OrgID:   spaceMap[app.SpaceGuid].OrganizationGuid,
			Space:   spaceMap[app.SpaceGuid].Name,
			SpaceID: spaceMap[app.SpaceGuid].Guid,
		}
		appInfo.Monitored = c.spaceWhiteList == nil || c.spaceWhiteList[appInfo.Org] ||
			c.spaceWhiteList[appInfo.Org+"."+appInfo.Space] ||
			c.spaceWhiteList[appInfo.Org+"."+appInfo.Space+"."+appInfo.Name]
		newAppInfo[app.Guid] = appInfo
	}

	c.appInfoLock.Lock()
	c.appInfosByGuid = newAppInfo
	c.appInfoLock.Unlock()
	c.logger.Debug("Refreshed")
}

func (c *Caching) GetAppInfo(appGuid string) AppInfo {
	var appInfo AppInfo
	var ok bool
	var old bool
	func() {
		c.appInfoLock.RLock()
		defer c.appInfoLock.RUnlock()
		appInfo, ok = c.appInfosByGuid[appGuid]
	}()
	if ok && !old {
		return appInfo
	} else {
		if !ok {
			c.logger.Info("App info not found for GUID",
				lager.Data{"guid": appGuid})
		}
		// call the client api to get the name for this app
		// purposely create a new client due to issue in using a single client
		start := time.Now()
		cfClient, err := cfclient.NewClient(c.cfClientConfig)
		if err != nil {
			c.logger.Error("error creating cfclient", err)
			return AppInfo{
				Name:      "",
				Org:       "",
				OrgID:     "",
				Space:     "",
				SpaceID:   "",
				Monitored: false,
			}
		}
		app, err := cfClient.AppByGuid(appGuid)
		stop := time.Now()
		c.logger.Debug("app info lookup time", nil, lager.Data{"time_ms": stop.Sub(start).Milliseconds()})
		if err != nil {
			c.logger.Error("error getting app info", err, lager.Data{"guid": appGuid})
			return AppInfo{
				Name:      "",
				Org:       "",
				OrgID:     "",
				Space:     "",
				SpaceID:   "",
				Monitored: false,
			}
		} else {
			// store app info in map
			c.addAppinfoRecord(app)
			// return App Info
			return appInfo
		}
	}
}

func (c *Caching) setInstanceName() error {
	// instance id to track multiple nozzles, used for logging
	hostName, err := os.Hostname()
	if err != nil {
		c.logger.Error("failed to get hostname for nozzle instance", err)
		c.instanceName = fmt.Sprintf("pid-%d", os.Getpid())
	} else {
		c.instanceName = fmt.Sprintf("pid-%d@%s", os.Getpid(), hostName)
	}
	c.logger.Info("getting nozzle instance name", lager.Data{"name": c.instanceName})
	return err
}

func (c *Caching) GetInstanceName() string {
	return c.instanceName
}

func (c *Caching) GetEnvironmentName() string {
	return c.environment
}
