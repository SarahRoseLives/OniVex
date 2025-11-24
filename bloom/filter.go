package bloom

import (
	"hash/fnv"
	"math"
)

type Filter struct {
	BitSet []bool `json:"bitset"`
	K      uint   `json:"k"` // Number of hash functions
	M      uint   `json:"m"` // Size of bitset
}

func New(n uint, fpRate float64) *Filter {
	m := uint(math.Ceil(float64(n) * math.Log(fpRate) / math.Log(1.0/math.Pow(2.0, math.Log(2.0)))))
	k := uint(math.Round(math.Log(2.0) * float64(m) / float64(n)))
	return &Filter{
		BitSet: make([]bool, m),
		K:      k,
		M:      m,
	}
}

func (f *Filter) Add(data []byte) {
	h := fnv.New64a()
	h.Write(data)
	hash1 := h.Sum64()
	h.Write(data)
	hash2 := h.Sum64()

	for i := uint(0); i < f.K; i++ {
		ind := (hash1 + uint64(i)*hash2) % uint64(f.M)
		f.BitSet[ind] = true
	}
}

func (f *Filter) Test(data []byte) bool {
	h := fnv.New64a()
	h.Write(data)
	hash1 := h.Sum64()
	h.Write(data)
	hash2 := h.Sum64()

	for i := uint(0); i < f.K; i++ {
		ind := (hash1 + uint64(i)*hash2) % uint64(f.M)
		if !f.BitSet[ind] {
			return false
		}
	}
	return true
}