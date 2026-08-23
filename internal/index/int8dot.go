package index

// QuantizedVector is symmetric int8 codes with a scale.
type QuantizedVector struct {
	Codes []int8
	Scale float64
}

// QuantizedDotFlat computes int8 dot * scales (matches int8-dot.ts).
func QuantizedDotFlat(query QuantizedVector, codes []int8, offset, dim int, docScale float64) float64 {
	q := query.Codes
	var s0, s1, s2, s3, s4, s5, s6, s7 int
	i := 0
	for ; i+7 < dim; i += 8 {
		s0 += int(q[i]) * int(codes[offset+i])
		s1 += int(q[i+1]) * int(codes[offset+i+1])
		s2 += int(q[i+2]) * int(codes[offset+i+2])
		s3 += int(q[i+3]) * int(codes[offset+i+3])
		s4 += int(q[i+4]) * int(codes[offset+i+4])
		s5 += int(q[i+5]) * int(codes[offset+i+5])
		s6 += int(q[i+6]) * int(codes[offset+i+6])
		s7 += int(q[i+7]) * int(codes[offset+i+7])
	}
	sum := s0 + s1 + s2 + s3 + s4 + s5 + s6 + s7
	for ; i < dim; i++ {
		sum += int(q[i]) * int(codes[offset+i])
	}
	return float64(sum) * query.Scale * docScale
}
