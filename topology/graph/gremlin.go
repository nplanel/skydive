/*
 * Copyright (C) 2015 Red Hat, Inc.
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

package graph

import (
	"errors"
	"time"

	"github.com/skydive-project/skydive/logging"
	"github.com/skydive-project/skydive/topology/graph/gremlin"
)

type GremlinBackend struct {
	client *gremlin.GremlinClient
}

func idToPropertiesString(i Identifier) (string, error) {
	properties := map[string]interface{}{
		"_ID": string(i),
	}

	encoder := gremlin.GremlinPropertiesEncoder{}
	err := encoder.Encode(properties)
	if err != nil {
		logging.GetLogger().Errorf("Error while retrieving a Node: %s", err.Error())
		return "", err
	}

	return encoder.String(), nil
}

func toPropertiesString(e graphElement) ([]byte, error) {
	properties := map[string]interface{}{
		"_ID":   string(e.ID),
		"_host": e.host,
	}
	for k, v := range e.metadata {
		if k[0] == '_' {
			return nil, errors.New("Properties starting with _ are reserved")
		}
		properties[k] = v
	}

	encoder := gremlin.GremlinPropertiesEncoder{}
	err := encoder.Encode(properties)

	return encoder.Bytes(), err
}

func gremElementID(e gremlin.GremlinElement) Identifier {
	return Identifier(e.Properties["_ID"][0].Value.(string))
}

func gremElementHost(e gremlin.GremlinElement) string {
	return e.Properties["_host"][0].Value.(string)
}

func gremElementToNode(e gremlin.GremlinElement) *Node {
	return &Node{
		graphElement: graphElement{
			ID:       gremElementID(e),
			metadata: gremElementMetadata(e),
			host:     gremElementHost(e),
		},
	}
}

func gremElementToEdge(e gremlin.GremlinElement) *Edge {
	return &Edge{
		graphElement: graphElement{
			ID:       gremElementID(e),
			metadata: gremElementMetadata(e),
			host:     gremElementHost(e),
		},
	}
}

func gremElementMetadata(e gremlin.GremlinElement) Metadata {
	m := Metadata{}
	for k, v := range e.Properties {
		if k[0] != '_' {
			switch v[0].Value.(type) {
			case float64:
				m[k] = int64(v[0].Value.(float64))
			default:
				m[k] = v[0].Value
			}
		}
	}
	return m
}

func (g GremlinBackend) SetMetadata(i interface{}, meta Metadata) bool {
	var e graphElement
	var elType string

	switch i.(type) {
	case *Node:
		e = i.(*Node).graphElement
		elType = "V"
	case *Edge:
		e = i.(*Edge).graphElement
		elType = "V"
	}

	properties, err := idToPropertiesString(e.ID)
	if err != nil {
		logging.GetLogger().Errorf("Error while retrieving a Node: %s", err.Error())
		return false
	}

	query := "g." + elType + "().has(" + properties + ")"

	els, err := g.client.QueryElements(query)
	if err != nil || len(els) == 0 {
		return false
	}

	if len(els) > 1 {
		logging.GetLogger().Errorf("Found more than one node for this ID: " + string(e.ID))
		return false
	}
	el := els[0]

	query = "g." + elType + "(" + string(el.ID) + ").properties().drop()"

	_, err = g.client.Query(query)
	if err != nil {
		logging.GetLogger().Errorf("Gremlin query error: %s, %s", query, err.Error())
		return false
	}

	j := meta.String()

	query = "g." + elType + "(" + string(el.ID) + ")"
	query += `.sideEffect{v = it; ["_ID": "` + string(e.ID) + `"`
	query += `, "_host": "` + string(e.host) + `"`
	query += `,` + j[1:len(j)-1] + `]`
	query += `.each{v.get().property(it.key, it.value)}}`

	_, err = g.client.Query(query)
	if err != nil {
		logging.GetLogger().Errorf("Gremlin query error: %s, %s", query, err.Error())
		return false
	}

	return true
}

func (g GremlinBackend) AddMetadata(i interface{}, k string, v interface{}) bool {
	var e graphElement
	var elType string

	switch i.(type) {
	case *Node:
		e = i.(*Node).graphElement
		elType = "V"
	case *Edge:
		e = i.(*Edge).graphElement
		elType = "V"
	}

	properties, err := idToPropertiesString(e.ID)
	if err != nil {
		logging.GetLogger().Errorf("Error while retrieving a Node: %s", err.Error())
		return false
	}

	encoder := gremlin.GremlinPropertiesEncoder{}
	encoder.EncodeKVPair(k, v)

	query := "g." + elType + "().has(" + properties + ").property(" + encoder.String() + ")"

	_, err = g.client.Query(query)
	if err != nil {
		logging.GetLogger().Errorf("Gremlin query error: %s, %s", query, err.Error())
		return false
	}

	return true
}

func (g GremlinBackend) AddEdge(e *Edge) bool {
	properties, err := toPropertiesString(e.graphElement)
	if err != nil {
		logging.GetLogger().Errorf("Error while adding a new Edge: %s", err.Error())
		return false
	}

	propsParent, err := idToPropertiesString(e.parent)
	if err != nil {
		logging.GetLogger().Errorf("Error while adding a new Edge: %s", err.Error())
		return false
	}

	propsChild, err := idToPropertiesString(e.child)
	if err != nil {
		logging.GetLogger().Errorf("Error while adding a new Edge: %s", err.Error())
		return false
	}
	// g.V().has("_ID","1cd6f416-b478-58b5-5357-fd8e277adeb2").next()
	// .addEdge('linked',
	//     g.V().has("_ID","4e9d69ce-2917-4731-49d6-f04ca2b34be2").next(),
	//     "_ID","abd2938d-b391-5b82-701e-e596f58d2a2d",
	//     "_host","skydive-compile.localdomain",
	//     "RelationType","ownership")
	//: Script Evaluation Error
	query := "g.V().has(" + propsParent + ").next()"
	query += ".addEdge(g.V().has(" + propsChild + ").next(), " + "g.V().has(" + string(properties) + ").next(), 'linked')"
	_, err = g.client.Query(query)
	if err != nil {
		logging.GetLogger().Errorf("Error while adding a new Node: %s", err.Error())
		return false
	}

	return true
}

func (g GremlinBackend) GetEdge(i Identifier, t *time.Time) *Edge {
	properties, err := idToPropertiesString(i)
	if err != nil {
		logging.GetLogger().Errorf("Error while retrieving a Node: %s", err.Error())
		return nil
	}

	query := "g.E().has(" + properties + ")"

	els, err := g.client.QueryElements(query)
	if err != nil {
		return nil
	}

	switch l := len(els); {
	case l == 0:
		return nil
	case l > 1:
		logging.GetLogger().Error("Found more than one edge for this ID: " + string(i))
		return nil
	}

	edge := gremElementToEdge(els[0])

	parent, child := g.GetEdgeNodes(edge, t)
	if parent == nil || child == nil {
		return nil
	}

	edge.parent = parent.ID
	edge.child = child.ID

	return edge
}

func (g GremlinBackend) GetEdgeNodes(e *Edge, t *time.Time) (*Node, *Node) {
	properties, err := idToPropertiesString(e.ID)
	if err != nil {
		logging.GetLogger().Errorf("Error while retrieving a Edge: %s", err.Error())
		return nil, nil
	}

	query := "g.E().has(" + properties + ").bothV()"

	els, err := g.client.QueryElements(query)
	if err != nil {
		return nil, nil
	}

	if len(els) != 2 {
		logging.GetLogger().Errorf("Not found 2 nodes for this edge: " + string(e.ID))
		return nil, nil
	}

	return gremElementToNode(els[0]), gremElementToNode(els[1])
}

func (g GremlinBackend) AddNode(n *Node) bool {
	properties, err := toPropertiesString(n.graphElement)
	if err != nil {
		logging.GetLogger().Errorf("Error while adding a new Node: %s", err.Error())
		return false
	}

	query := "graph.addVertex(" + string(properties) + ")"

	_, err = g.client.Query(query)
	if err != nil {
		logging.GetLogger().Errorf("Gremlin query error: %s, %s", query, err.Error())
		return false
	}

	return true
}

func (g GremlinBackend) GetNode(i Identifier, t *time.Time) *Node {
	properties, err := idToPropertiesString(i)
	if err != nil {
		logging.GetLogger().Errorf("Error while retrieving a Node: %s", err.Error())
		return nil
	}

	query := "g.V().has(" + properties + ")"

	els, err := g.client.QueryElements(query)
	if err != nil {
		return nil
	}

	switch l := len(els); {
	case l == 0:
		return nil
	case l > 1:
		logging.GetLogger().Errorf("Found more than one node for this ID: " + string(i))
		return nil
	}

	return gremElementToNode(els[0])
}

func (g GremlinBackend) GetNodeEdges(n *Node, t *time.Time) []*Edge {
	var edges []*Edge

	properties, err := idToPropertiesString(n.ID)
	if err != nil {
		logging.GetLogger().Errorf("Error while retrieving a Node: %s", err.Error())
		return edges
	}

	query := "g.V().has(" + properties + ").bothE()"

	els, err := g.client.QueryElements(query)
	if err != nil {
		return edges
	}

	for _, el := range els {
		edges = append(edges, gremElementToEdge(el))
	}

	return edges
}

func (g GremlinBackend) DelEdge(e *Edge) bool {
	properties, err := idToPropertiesString(e.ID)
	if err != nil {
		logging.GetLogger().Errorf("Error while deleting edge: %s", err.Error())
		return false
	}

	query := "g.E().has(" + properties + ").drop()"

	_, err = g.client.Query(query)
	if err != nil {
		logging.GetLogger().Errorf("Error while deleting edge: %s", err.Error())
		return false
	}

	return true
}

func (g GremlinBackend) DelNode(n *Node) bool {
	properties, err := idToPropertiesString(n.ID)
	if err != nil {
		logging.GetLogger().Errorf("Error while deleting node: %s", err.Error())
		return false
	}

	query := "g.V().has(" + properties + ").drop()"

	_, err = g.client.Query(query)
	if err != nil {
		logging.GetLogger().Errorf("Error while deleting node: %s", err.Error())
		return false
	}

	return true
}

func (g GremlinBackend) GetNodes(t *time.Time) []*Node {
	var nodes []*Node

	query := "g.V().has('_ID')"

	els, err := g.client.QueryElements(query)
	if err != nil {
		return nodes
	}

	for _, e := range els {
		nodes = append(nodes, gremElementToNode(e))
	}

	return nodes
}

func (g GremlinBackend) GetEdges(t *time.Time) []*Edge {
	var edges []*Edge

	query := "g.E().has('_ID')"

	els, err := g.client.QueryElements(query)
	if err != nil {
		return edges
	}

	for _, e := range els {
		edge := gremElementToEdge(e)
		parent, child := g.GetEdgeNodes(edge, t)
		if parent == nil || child == nil {
			continue
		}

		edge.parent = parent.ID
		edge.child = child.ID

		edges = append(edges, edge)
	}

	return edges
}

func NewGremlinBackend(endpoint string) (*GremlinBackend, error) {
	c, err := gremlin.NewClient(endpoint)
	if err != nil {
		return nil, err
	}
	if err = c.Connect(); err != nil {
		return nil, err
	}

	logging.GetLogger().Infof("Connected to gremlin server %s", endpoint)

	return &GremlinBackend{
		client: c,
	}, nil
}
