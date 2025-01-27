// Package store pkg/route-finder/store/dijkstra.go
package store

import (
	"container/heap"
	"context"
	"errors"
	"sort"

	"github.com/skycoin/skywire/pkg/routing"
	"github.com/skycoin/skywire/pkg/skywire-utilities/pkg/cipher"
)

// package level errors
var (
	ErrNoRoute       = errors.New("no route to destination")
	ErrContextClosed = errors.New("context closed or timed out")
	ErrRouteNotFound = errors.New("route not found")
)

// dist is constant for now, can be latencies in a new implementation
const (
	infinity         = int(^uint(0) >> 1)
	distBetweenNodes = 1
)

// Shortest returns a set of max number shortest routes from source to destination which length is between minLen and
// maxLen
func (g *Graph) Shortest(ctx context.Context, source, destination cipher.PubKey, minLen, maxLen, number int) (routes []routing.Route, err error) {
	sourceVertex, ok := g.graph[source]
	if !ok {
		return nil, ErrNoRoute
	}

	destinationVertex, ok := g.graph[destination]
	if !ok {
		return nil, ErrNoRoute
	}

	previousNodes, err := g.dijkstra(ctx, sourceVertex, destinationVertex, minLen, maxLen)
	if err != nil {
		return nil, err
	}
	return g.routes(ctx, previousNodes, destinationVertex, minLen, maxLen, number)
}

type queueElement struct {
	vertex   *vertex
	distance int
	hops     int
}

type previousNode struct {
	distance int
	hops     int
	previous *vertex
}

// Implement node version of: https://rosettacode.org/wiki/Dijkstra%27s_algorithm#Go
// dijkstra computes optimal paths from source node to every other node, but it keeps track of every other
// suboptimal route to destination and returns them
func (g *Graph) dijkstra(ctx context.Context, source, destination *vertex, minhop, maxhop int) ([]previousNode, error) {
	dist := make(map[*vertex]map[int]int)
	prev := make(map[*vertex]map[int]*vertex)
	destinationPrev := make([]previousNode, 0)

	sid := source
	// Initialize distance for source
	dist[sid] = make(map[int]int)
	dist[sid][0] = 0 // 0 hops, distance 0

	q := &priorityQueue{[]*queueElement{}, make(map[*vertex]int), make(map[*vertex]int)}
	heap.Init(q)
	// Add source to the queue
	heap.Push(q, &queueElement{vertex: sid, distance: 0, hops: 0})

	// Initialize other vertices
	for _, v := range g.graph {
		if v != sid {
			dist[v] = make(map[int]int)
		}
		prev[v] = make(map[int]*vertex)
	}

	for q.Len() > 0 {
		select {
		case <-ctx.Done():
			return nil, ErrContextClosed
		default:
			uElement := heap.Pop(q).(*queueElement)
			u := uElement.vertex
			currentDist := uElement.distance
			currentHops := uElement.hops

			// Skip if a better distance already exists for this vertex and hop count
			if d, exists := dist[u][currentHops]; !exists || currentDist > d {
				continue
			}

			// Process neighbors
			for _, v := range u.neighbors {
				newHops := currentHops + 1
				if newHops > maxhop {
					continue
				}

				// Assuming edge weight is 1; replace with actual weight retrieval
				edgeWeight := 1
				alt := currentDist + edgeWeight

				if v == destination {
					if newHops >= minhop {
						pn := previousNode{
							distance: alt,
							hops:     newHops,
							previous: u,
						}
						destinationPrev = append(destinationPrev, pn)
					}
				} else {
					// Check if this path is better
					existingDist, exists := dist[v][newHops]
					if !exists || alt < existingDist {
						dist[v][newHops] = alt
						prev[v][newHops] = u
						heap.Push(q, &queueElement{
							vertex:   v,
							distance: alt,
							hops:     newHops,
						})
					}
				}
			}
		}
	}

	// Find the best path in destinationPrev
	if len(destinationPrev) == 0 {
		return nil, errors.New("no path found within hop constraints")
	}

	// Select the entry with the smallest distance
	bestIndex := 0
	for i, pn := range destinationPrev {
		if pn.distance < destinationPrev[bestIndex].distance {
			bestIndex = i
		}
	}
	best := destinationPrev[bestIndex]

	// Reconstruct the path
	path := []previousNode{}
	currentVertex := destination
	currentHops := best.hops
	for currentVertex != source && currentHops > 0 {
		prevVertex := prev[currentVertex][currentHops]
		if prevVertex == nil {
			break // Path is broken
		}
		path = append([]previousNode{{
			distance: dist[currentVertex][currentHops],
			hops:     currentHops,
			previous: prevVertex,
		}}, path...)
		currentVertex = prevVertex
		currentHops--
	}

	// Add the source node
	path = append([]previousNode{{
		distance: 0,
		hops:     0,
		previous: nil,
	}}, path...)

	return path, nil
}

// Route sorts by length and backtraces every route from destination to source. Only adds the paths
// with length between minLen and maxLen and returns a maximum of number routes
func (g *Graph) routes(ctx context.Context, previousNodes []previousNode, destination *vertex, minLen, maxLen, number int) ([]routing.Route, error) {
	// Sort
	sort.Slice(previousNodes, func(i, j int) bool {
		return previousNodes[i].distance < previousNodes[j].distance
	})

	// Backtrace
	routes := make([]routing.Route, 0)

	for _, prev := range previousNodes {
		if len(routes) == number {
			break
		}

		select {
		case <-ctx.Done():
			return nil, ErrContextClosed
		default:
			if prev.distance >= minLen && prev.distance <= maxLen {
				var route routing.Route
				hop := routing.Hop{
					From: prev.previous.edge,
					To:   destination.edge,
					TpID: prev.previous.connections[destination.edge].ID,
				}
				route.Hops = append(route.Hops, hop)
				prevVertex := prev.previous
				for g.prev[prevVertex] != nil {
					hop := routing.Hop{
						From: g.prev[prevVertex].edge,
						To:   prevVertex.edge,
						TpID: g.prev[prevVertex].connections[prevVertex.edge].ID,
					}
					route.Hops = append(route.Hops, hop)
					prevVertex = g.prev[prevVertex]
				}

				// because we are backtracking routes are reversed
				route = reverseRoute(route)
				routes = append(routes, route)
			}
		}
	}

	if len(routes) == 0 {
		return nil, ErrRouteNotFound
	}
	return routes, nil
}

func reverseRoute(r routing.Route) routing.Route {
	for left, right := 0, len(r.Hops)-1; left < right; left, right = left+1, right-1 {
		r.Hops[left], r.Hops[right] = r.Hops[right], r.Hops[left]
	}

	return r
}
