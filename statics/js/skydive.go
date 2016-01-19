package main

import (
	"github.com/gopherjs/gopherjs/js"
	"honnef.co/go/js/console"

	"github.com/redhat-cip/skydive/topology/graph"
)

var arrayint = [...]int{1, 2, 3, 4, 5}

func returnarray() []int {
	return arrayint[:]
}

var g *graph.Graph

func main() {
	console.Clear()
	console.Log("testing")

	backend, err := graph.NewMemoryBackend()
	if err != nil {
		console.Log("backend init error" + err.Error())
		return
	}

	g, err := graph.NewGraph(backend)
	if err != nil {
		console.Log("NewGraph error" + err.Error())
		return
	}
	js.Global.Set("GenID", graph.GenID)

	meta := graph.Metadatas{"IfIndex": uint32(0x1324), "Type": "veth"}
	js.Global.Set("mmm", meta)
	js.Global.Set("graph", js.MakeWrapper(g))

	graphjs := js.Global.Get("graph")
	graphjs.Set("returnarray", returnarray)

	/*
		In the console type :
				graph.NewNode("abcaaa",{IfIndex: 4900, Type: "veth"})
	                        graph.String()
	*/

}
