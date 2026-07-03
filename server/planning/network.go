package planning

import "sort"

// Node is a team in the dependency network.
type Node struct {
	Team        string  `json:"team"`
	Weeks       float64 `json:"weeks"`       // total estimated weeks across all initiatives (load)
	Initiatives int     `json:"initiatives"` // how many initiatives this team is in the path of
	Blocks      int     `json:"blocks"`      // # of (initiative,team) it is a dependency for (out-degree)
	WaitsOn     int     `json:"waitsOn"`     // # of dependencies it waits on (in-degree)
}

// Edge is a directed dependency: From must finish before To can proceed (From
// blocks To), aggregated across initiatives.
type Edge struct {
	From  string `json:"from"`
	To    string `json:"to"`
	Count int    `json:"count"` // number of initiatives with this dependency
}

// Network is the directed cross-pod dependency graph derived from a Plan.
type Network struct {
	Nodes []Node `json:"nodes"`
	Edges []Edge `json:"edges"`
}

// BuildNetwork aggregates a Plan into a directed dependency network: node load =
// sum of weeks; a "<dep> Sequence" entry "T waits on D" becomes a directed edge
// D -> T (D blocks T).
func BuildNetwork(p *Plan) *Network {
	nodes := map[string]*Node{}
	node := func(team string) *Node {
		n := nodes[team]
		if n == nil {
			n = &Node{Team: team}
			nodes[team] = n
		}
		return n
	}
	type key struct{ from, to string }
	edges := map[key]int{}

	for _, init := range p.Initiatives {
		for team, w := range init.Work {
			if !w.InPath {
				continue
			}
			n := node(team)
			n.Weeks += w.Weeks
			n.Initiatives++
			for _, dep := range w.DependsOn {
				if dep == team {
					continue // ignore self-dependencies
				}
				node(dep) // ensure the dependency is a node even if it has no own work
				edges[key{dep, team}]++
			}
		}
	}

	net := &Network{}
	for _, n := range nodes {
		net.Nodes = append(net.Nodes, *n)
	}
	for k, c := range edges {
		net.Edges = append(net.Edges, Edge{From: k.from, To: k.to, Count: c})
		nodes[k.from].Blocks += c
		nodes[k.to].WaitsOn += c
	}
	// recompute degree fields onto the returned nodes (map copies are stale)
	deg := map[string]*Node{}
	for i := range net.Nodes {
		deg[net.Nodes[i].Team] = &net.Nodes[i]
	}
	for i := range net.Nodes {
		net.Nodes[i].Blocks = 0
		net.Nodes[i].WaitsOn = 0
	}
	for _, e := range net.Edges {
		deg[e.From].Blocks += e.Count
		deg[e.To].WaitsOn += e.Count
	}

	sort.Slice(net.Nodes, func(i, j int) bool { return net.Nodes[i].Weeks > net.Nodes[j].Weeks })
	sort.Slice(net.Edges, func(i, j int) bool {
		if net.Edges[i].From != net.Edges[j].From {
			return net.Edges[i].From < net.Edges[j].From
		}
		return net.Edges[i].To < net.Edges[j].To
	})
	return net
}
