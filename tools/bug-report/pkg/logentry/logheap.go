package logentry

import "istio.io/pkg/log"

type LogHeap []*LogEntry

func (h LogHeap) Len() int {
	return len(h)
}

func (h LogHeap) Less(i, j int) bool {
	if h[i].LogImportance != h[j].LogImportance {
		return h[i].LogImportance > h[j].LogImportance
	}
	return h[i].CompressedSize < h[j].CompressedSize
}

func (h LogHeap) Swap(i, j int) {
	h[i], h[j] = h[j], h[i]
}

func (h *LogHeap) Push(x interface{}) {
	add := x.(*LogEntry)
	log.Infof("Adding -- %s", add.Path)
	*h = append(*h, add)
}

func (h *LogHeap) Pop() interface{} {
	old := *h
	n := len(old)
	ret := old[n-1]
	old[n-1] = nil
	*h = old[0:n-1]
	return ret
}

func (h LogHeap) Peek() interface{} {
	return h[h.Len()-1]
}



