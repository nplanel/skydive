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
	"encoding/json"
	//	"os"
	"sync"

	"github.com/nu7hatch/gouuid"
)

type Identifier string

type GraphEventListener interface {
	OnNodeUpdated(n *Node)
	OnNodeAdded(n *Node)
	OnNodeDeleted(n *Node)
	OnEdgeUpdated(e *Edge)
	OnEdgeAdded(e *Edge)
	OnEdgeDeleted(e *Edge)
}

type Metadatas map[string]interface{}

type graphElement struct {
	ID        Identifier
	metadatas Metadatas
}

type Node struct {
	graphElement
}

type Edge struct {
	graphElement
	parent Identifier
	child  Identifier
}

type GraphBackend interface {
	AddNode(n *Node) bool
	DelNode(n *Node) bool
	GetNode(i Identifier) *Node
	GetNodeEdges(n *Node) []*Edge

	AddEdge(e *Edge) bool
	DelEdge(e *Edge) bool
	GetEdge(i Identifier) *Edge
	GetEdgeNodes(e *Edge) (*Node, *Node)

	SetMetadata(e interface{}, k string, v interface{}) bool
	SetMetadatas(e interface{}, m Metadatas) bool

	GetNodes() []*Node
	GetEdges() []*Edge
}

type Graph struct {
	sync.RWMutex
	backend        GraphBackend
	host           string
	eventListeners []GraphEventListener
}

// global host name
var host string

func GenID() Identifier {
	u, _ := uuid.NewV4()

	return Identifier(u.String())
}

func (e *graphElement) Metadatas() Metadatas {
	return e.metadatas
}

func (e *graphElement) matchFilters(f Metadatas) bool {
	for k, v := range f {
		nv, ok := e.metadatas[k]
		if !ok || v != nv {
			return false
		}
	}

	return true
}

func (e *graphElement) String() string {
	j, _ := json.Marshal(e)
	return string(j)
}

func (n *Node) MarshalJSON() ([]byte, error) {
	return json.Marshal(&struct {
		ID        Identifier
		Metadatas Metadatas `json:",omitempty"`
		Host      string
	}{
		ID:        n.ID,
		Metadatas: n.metadatas,
		Host:      host,
	})
}

func (e *Edge) MarshalJSON() ([]byte, error) {
	return json.Marshal(&struct {
		ID        Identifier
		Metadatas Metadatas `json:",omitempty"`
		Parent    Identifier
		Child     Identifier
		Host      string
	}{
		ID:        e.ID,
		Metadatas: e.metadatas,
		Parent:    e.parent,
		Child:     e.child,
		Host:      host,
	})
}

func (g *Graph) notifyMetadataUpdated(e interface{}) {
	switch e.(type) {
	case *Node:
		g.NotifyNodeUpdated(e.(*Node))
	case *Edge:
		g.NotifyEdgeUpdated(e.(*Edge))
	}
}

func (g *Graph) SetMetadatas(e interface{}, m Metadatas) {
	if !g.backend.SetMetadatas(e, m) {
		return
	}
	g.notifyMetadataUpdated(e)
}

func (g *Graph) SetMetadata(e interface{}, k string, v interface{}) {
	if !g.backend.SetMetadata(e, k, v) {
		return
	}
	g.notifyMetadataUpdated(e)
}

func (g *Graph) getAncestorsTo(n *Node, f Metadatas, ancestors *[]*Node) bool {
	*ancestors = append(*ancestors, n)

	edges := g.backend.GetNodeEdges(n)

	for _, e := range edges {
		parent, child := g.backend.GetEdgeNodes(e)

		if child != nil && child.ID == n.ID && parent.matchFilters(f) {
			*ancestors = append(*ancestors, parent)
			return true
		}
	}

	for _, e := range edges {
		parent, child := g.backend.GetEdgeNodes(e)

		if child != nil && child.ID == n.ID {
			if g.getAncestorsTo(parent, f, ancestors) {
				return true
			}
		}
	}

	return false
}

func (g *Graph) GetAncestorsTo(n *Node, f Metadatas) ([]*Node, bool) {
	ancestors := []*Node{}

	ok := g.getAncestorsTo(n, f, &ancestors)

	return ancestors, ok
}

func (g *Graph) LookupParentNodes(n *Node, f Metadatas) []*Node {
	parents := []*Node{}

	for _, e := range g.backend.GetNodeEdges(n) {
		parent, child := g.backend.GetEdgeNodes(e)

		if child != nil && child.ID == n.ID && parent.matchFilters(f) {
			parents = append(parents, child)
		}
	}

	return parents
}

func (g *Graph) LookupChildren(n *Node, f Metadatas) []*Node {
	children := []*Node{}

	for _, e := range g.backend.GetNodeEdges(n) {
		parent, child := g.backend.GetEdgeNodes(e)

		if parent != nil && parent.ID == n.ID && child.matchFilters(f) {
			children = append(children, child)
		}
	}

	return children
}

func (g *Graph) AreLinked(n1 *Node, n2 *Node) bool {
	for _, e := range g.backend.GetNodeEdges(n1) {
		parent, child := g.backend.GetEdgeNodes(e)
		if parent == nil || child == nil {
			continue
		}

		if child.ID == n2.ID || parent.ID == n2.ID {
			return true
		}
	}

	return false
}

func (g *Graph) Link(n1 *Node, n2 *Node) {
	g.NewEdge(GenID(), n1, n2, nil)
}

func (g *Graph) Unlink(n1 *Node, n2 *Node) {
	for _, e := range g.backend.GetNodeEdges(n1) {
		parent, child := g.backend.GetEdgeNodes(e)
		if parent == nil || child == nil {
			continue
		}

		if child.ID == n2.ID || parent.ID == n2.ID {
			g.DelEdge(e)
		}
	}
}

func (g *Graph) Replace(o *Node, n *Node, m Metadatas) *Node {
	for _, e := range g.backend.GetNodeEdges(o) {
		parent, child := g.backend.GetEdgeNodes(e)
		if parent == nil || child == nil {
			continue
		}

		g.DelEdge(e)

		if parent.ID == n.ID {
			g.Link(n, child)
		} else {
			g.Link(parent, n)
		}
	}
	n.metadatas = o.metadatas
	g.NotifyNodeUpdated(n)

	g.DelNode(o)

	return n
}

func (g *Graph) LookupFirstNode(m Metadatas) *Node {
	nodes := g.LookupNodes(m)
	if len(nodes) > 0 {
		return nodes[0]
	}

	return nil
}

func (g *Graph) LookupNodes(m Metadatas) []*Node {
	nodes := []*Node{}

	for _, n := range g.backend.GetNodes() {
		if n.matchFilters(m) {
			nodes = append(nodes, n)
		}
	}

	return nodes
}

func (g *Graph) LookupNodesFromKey(key string) []*Node {
	nodes := []*Node{}

	for _, n := range g.backend.GetNodes() {
		_, ok := n.metadatas[key]
		if ok {
			nodes = append(nodes, n)
		}
	}

	return nodes
}

func (g *Graph) AddEdge(e *Edge) bool {
	if !g.backend.AddEdge(e) {
		return false
	}
	g.NotifyEdgeAdded(e)

	return true
}

func (g *Graph) GetEdge(i Identifier) *Edge {
	return g.backend.GetEdge(i)
}

func (g *Graph) AddNode(n *Node) bool {
	if !g.backend.AddNode(n) {
		return false
	}
	g.NotifyNodeAdded(n)

	return true
}

func (g *Graph) GetNode(i Identifier) *Node {
	return g.backend.GetNode(i)
}

func (g *Graph) NewNode(i Identifier, m Metadatas) *Node {
	n := &Node{
		graphElement: graphElement{
			ID: i,
		},
	}

	if m != nil {
		n.metadatas = m
	} else {
		n.metadatas = make(Metadatas)
	}

	if !g.AddNode(n) {
		return nil
	}

	return n
}

func (g *Graph) NewEdge(i Identifier, p *Node, c *Node, m Metadatas) *Edge {
	e := &Edge{
		parent: p.ID,
		child:  c.ID,
		graphElement: graphElement{
			ID: i,
		},
	}

	if m != nil {
		e.metadatas = m
	} else {
		e.metadatas = make(Metadatas)
	}

	if !g.AddEdge(e) {
		return nil
	}

	return e
}

func (g *Graph) DelEdge(e *Edge) {
	if g.backend.DelEdge(e) {
		g.NotifyEdgeDeleted(e)
	}
}

func (g *Graph) DelNode(n *Node) {
	for _, e := range g.backend.GetNodeEdges(n) {
		g.DelEdge(e)
	}

	if g.backend.DelNode(n) {
		g.NotifyNodeDeleted(n)
	}
}

func (g *Graph) delSubGraph(n *Node, m map[Identifier]bool) {
	if _, ok := m[n.ID]; ok {
		return
	}
	m[n.ID] = true

	for _, e := range g.backend.GetNodeEdges(n) {
		_, child := g.backend.GetEdgeNodes(e)

		if child.ID != n.ID {
			g.delSubGraph(child, m)
			g.DelNode(child)
		}
	}
}

func (g *Graph) DelSubGraph(n *Node) {
	g.delSubGraph(n, make(map[Identifier]bool))
}

func (g *Graph) GetNodes() []*Node {
	return g.backend.GetNodes()
}

func (g *Graph) GetEdges() []*Edge {
	return g.backend.GetEdges()
}

func (g *Graph) String() string {
	j, _ := json.Marshal(g)
	return string(j)
}

func (g *Graph) MarshalJSON() ([]byte, error) {
	return json.Marshal(&struct {
		Nodes []*Node
		Edges []*Edge
	}{
		Nodes: g.GetNodes(),
		Edges: g.GetEdges(),
	})
}

func (g *Graph) NotifyNodeUpdated(n *Node) {
	for _, l := range g.eventListeners {
		l.OnNodeUpdated(n)
	}
}

func (g *Graph) NotifyNodeDeleted(n *Node) {
	for _, l := range g.eventListeners {
		l.OnNodeDeleted(n)
	}
}

func (g *Graph) NotifyNodeAdded(n *Node) {
	for _, l := range g.eventListeners {
		l.OnNodeAdded(n)
	}
}

func (g *Graph) NotifyEdgeUpdated(e *Edge) {
	for _, l := range g.eventListeners {
		l.OnEdgeUpdated(e)
	}
}

func (g *Graph) NotifyEdgeDeleted(e *Edge) {
	for _, l := range g.eventListeners {
		l.OnEdgeDeleted(e)
	}
}

func (g *Graph) NotifyEdgeAdded(e *Edge) {
	for _, l := range g.eventListeners {
		l.OnEdgeAdded(e)
	}
}

func (g *Graph) AddEventListener(l GraphEventListener) {
	g.Lock()
	defer g.Unlock()

	g.eventListeners = append(g.eventListeners, l)
}

func NewGraph(b GraphBackend) (*Graph, error) {
	/*	h, err := os.Hostname()
		if err != nil {
			return nil, err
		}*/
	host = "fixme from gopherjs : package os can't be used at runtime" //h

	return &Graph{
		backend: b,
	}, nil
}
