package roaring

import (
	"bytes"
	"fmt"
	"math/rand"
	"runtime"
	"testing"

	"github.com/willf/bitset"
)

// BENCHMARKS, to run them type "go test -bench Benchmark -run -"

var c uint

// go test -bench BenchmarkMemoryUsage -run -
func BenchmarkMemoryUsage(b *testing.B) {
	b.StopTimer()
	bitmaps := make([]*Bitmap, 0, 10)

	incr := uint32(1 << 16)
	max := uint32(1<<32 - 1)
	for x := 0; x < 10; x++ {
		rb := NewBitmap()

		var i uint32
		for i = 0; i <= max-incr; i += incr {
			rb.Add(i)
		}

		bitmaps = append(bitmaps, rb)
	}

	var stats runtime.MemStats
	runtime.ReadMemStats(&stats)
	fmt.Printf("\nHeapInUse %d\n", stats.HeapInuse)
	fmt.Printf("HeapObjects %d\n", stats.HeapObjects)
	b.StartTimer()
}

// go test -bench BenchmarkIntersection -run -
func BenchmarkIntersectionBitset(b *testing.B) {
	b.StopTimer()
	r := rand.New(rand.NewSource(0))
	s1 := bitset.New(0)
	sz := 150000
	initsize := 65000
	for i := 0; i < initsize; i++ {
		s1.Set(uint(r.Int31n(int32(sz))))
	}
	s2 := bitset.New(0)
	sz = 100000000
	initsize = 65000
	for i := 0; i < initsize; i++ {
		s2.Set(uint(r.Int31n(int32(sz))))
	}
	b.StartTimer()
	card := uint(0)
	for j := 0; j < b.N; j++ {
		s3 := s1.Intersection(s2)
		card = card + s3.Count()
	}
}

// go test -bench BenchmarkIntersection -run -
func BenchmarkIntersectionRoaring(b *testing.B) {
	b.StopTimer()
	r := rand.New(rand.NewSource(0))
	s1 := NewBitmap()
	sz := 150000
	initsize := 65000
	for i := 0; i < initsize; i++ {
		s1.Add(uint32(r.Int31n(int32(sz))))
	}
	s2 := NewBitmap()
	sz = 100000000
	initsize = 65000
	for i := 0; i < initsize; i++ {
		s2.Add(uint32(r.Int31n(int32(sz))))
	}
	b.StartTimer()
	card := uint64(0)
	for j := 0; j < b.N; j++ {
		s3 := And(s1, s2)
		card = card + s3.GetCardinality()
	}
}

// go test -bench BenchmarkUnion -run -
func BenchmarkUnionBitset(b *testing.B) {
	b.StopTimer()
	r := rand.New(rand.NewSource(0))
	s1 := bitset.New(0)
	sz := 150000
	initsize := 65000
	for i := 0; i < initsize; i++ {
		s1.Set(uint(r.Int31n(int32(sz))))
	}
	s2 := bitset.New(0)
	sz = 100000000
	initsize = 65000
	for i := 0; i < initsize; i++ {
		s2.Set(uint(r.Int31n(int32(sz))))
	}
	b.StartTimer()
	card := uint(0)
	for j := 0; j < b.N; j++ {
		s3 := s1.Union(s2)
		card = card + s3.Count()
	}
}

// go test -bench BenchmarkUnion -run -
func BenchmarkUnionRoaring(b *testing.B) {
	b.StopTimer()
	r := rand.New(rand.NewSource(0))
	s1 := NewBitmap()
	sz := 150000
	initsize := 65000
	for i := 0; i < initsize; i++ {
		s1.Add(uint32(r.Int31n(int32(sz))))
	}
	s2 := NewBitmap()
	sz = 100000000
	initsize = 65000
	for i := 0; i < initsize; i++ {
		s2.Add(uint32(r.Int31n(int32(sz))))
	}
	b.StartTimer()
	card := uint64(0)
	for j := 0; j < b.N; j++ {
		s3 := Or(s1, s2)
		card = card + s3.GetCardinality()
	}
}

// go test -bench BenchmarkSize -run -
func BenchmarkSizeBitset(b *testing.B) {
	b.StopTimer()
	r := rand.New(rand.NewSource(0))
	s1 := bitset.New(0)
	sz := 150000
	initsize := 65000
	for i := 0; i < initsize; i++ {
		s1.Set(uint(r.Int31n(int32(sz))))
	}
	s2 := bitset.New(0)
	sz = 100000000
	initsize = 65000
	for i := 0; i < initsize; i++ {
		s2.Set(uint(r.Int31n(int32(sz))))
	}
	fmt.Printf("%.1f MB ", float32(s1.BinaryStorageSize()+s2.BinaryStorageSize())/(1024.0*1024))

}

// go test -bench BenchmarkSize -run -
func BenchmarkSizeRoaring(b *testing.B) {
	b.StopTimer()
	r := rand.New(rand.NewSource(0))
	s1 := NewBitmap()
	sz := 150000
	initsize := 65000
	for i := 0; i < initsize; i++ {
		s1.Add(uint32(r.Int31n(int32(sz))))
	}
	s2 := NewBitmap()
	sz = 100000000
	initsize = 65000
	for i := 0; i < initsize; i++ {
		s2.Add(uint32(r.Int31n(int32(sz))))
	}
	fmt.Printf("%.1f MB ", float32(s1.GetSerializedSizeInBytes()+s2.GetSerializedSizeInBytes())/(1024.0*1024))

}

// go test -bench BenchmarkSet -run -
func BenchmarkSetRoaring(b *testing.B) {
	b.StopTimer()
	r := rand.New(rand.NewSource(0))
	sz := 1000000
	s := NewBitmap()
	b.StartTimer()
	for i := 0; i < b.N; i++ {
		s.Add(uint32(r.Int31n(int32(sz))))
	}
}

func BenchmarkSetBitset(b *testing.B) {
	b.StopTimer()
	r := rand.New(rand.NewSource(0))
	sz := 1000000
	s := bitset.New(0)
	b.StartTimer()
	for i := 0; i < b.N; i++ {
		s.Set(uint(r.Int31n(int32(sz))))
	}
}

// go test -bench BenchmarkGetTest -run -
func BenchmarkGetTestRoaring(b *testing.B) {
	b.StopTimer()
	r := rand.New(rand.NewSource(0))
	sz := 1000000
	initsize := 50000
	s := NewBitmap()
	for i := 0; i < initsize; i++ {
		s.Add(uint32(r.Int31n(int32(sz))))
	}
	b.StartTimer()
	for i := 0; i < b.N; i++ {
		s.Contains(uint32(r.Int31n(int32(sz))))
	}
}

func BenchmarkGetTestBitSet(b *testing.B) {
	b.StopTimer()
	r := rand.New(rand.NewSource(0))
	sz := 1000000
	initsize := 50000
	s := bitset.New(0)
	for i := 0; i < initsize; i++ {
		s.Set(uint(r.Int31n(int32(sz))))
	}
	b.StartTimer()
	for i := 0; i < b.N; i++ {
		s.Test(uint(r.Int31n(int32(sz))))
	}
}

// go test -bench BenchmarkCount -run -
func BenchmarkCountRoaring(b *testing.B) {
	b.StopTimer()
	r := rand.New(rand.NewSource(0))
	s := NewBitmap()
	sz := 1000000
	initsize := 50000
	for i := 0; i < initsize; i++ {
		s.Add(uint32(r.Int31n(int32(sz))))
	}
	b.StartTimer()
	for i := 0; i < b.N; i++ {
		s.GetCardinality()
	}
}

func BenchmarkCountBitset(b *testing.B) {
	b.StopTimer()
	r := rand.New(rand.NewSource(0))
	s := bitset.New(0)
	sz := 1000000
	initsize := 50000
	for i := 0; i < initsize; i++ {

		s.Set(uint(r.Int31n(int32(sz))))
	}
	b.StartTimer()
	for i := 0; i < b.N; i++ {
		s.Count()
	}
}

// go test -bench BenchmarkIterate -run -
func BenchmarkIterateRoaring(b *testing.B) {
	b.StopTimer()
	r := rand.New(rand.NewSource(0))
	s := NewBitmap()
	sz := 150000
	initsize := 65000
	for i := 0; i < initsize; i++ {
		s.Add(uint32(r.Int31n(int32(sz))))
	}
	b.StartTimer()
	for j := 0; j < b.N; j++ {
		c = uint(0)
		i := s.Iterator()
		for i.HasNext() {
			i.Next()
			c++
		}
	}
}

// go test -bench BenchmarkSparseIterate -run -
func BenchmarkSparseIterateRoaring(b *testing.B) {
	b.StopTimer()
	r := rand.New(rand.NewSource(0))
	s := NewBitmap()
	sz := 100000000
	initsize := 65000
	for i := 0; i < initsize; i++ {
		s.Add(uint32(r.Int31n(int32(sz))))
	}
	b.StartTimer()
	for j := 0; j < b.N; j++ {
		c = uint(0)
		i := s.Iterator()
		for i.HasNext() {
			i.Next()
			c++
		}
	}

}

// go test -bench BenchmarkIterate -run -
func BenchmarkIterateBitset(b *testing.B) {
	b.StopTimer()
	r := rand.New(rand.NewSource(0))
	s := bitset.New(0)
	sz := 150000
	initsize := 65000
	for i := 0; i < initsize; i++ {
		s.Set(uint(r.Int31n(int32(sz))))
	}
	b.StartTimer()
	for j := 0; j < b.N; j++ {
		c = uint(0)
		for i, e := s.NextSet(0); e; i, e = s.NextSet(i + 1) {
			c++
		}
	}
}

// go test -bench BenchmarkSparseContains -run -
func BenchmarkSparseContains(b *testing.B) {
	b.StopTimer()
	r := rand.New(rand.NewSource(0))
	s := NewBitmap()
	sz := 10000000
	initsize := 65000
	for i := 0; i < initsize; i++ {
		s.Add(uint32(r.Int31n(int32(sz))))
	}
	var a [1024]uint32
	for i := 0; i < 1024; i++ {
		a[i] = uint32(r.Int31n(int32(sz)))
	}
	b.StartTimer()
	for j := 0; j < b.N; j++ {
		c = uint(0)
		for i := 0; i < 1024; i++ {
			if s.Contains(a[i]) {
				c++
			}

		}
	}
}

// go test -bench BenchmarkSparseIterate -run -
func BenchmarkSparseIterateBitset(b *testing.B) {
	b.StopTimer()
	r := rand.New(rand.NewSource(0))
	s := bitset.New(0)
	sz := 100000000
	initsize := 65000
	for i := 0; i < initsize; i++ {
		s.Set(uint(r.Int31n(int32(sz))))
	}
	b.StartTimer()
	for j := 0; j < b.N; j++ {
		c = uint(0)
		for i, e := s.NextSet(0); e; i, e = s.NextSet(i + 1) {
			c++
		}
	}
}

func BenchmarkSerializationSparse(b *testing.B) {
	b.StopTimer()
	r := rand.New(rand.NewSource(0))
	s := NewBitmap()
	sz := 100000000
	initsize := 65000
	for i := 0; i < initsize; i++ {
		s.Add(uint32(r.Int31n(int32(sz))))
	}
	buf := make([]byte, 0, s.GetSerializedSizeInBytes())
	b.StartTimer()

	for j := 0; j < b.N; j++ {
		w := bytes.NewBuffer(buf[:0])
		s.WriteTo(w)
	}
}

func BenchmarkSerializationMid(b *testing.B) {
	b.StopTimer()
	r := rand.New(rand.NewSource(0))
	s := NewBitmap()
	sz := 10000000
	initsize := 65000
	for i := 0; i < initsize; i++ {
		s.Add(uint32(r.Int31n(int32(sz))))
	}
	buf := make([]byte, 0, s.GetSerializedSizeInBytes())
	b.StartTimer()

	for j := 0; j < b.N; j++ {
		w := bytes.NewBuffer(buf[:0])
		s.WriteTo(w)
	}
}

func BenchmarkSerializationDense(b *testing.B) {
	b.StopTimer()
	r := rand.New(rand.NewSource(0))
	s := NewBitmap()
	sz := 150000
	initsize := 65000
	for i := 0; i < initsize; i++ {
		s.Add(uint32(r.Int31n(int32(sz))))
	}
	buf := make([]byte, 0, s.GetSerializedSizeInBytes())
	b.StartTimer()

	for j := 0; j < b.N; j++ {
		w := bytes.NewBuffer(buf[:0])
		s.WriteTo(w)
	}
}

func BenchmarkEqualsSparse(b *testing.B) {
	b.StopTimer()
	r := rand.New(rand.NewSource(0))
	s := NewBitmap()
	t := NewBitmap()
	sz := 100000000
	initsize := 65000
	for i := 0; i < initsize; i++ {
		n := uint32(r.Int31n(int32(sz)))
		s.Add(n)
		t.Add(n)
	}
	b.StartTimer()

	for j := 0; j < b.N; j++ {
		s.Equals(t)
	}
}

func BenchmarkEqualsClone(b *testing.B) {
	b.StopTimer()
	r := rand.New(rand.NewSource(0))
	s := NewBitmap()
	sz := 100000000
	initsize := 65000
	for i := 0; i < initsize; i++ {
		s.Add(uint32(r.Int31n(int32(sz))))
	}
	t := s.Clone()
	b.StartTimer()

	for j := 0; j < b.N; j++ {
		s.Equals(t)
	}
}

func BenchmarkSequentialAdd(b *testing.B) {
	for j := 0; j < b.N; j++ {
		s := NewBitmap()
		for i := 0; i < 10000000; i += 16 {
			s.Add(uint32(i))
		}
	}
}

func BenchmarkXor(b *testing.B) {
	b.StopTimer()
	r := rand.New(rand.NewSource(0))
	s := NewBitmap()
	sz := 100000000
	initsize := 65000
	for i := 0; i < initsize; i++ {
		n := uint32(r.Int31n(int32(sz)))
		s.Add(n)
	}
	x2 := NewBitmap()
	for i := 0; i < initsize; i++ {
		n := uint32(r.Int31n(int32(sz)))
		x2.Add(n)
	}
	b.StartTimer()

	for j := 0; j < b.N; j++ {
		s.Clone().Xor(x2)
	}
}

func BenchmarkXorLopsided(b *testing.B) {
	b.StopTimer()
	r := rand.New(rand.NewSource(0))
	s := NewBitmap()
	sz := 100000000
	initsize := 65000
	for i := 0; i < initsize; i++ {
		n := uint32(r.Int31n(int32(sz)))
		s.Add(n)
	}
	x2 := NewBitmap()
	for i := 0; i < 32; i++ {
		n := uint32(r.Int31n(int32(sz)))
		x2.Add(n)
	}
	b.StartTimer()

	for j := 0; j < b.N; j++ {
		s.Clone().Xor(x2)
	}
}
