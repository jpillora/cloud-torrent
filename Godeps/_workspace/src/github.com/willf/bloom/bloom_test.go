package bloom

import (
	"bytes"
	"encoding/binary"
	"encoding/gob"
	"encoding/json"
	"fmt"
	"runtime"
	"sync"
	"testing"
)

func TestConcurrent(t *testing.T) {
	gmp := runtime.GOMAXPROCS(2)
	defer runtime.GOMAXPROCS(gmp)

	f := New(1000, 4)
	n1 := []byte("Bess")
	n2 := []byte("Jane")
	f.Add(n1)
	f.Add(n2)

	var wg sync.WaitGroup
	const try = 1000
	var err1, err2 error

	wg.Add(1)
	go func() {
		for i := 0; i < try; i++ {
			n1b := f.Test(n1)
			if !n1b {
				err1 = fmt.Errorf("%v should be in", n1)
				break
			}
		}
		wg.Done()
	}()

	wg.Add(1)
	go func() {
		for i := 0; i < try; i++ {
			n2b := f.Test(n2)
			if !n2b {
				err2 = fmt.Errorf("%v should be in", n2)
				break
			}
		}
		wg.Done()
	}()

	wg.Wait()

	if err1 != nil {
		t.Fatal(err1)
	}
	if err2 != nil {
		t.Fatal(err2)
	}
}

func TestBasic(t *testing.T) {
	f := New(1000, 4)
	n1 := []byte("Bess")
	n2 := []byte("Jane")
	n3 := []byte("Emma")
	f.Add(n1)
	n3a := f.TestAndAdd(n3)
	n1b := f.Test(n1)
	n2b := f.Test(n2)
	n3b := f.Test(n3)
	if !n1b {
		t.Errorf("%v should be in.", n1)
	}
	if n2b {
		t.Errorf("%v should not be in.", n2)
	}
	if n3a {
		t.Errorf("%v should not be in the first time we look.", n3)
	}
	if !n3b {
		t.Errorf("%v should be in the second time we look.", n3)
	}
}

func TestBasicUint32(t *testing.T) {
	f := New(1000, 4)
	n1 := make([]byte, 4)
	n2 := make([]byte, 4)
	n3 := make([]byte, 4)
	n4 := make([]byte, 4)
	binary.BigEndian.PutUint32(n1, 100)
	binary.BigEndian.PutUint32(n2, 101)
	binary.BigEndian.PutUint32(n3, 102)
	binary.BigEndian.PutUint32(n4, 103)
	f.Add(n1)
	n3a := f.TestAndAdd(n3)
	n1b := f.Test(n1)
	n2b := f.Test(n2)
	n3b := f.Test(n3)
	f.Test(n4)
	if !n1b {
		t.Errorf("%v should be in.", n1)
	}
	if n2b {
		t.Errorf("%v should not be in.", n2)
	}
	if n3a {
		t.Errorf("%v should not be in the first time we look.", n3)
	}
	if !n3b {
		t.Errorf("%v should be in the second time we look.", n3)
	}
}

func TestString(t *testing.T) {
	f := NewWithEstimates(1000, 0.001)
	n1 := "Love"
	n2 := "is"
	n3 := "in"
	n4 := "bloom"
	f.AddString(n1)
	n3a := f.TestAndAddString(n3)
	n1b := f.TestString(n1)
	n2b := f.TestString(n2)
	n3b := f.TestString(n3)
	f.TestString(n4)
	if !n1b {
		t.Errorf("%v should be in.", n1)
	}
	if n2b {
		t.Errorf("%v should not be in.", n2)
	}
	if n3a {
		t.Errorf("%v should not be in the first time we look.", n3)
	}
	if !n3b {
		t.Errorf("%v should be in the second time we look.", n3)
	}

}

func testEstimated(n uint, maxFp float64, t *testing.T) {
	m, k := EstimateParameters(n, maxFp)
	f := NewWithEstimates(n, maxFp)
	fpRate := f.EstimateFalsePositiveRate(n)
	if fpRate > 1.10*maxFp {
		t.Errorf("False positive rate too high: n: %v; m: %v; k: %v; maxFp: %f; fpRate: %f, fpRate/maxFp: %f", n, m, k, maxFp, fpRate, fpRate/maxFp)
	}
}

func TestEstimated1000_0001(t *testing.T)     { testEstimated(1000, 0.000100, t) }
func TestEstimated10000_0001(t *testing.T)    { testEstimated(10000, 0.000100, t) }
func TestEstimated100000_0001(t *testing.T)   { testEstimated(100000, 0.000100, t) }
func TestEstimated1000000_0001(t *testing.T)  { testEstimated(1000000, 0.000100, t) }
func TestEstimated10000000_0001(t *testing.T) { testEstimated(10000000, 0.000100, t) }
func TestEstimated1000_001(t *testing.T)      { testEstimated(1000, 0.001000, t) }
func TestEstimated10000_001(t *testing.T)     { testEstimated(10000, 0.001000, t) }
func TestEstimated100000_001(t *testing.T)    { testEstimated(100000, 0.001000, t) }
func TestEstimated1000000_001(t *testing.T)   { testEstimated(1000000, 0.001000, t) }
func TestEstimated10000000_001(t *testing.T)  { testEstimated(10000000, 0.001000, t) }
func TestEstimated1000_01(t *testing.T)       { testEstimated(1000, 0.010000, t) }
func TestEstimated10000_01(t *testing.T)      { testEstimated(10000, 0.010000, t) }
func TestEstimated100000_01(t *testing.T)     { testEstimated(100000, 0.010000, t) }
func TestEstimated1000000_01(t *testing.T)    { testEstimated(1000000, 0.010000, t) }
func TestEstimated10000000_01(t *testing.T)   { testEstimated(10000000, 0.010000, t) }

func TestMarshalUnmarshalJSON(t *testing.T) {
	f := New(1000, 4)
	data, err := json.Marshal(f)
	if err != nil {
		t.Fatal(err.Error())
	}

	var g BloomFilter
	err = json.Unmarshal(data, &g)
	if err != nil {
		t.Fatal(err.Error())
	}
	if g.m != f.m {
		t.Error("invalid m value")
	}
	if g.k != f.k {
		t.Error("invalid k value")
	}
	if g.b == nil {
		t.Fatal("bitset is nil")
	}
	if !g.b.Equal(f.b) {
		t.Error("bitsets are not equal")
	}
}

func TestWriteToReadFrom(t *testing.T) {
	var b bytes.Buffer
	f := New(1000, 4)
	_, err := f.WriteTo(&b)
	if err != nil {
		t.Fatal(err)
	}

	g := New(1000, 1)
	_, err = g.ReadFrom(&b)
	if err != nil {
		t.Fatal(err)
	}
	if g.m != f.m {
		t.Error("invalid m value")
	}
	if g.k != f.k {
		t.Error("invalid k value")
	}
	if g.b == nil {
		t.Fatal("bitset is nil")
	}
	if !g.b.Equal(f.b) {
		t.Error("bitsets are not equal")
	}

	g.Test([]byte(""))
}

func TestReadWriteBinary(t *testing.T) {
	f := New(1000, 4)
	var buf bytes.Buffer
	bytesWritten, err := f.WriteTo(&buf)
	if err != nil {
		t.Fatal(err.Error())
	}
	if bytesWritten != int64(buf.Len()) {
		t.Errorf("incorrect write length %d != %d", bytesWritten, buf.Len())
	}

	var g BloomFilter
	bytesRead, err := g.ReadFrom(&buf)
	if err != nil {
		t.Fatal(err.Error())
	}
	if bytesRead != bytesWritten {
		t.Errorf("read unexpected number of bytes %d != %d", bytesRead, bytesWritten)
	}
	if g.m != f.m {
		t.Error("invalid m value")
	}
	if g.k != f.k {
		t.Error("invalid k value")
	}
	if g.b == nil {
		t.Fatal("bitset is nil")
	}
	if !g.b.Equal(f.b) {
		t.Error("bitsets are not equal")
	}
}

func TestEncodeDecodeGob(t *testing.T) {
	f := New(1000, 4)
	f.Add([]byte("one"))
	f.Add([]byte("two"))
	f.Add([]byte("three"))
	var buf bytes.Buffer
	err := gob.NewEncoder(&buf).Encode(f)
	if err != nil {
		t.Fatal(err.Error())
	}

	var g BloomFilter
	err = gob.NewDecoder(&buf).Decode(&g)
	if err != nil {
		t.Fatal(err.Error())
	}
	if g.m != f.m {
		t.Error("invalid m value")
	}
	if g.k != f.k {
		t.Error("invalid k value")
	}
	if g.b == nil {
		t.Fatal("bitset is nil")
	}
	if !g.b.Equal(f.b) {
		t.Error("bitsets are not equal")
	}
	if !g.Test([]byte("three")) {
		t.Errorf("missing value 'three'")
	}
	if !g.Test([]byte("two")) {
		t.Errorf("missing value 'two'")
	}
	if !g.Test([]byte("one")) {
		t.Errorf("missing value 'one'")
	}
}

func BenchmarkEstimated(b *testing.B) {
	for n := uint(100000); n <= 100000; n *= 10 {
		for fp := 0.1; fp >= 0.0001; fp /= 10.0 {
			f := NewWithEstimates(n, fp)
			f.EstimateFalsePositiveRate(n)
		}
	}
}

func BenchmarkSeparateTestAndAdd(b *testing.B) {
	f := NewWithEstimates(uint(b.N), 0.0001)
	key := make([]byte, 100)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		binary.BigEndian.PutUint32(key, uint32(i))
		f.Test(key)
		f.Add(key)
	}
}

func BenchmarkCombinedTestAndAdd(b *testing.B) {
	f := NewWithEstimates(uint(b.N), 0.0001)
	key := make([]byte, 100)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		binary.BigEndian.PutUint32(key, uint32(i))
		f.TestAndAdd(key)
	}
}

func MergeTest(b *testing.B) {
	f := New(1000, 4)
	n1 := []byte("f")
	f.Add(n1)

	g := New(1000, 4)
	n2 := []byte("g")
	g.Add(n2)

	h := New(999, 4)
	n3 := []byte("h")
	h.Add(n3)

	j := New(1000, 5)
	n4 := []byte("j")
	j.Add(n4)

	var err error

	err = f.Merge(g)
	if err != nil {
		b.Errorf("There should be no error when merging two similar filters")
	}

	err = f.Merge(h)
	if err == nil {
		b.Errorf("There should be an error when merging filters with mismatched m")
	}

	err = f.Merge(j)
	if err == nil {
		b.Errorf("There should be an error when merging filters with mismatched k")
	}

	n2b := f.Test(n2)
	if !n2b {
		b.Errorf("The value doesn't exist after a valid merge")
	}

	n3b := f.Test(n3)
	if n3b {
		b.Errorf("The value exists after an invalid merge")
	}

	n4b := f.Test(n4)
	if n4b {
		b.Errorf("The value exists after an invalid merge")
	}
}

func CopyTest(b *testing.B) {
	f := New(1000, 4)
	n1 := []byte("f")
	f.Add(n1)

	// copy here instead of New
	g := f.Copy()
	n2 := []byte("g")
	g.Add(n2)

	n1fb := f.Test(n1)
	if !n1fb {
		b.Errorf("The value doesn't exist in original after making a copy")
	}

	n1gb := g.Test(n1)
	if !n1gb {
		b.Errorf("The value doesn't exist in the copy")
	}

	n2fb := f.Test(n2)
	if n2fb {
		b.Errorf("The value exists in the original, it should only exist in copy")
	}

	n2gb := g.Test(n2)
	if !n2gb {
		b.Errorf("The value doesn't exist in copy after Add()")
	}
}
