package index

// TopKDistanceEntry is an index + distance pair.
type TopKDistanceEntry struct {
	Index    int
	Distance float64
}

// TopKDistanceCollector keeps the k smallest distances (max-heap of size k).
type TopKDistanceCollector struct {
	k    int
	heap []TopKDistanceEntry
}

// NewTopKDistanceCollector creates a collector.
func NewTopKDistanceCollector(k int) *TopKDistanceCollector {
	return &TopKDistanceCollector{k: k}
}

// Offer considers a candidate.
func (c *TopKDistanceCollector) Offer(index int, distance float64) {
	if c.k <= 0 {
		return
	}
	if len(c.heap) < c.k {
		c.heap = append(c.heap, TopKDistanceEntry{Index: index, Distance: distance})
		c.bubbleUp(len(c.heap) - 1)
		return
	}
	if distance < c.heap[0].Distance {
		c.heap[0] = TopKDistanceEntry{Index: index, Distance: distance}
		c.siftDown(0)
	}
}

// Finish returns entries sorted by ascending distance.
func (c *TopKDistanceCollector) Finish() []TopKDistanceEntry {
	out := make([]TopKDistanceEntry, len(c.heap))
	copy(out, c.heap)
	// insertion sort by distance asc (k is small)
	for i := 1; i < len(out); i++ {
		j := i
		for j > 0 && out[j].Distance < out[j-1].Distance {
			out[j], out[j-1] = out[j-1], out[j]
			j--
		}
	}
	return out
}

func (c *TopKDistanceCollector) bubbleUp(i int) {
	for i > 0 {
		p := (i - 1) >> 1
		if c.heap[i].Distance <= c.heap[p].Distance {
			break
		}
		c.heap[i], c.heap[p] = c.heap[p], c.heap[i]
		i = p
	}
}

func (c *TopKDistanceCollector) siftDown(i int) {
	for {
		left := (i << 1) + 1
		if left >= len(c.heap) {
			break
		}
		largest := left
		right := left + 1
		if right < len(c.heap) && c.heap[right].Distance > c.heap[left].Distance {
			largest = right
		}
		if c.heap[largest].Distance <= c.heap[i].Distance {
			break
		}
		c.heap[i], c.heap[largest] = c.heap[largest], c.heap[i]
		i = largest
	}
}

type scorePair struct {
	v float64
	i int
}

// SelectTopKScoreIndices returns indices of the k highest scores.
func SelectTopKScoreIndices(scores []float64, k int) []int {
	if k <= 0 || len(scores) == 0 {
		return nil
	}
	top := make([]scorePair, 0, k)
	for i, v := range scores {
		if len(top) < k {
			top = append(top, scorePair{v: v, i: i})
			sortScoreAsc(top)
			continue
		}
		if v > top[0].v {
			top[0] = scorePair{v: v, i: i}
			sortScoreAsc(top)
		}
	}
	sortScoreDesc(top)
	out := make([]int, len(top))
	for i, p := range top {
		out[i] = p.i
	}
	return out
}

func sortScoreAsc(top []scorePair) {
	for i := 1; i < len(top); i++ {
		j := i
		for j > 0 && top[j].v < top[j-1].v {
			top[j], top[j-1] = top[j-1], top[j]
			j--
		}
	}
}

func sortScoreDesc(top []scorePair) {
	for i := 1; i < len(top); i++ {
		j := i
		for j > 0 && top[j].v > top[j-1].v {
			top[j], top[j-1] = top[j-1], top[j]
			j--
		}
	}
}
