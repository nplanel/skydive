/*
 * Copyright (C) 2016 Red Hat, Inc.
 *
 * Licensed to the Apache Software Foundation (ASF) under one
 * or more contributor license agreements.  See the NOTICE file
 * distributed with this work for additional information
 * regarding copyright ownership.  The ASF licenses this file
 * to you under the Apache License, Version 2.0 (the
 * "License"); you may not use this file except in compliance
 * with the License.  You may obtain a copy of the License at
 *
 *  http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing,
 * software distributed under the License is distributed on an
 * "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY
 * KIND, either express or implied.  See the License for the
 * specific language governing permissions and limitations
 * under the License.
 *
 */

package mappings

import (
	"fmt"

	"github.com/golang/protobuf/proto"
	"github.com/lebauce/dockerclient"
	"github.com/pmylund/go-cache"

	"github.com/redhat-cip/skydive/config"
	"github.com/redhat-cip/skydive/flow"
	"github.com/redhat-cip/skydive/logging"
	"github.com/redhat-cip/skydive/topology/probes"
)

type DockerMapper struct {
	client           *dockerclient.DockerClient
	probe            *probes.NetNSProbe
	cache            *cache.Cache
	cacheUpdaterChan chan string
}

type DockerContainerAttributes struct {
	ContainerID string
}

func (mapper *DockerMapper) getContainerFromMAC(MAC string) *string {
	if i, f := mapper.cache.Get(MAC); f {
		id := i.(string)
		return proto.String(id)
	}

	return nil
}

func (mapper *DockerMapper) Enhance(f *flow.Flow) {
	// TODO: find a place to put the container ID
	// ethFlow := f.GetStatistics().Endpoints[flow.FlowEndpointType_ETHERNET.Value()]
	// mapper.getContainerFromMAC(ethFlow.AB.Value)
	// mapper.getContainerFromMAC(ethFlow.BA.Value)
}

func (mapper *DockerMapper) handleDockerEvent(event *dockerclient.Event) {
	info, err := mapper.client.InspectContainer(event.ID)
	if err != nil {
		return
	}

	pid := info.State.Pid
	namespace := fmt.Sprintf("/proc/%d/ns/net", pid)

	if event.Status == "start" {
		mac := info.NetworkSettings.MacAddress
		logging.GetLogger().Debug("Register docker container %s with MAC %s", info.Id, mac)
		mapper.cache.Set(mac, info.Id, -1)

		logging.GetLogger().Debug("Listening for namespace %s with PID %d", namespace, pid)
		mapper.probe.Register(namespace)
	} else if event.Status == "die" {
		id := ""
		for mac, item := range mapper.cache.Items() {
			if item.Object == info.Id {
				id = info.Id
				logging.GetLogger().Debug("Unregister docker container %s with MAC %s", info.Id, mac)
				mapper.cache.Delete(mac)
			}
		}
		if id == "" {
			logging.GetLogger().Error("Container %s was not in cached", info.Id)
		}

		logging.GetLogger().Debug("Stop listening for namespace %s with PID %d", namespace, pid)
		mapper.probe.Unregister(namespace)
	}
}

func NewDockerMapper(dockerURL string, probe *probes.NetNSProbe) (*DockerMapper, error) {
	logging.GetLogger().Debug("Connecting to Docker daemon: %s", dockerURL)
	client, err := dockerclient.NewDockerClient(dockerURL, nil)
	if err != nil {
		return nil, err
	}

	mapper := &DockerMapper{client: client, probe: probe}
	mapper.cache = cache.New(cache.NoExpiration, 0)

	eventsOptions := &dockerclient.MonitorEventsOptions{
		Filters: &dockerclient.MonitorEventsFilters{
			Events: []string{"start", "die"},
		},
	}

	eventErrChan, err := client.MonitorEvents(eventsOptions, nil)
	if err != nil {
		return nil, err
	}

	go func() {
		for e := range eventErrChan {
			if e.Error != nil {
				return
			}
			mapper.handleDockerEvent(&e.Event)
		}
	}()

	containers, err := client.ListContainers(false, false, "")
	if err != nil {
		return nil, err
	}

	go func() {
		for _, c := range containers {
			info, err := mapper.client.InspectContainer(c.Id)
			if err != nil {
				return
			}

			mapper.cache.Set(info.NetworkSettings.MacAddress, c.Id, -1)
		}
	}()

	return mapper, nil
}

func NewDockerMapperFromConfig(probe *probes.NetNSProbe) (*DockerMapper, error) {
	dockerURL := config.GetConfig().Section("docker").Key("url").String()
	if dockerURL == "" {
		dockerURL = "unix:///var/run/docker.sock"
	}

	return NewDockerMapper(dockerURL, probe)
}