package store

// implementation from https://rosettacode.org/wiki/Dijkstra%27s_algorithm#Go
// A priorityQueue implements heap.Interface and holds Items.
// Adjust priorityQueue to handle queueElement
type priorityQueue struct {
	items      []*queueElement
	priorities map[*vertex]int
	index      map[*vertex]int
}

func (pq *priorityQueue) Len() int           { return len(pq.items) }
func (pq *priorityQueue) Less(i, j int) bool { return pq.items[i].distance < pq.items[j].distance }
func (pq *priorityQueue) Swap(i, j int) {
	pq.items[i], pq.items[j] = pq.items[j], pq.items[i]
	pq.index[pq.items[i].vertex] = i
	pq.index[pq.items[j].vertex] = j
}

func (pq *priorityQueue) Push(x interface{}) {
	item := x.(*queueElement)
	pq.index[item.vertex] = len(pq.items)
	pq.items = append(pq.items, item)
}

func (pq *priorityQueue) Pop() interface{} {
	old := pq.items
	n := len(old)
	item := old[n-1]
	pq.index[item.vertex] = -1
	pq.items = old[0 : n-1]
	return item
}
