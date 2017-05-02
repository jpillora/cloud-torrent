package roaring

// to run just these tests: go test -run TestSerialization*

import (
	"bytes"
	"encoding/gob"
	"fmt"
	"io/ioutil"
	"math/rand"
	"os"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestSerializationOfEmptyBitmap(t *testing.T) {
	rb := NewBitmap()

	buf := &bytes.Buffer{}
	_, err := rb.WriteTo(buf)
	if err != nil {
		t.Errorf("Failed writing")
	}

	newrb := NewBitmap()
	_, err = newrb.ReadFrom(buf)
	if err != nil {
		t.Errorf("Failed reading: %v", err)
	}
	if !rb.Equals(newrb) {
		p("rb = '%s'", rb)
		p("but newrb = '%s'", newrb)
		t.Errorf("Cannot retrieve serialized version; rb != newrb")
	}
}

func TestBase64_036(t *testing.T) {
	rb := BitmapOf(1, 2, 3, 4, 5, 100, 1000)

	bstr, _ := rb.ToBase64()

	if bstr == "" {
		t.Errorf("ToBase64 failed returned empty string")
	}

	newrb := NewBitmap()

	_, err := newrb.FromBase64(bstr)

	if err != nil {
		t.Errorf("Failed reading from base64 string")
	}

	if !rb.Equals(newrb) {
		t.Errorf("comparing the base64 to and from failed cannot retrieve serialized version")
	}
}

func TestSerializationBasic037(t *testing.T) {

	rb := BitmapOf(1, 2, 3, 4, 5, 100, 1000)

	buf := &bytes.Buffer{}
	_, err := rb.WriteTo(buf)
	if err != nil {
		t.Errorf("Failed writing")
	}

	newrb := NewBitmap()
	_, err = newrb.ReadFrom(buf)
	if err != nil {
		t.Errorf("Failed reading")
	}
	if !rb.Equals(newrb) {
		p("rb = '%s'", rb)
		p("but newrb = '%s'", newrb)
		t.Errorf("Cannot retrieve serialized version; rb != newrb")
	}
}

func TestSerializationToFile038(t *testing.T) {
	rb := BitmapOf(1, 2, 3, 4, 5, 100, 1000)
	fname := "myfile.bin"
	fout, err := os.OpenFile(fname, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0660)
	if err != nil {
		t.Errorf("Can't open a file for writing")
	}
	_, err = rb.WriteTo(fout)
	if err != nil {
		t.Errorf("Failed writing")
	}
	fout.Close()

	newrb := NewBitmap()
	fin, err := os.Open(fname)

	if err != nil {
		t.Errorf("Failed reading")
	}
	defer func() {
		fin.Close()
		err := os.Remove(fname)
		if err != nil {
			t.Errorf("could not delete %s ", fname)
		}
	}()
	_, _ = newrb.ReadFrom(fin)
	if !rb.Equals(newrb) {
		t.Errorf("Cannot retrieve serialized version")
	}
}

func TestSerializationReadRunsFromFile039(t *testing.T) {
	fn := "testdata/bitmapwithruns.bin"

	p("reading file '%s'", fn)
	by, err := ioutil.ReadFile(fn)
	if err != nil {
		panic(err)
	}

	newrb := NewBitmap()
	_, err = newrb.ReadFrom(bytes.NewBuffer(by))
	if err != nil {
		t.Errorf("Failed reading %s: %s", fn, err)
	}
}

func TestSerializationBasic4WriteAndReadFile040(t *testing.T) {

	//fname := "testdata/all3.msgp.snappy"
	fname := "testdata/all3.classic"

	rb := NewBitmap()
	for k := uint32(0); k < 100000; k += 1000 {
		rb.Add(k)
	}
	for k := uint32(100000); k < 200000; k++ {
		rb.Add(3 * k)
	}
	for k := uint32(700000); k < 800000; k++ {
		rb.Add(k)
	}
	rb.highlowcontainer.runOptimize()

	p("TestSerializationBasic4WriteAndReadFile is writing to '%s'", fname)
	fout, err := os.Create(fname)
	if err != nil {
		t.Errorf("Failed creating '%s'", fname)
	}
	_, err = rb.WriteTo(fout)
	if err != nil {
		t.Errorf("Failed writing to '%s'", fname)
	}
	fout.Close()

	fin, err := os.Open(fname)
	if err != nil {
		t.Errorf("Failed to Open '%s'", fname)
	}
	defer fin.Close()

	newrb := NewBitmap()
	_, err = newrb.ReadFrom(fin)
	if err != nil {
		t.Errorf("Failed reading from '%s': %s", fname, err)
	}
	if !rb.Equals(newrb) {
		t.Errorf("Bad serialization")
	}
}

func TestSerializationFromJava051(t *testing.T) {
	fname := "testdata/bitmapwithoutruns.bin"
	newrb := NewBitmap()
	fin, err := os.Open(fname)

	if err != nil {
		t.Errorf("Failed reading")
	}
	defer func() {
		fin.Close()
	}()

	_, _ = newrb.ReadFrom(fin)
	fmt.Println(newrb.GetCardinality())
	rb := NewBitmap()
	for k := uint32(0); k < 100000; k += 1000 {
		rb.Add(k)
	}
	for k := uint32(100000); k < 200000; k++ {
		rb.Add(3 * k)
	}
	for k := uint32(700000); k < 800000; k++ {
		rb.Add(k)
	}
	fmt.Println(rb.GetCardinality())
	if !rb.Equals(newrb) {
		t.Errorf("Bad serialization")
	}

}

func TestSerializationFromJavaWithRuns052(t *testing.T) {
	fname := "testdata/bitmapwithruns.bin"
	newrb := NewBitmap()
	fin, err := os.Open(fname)

	if err != nil {
		t.Errorf("Failed reading")
	}
	defer func() {
		fin.Close()
	}()
	_, _ = newrb.ReadFrom(fin)
	rb := NewBitmap()
	for k := uint32(0); k < 100000; k += 1000 {
		rb.Add(k)
	}
	for k := uint32(100000); k < 200000; k++ {
		rb.Add(3 * k)
	}
	for k := uint32(700000); k < 800000; k++ {
		rb.Add(k)
	}
	if !rb.Equals(newrb) {
		t.Errorf("Bad serialization")
	}

}

func TestSerializationBasic2_041(t *testing.T) {

	rb := BitmapOf(1, 2, 3, 4, 5, 100, 1000, 10000, 100000, 1000000)
	buf := &bytes.Buffer{}
	sz := rb.GetSerializedSizeInBytes()
	ub := BoundSerializedSizeInBytes(rb.GetCardinality(), 1000001)
	if sz > ub+10 {
		t.Errorf("Bad GetSerializedSizeInBytes; sz=%v, upper-bound=%v", sz, ub)
	}
	l := int(rb.GetSerializedSizeInBytes())
	_, err := rb.WriteTo(buf)
	if err != nil {
		t.Errorf("Failed writing")
	}
	if l != buf.Len() {
		t.Errorf("Bad GetSerializedSizeInBytes")
	}
	newrb := NewBitmap()
	_, err = newrb.ReadFrom(buf)
	if err != nil {
		t.Errorf("Failed reading")
	}
	if !rb.Equals(newrb) {
		t.Errorf("Cannot retrieve serialized version")
	}
}

func TestSerializationBasic3_042(t *testing.T) {

	Convey("roaringarray.writeTo and .readFrom should serialize and unserialize when containing all 3 container types", t, func() {
		rb := BitmapOf(1, 2, 3, 4, 5, 100, 1000, 10000, 100000, 1000000)
		for i := 5000000; i < 5000000+2*(1<<16); i++ {
			rb.AddInt(i)
		}

		// confirm all three types present
		var bc, ac, rc bool
		for _, v := range rb.highlowcontainer.containers {
			switch cn := v.(type) {
			case *bitmapContainer:
				bc = true
			case *arrayContainer:
				ac = true
			case *runContainer16:
				rc = true
			default:
				panic(fmt.Errorf("Unrecognized container implementation: %T", cn))
			}
		}
		if !bc {
			t.Errorf("no bitmapContainer found, change your test input so we test all three!")
		}
		if !ac {
			t.Errorf("no arrayContainer found, change your test input so we test all three!")
		}
		if !rc {
			t.Errorf("no runContainer16 found, change your test input so we test all three!")
		}

		var buf bytes.Buffer
		_, err := rb.WriteTo(&buf)
		if err != nil {
			t.Errorf("Failed writing")
		}

		newrb := NewBitmap()
		_, err = newrb.ReadFrom(&buf)
		if err != nil {
			t.Errorf("Failed reading")
		}
		c1, c2 := rb.GetCardinality(), newrb.GetCardinality()
		So(c2, ShouldEqual, c1)
		So(newrb.Equals(rb), ShouldBeTrue)
		//fmt.Printf("\n Basic3: good: match on card = %v", c1)
	})
}

func TestGobcoding043(t *testing.T) {
	rb := BitmapOf(1, 2, 3, 4, 5, 100, 1000)

	buf := new(bytes.Buffer)
	encoder := gob.NewEncoder(buf)
	err := encoder.Encode(rb)
	if err != nil {
		t.Errorf("Gob encoding failed")
	}

	var b Bitmap
	decoder := gob.NewDecoder(buf)
	err = decoder.Decode(&b)
	if err != nil {
		t.Errorf("Gob decoding failed")
	}

	if !b.Equals(rb) {
		t.Errorf("Decoded bitmap does not equal input bitmap")
	}
}

func TestSerializationRunContainerMsgpack028(t *testing.T) {

	Convey("runContainer writeTo and readFrom should return logically equivalent containers", t, func() {
		seed := int64(42)
		p("seed is %v", seed)
		rand.Seed(seed)

		trials := []trial{
			{n: 10, percentFill: .2, ntrial: 10},
			{n: 10, percentFill: .8, ntrial: 10},
			{n: 10, percentFill: .50, ntrial: 10},
			/*
				trial{n: 10, percentFill: .01, ntrial: 10},
				trial{n: 1000, percentFill: .50, ntrial: 10},
				trial{n: 1000, percentFill: .99, ntrial: 10},
			*/
		}

		tester := func(tr trial) {
			for j := 0; j < tr.ntrial; j++ {
				p("TestSerializationRunContainerMsgpack028 on check# j=%v", j)

				ma := make(map[int]bool)

				n := tr.n
				a := []uint16{}

				draw := int(float64(n) * tr.percentFill)
				for i := 0; i < draw; i++ {
					r0 := rand.Intn(n)
					a = append(a, uint16(r0))
					ma[r0] = true
				}

				orig := newRunContainer16FromVals(false, a...)

				// serialize
				var buf bytes.Buffer
				_, err := orig.writeToMsgpack(&buf)
				if err != nil {
					panic(err)
				}

				// deserialize
				restored := &runContainer16{}
				_, err = restored.readFromMsgpack(&buf)
				if err != nil {
					panic(err)
				}

				// and compare
				So(restored.equals(orig), ShouldBeTrue)

			}
			p("done with serialization of runContainer16 check for trial %#v", tr)
		}

		for i := range trials {
			tester(trials[i])
		}

	})
}

func TestSerializationArrayOnly032(t *testing.T) {

	Convey("arrayContainer writeTo and readFrom should return logically equivalent containers, so long as you pre-size the write target properly", t, func() {

		seed := int64(42)
		p("seed is %v", seed)
		rand.Seed(seed)

		trials := []trial{
			{n: 101, percentFill: .50, ntrial: 10},
		}

		tester := func(tr trial) {
			for j := 0; j < tr.ntrial; j++ {
				p(" on check# j=%v", j)
				ma := make(map[int]bool)

				n := tr.n

				draw := int(float64(n) * tr.percentFill)
				for i := 0; i < draw; i++ {
					r0 := rand.Intn(n)
					ma[r0] = true
				}

				//showArray16(a, "a")

				// vs arrayContainer
				ac := newArrayContainer()
				for k := range ma {
					ac.iadd(uint16(k))
				}

				buf := &bytes.Buffer{}
				_, err := ac.writeTo(buf)
				panicOn(err)

				// have to pre-size the array write-target properly
				// by telling it the cardinality to read.
				ac2 := newArrayContainerSize(int(ac.getCardinality()))

				_, err = ac2.readFrom(buf)
				panicOn(err)
				So(ac2.String(), ShouldResemble, ac.String())
			}
			p("done with randomized writeTo/readFrom for arrayContainer"+
				" checks for trial %#v", tr)
		}

		for i := range trials {
			tester(trials[i])
		}
	})
}

func TestSerializationRunOnly033(t *testing.T) {

	Convey("runContainer16 writeTo and readFrom should return logically equivalent containers", t, func() {

		seed := int64(42)
		p("seed is %v", seed)
		rand.Seed(seed)

		trials := []trial{
			{n: 100, percentFill: .50, ntrial: 1},
		}

		tester := func(tr trial) {
			for j := 0; j < tr.ntrial; j++ {
				p(" on check# j=%v", j)
				ma := make(map[int]bool)

				n := tr.n

				draw := int(float64(n) * tr.percentFill)
				for i := 0; i < draw; i++ {
					r0 := rand.Intn(n)
					ma[r0] = true
				}

				ac := newRunContainer16()
				for k := range ma {
					ac.iadd(uint16(k))
				}

				buf := &bytes.Buffer{}
				_, err := ac.writeTo(buf)
				panicOn(err)

				ac2 := newRunContainer16()

				_, err = ac2.readFrom(buf)
				panicOn(err)
				So(ac2.equals(ac), ShouldBeTrue)
				So(ac2.String(), ShouldResemble, ac.String())
			}
			p("done with randomized writeTo/readFrom for runContainer16"+
				" checks for trial %#v", tr)
		}

		for i := range trials {
			tester(trials[i])
		}
	})
}

func TestSerializationBitmapOnly034(t *testing.T) {

	Convey("bitmapContainer writeTo and readFrom should return logically equivalent containers", t, func() {

		seed := int64(42)
		p("seed is %v", seed)
		rand.Seed(seed)

		trials := []trial{
			{n: 1010, percentFill: .50, ntrial: 10},
		}

		tester := func(tr trial) {
			for j := 0; j < tr.ntrial; j++ {
				p(" on check# j=%v", j)
				ma := make(map[int]bool)

				n := tr.n

				draw := int(float64(n) * tr.percentFill)
				for i := 0; i < draw; i++ {
					r0 := rand.Intn(n)
					ma[r0] = true
				}

				//showArray16(a, "a")

				bc := newBitmapContainer()
				for k := range ma {
					bc.iadd(uint16(k))
				}

				buf := &bytes.Buffer{}
				_, err := bc.writeTo(buf)
				panicOn(err)

				bc2 := newBitmapContainer()

				_, err = bc2.readFrom(buf)
				panicOn(err)
				So(bc2.String(), ShouldResemble, bc.String())
				So(bc2.equals(bc), ShouldBeTrue)
			}
			p("done with randomized writeTo/readFrom for bitmapContainer"+
				" checks for trial %#v", tr)
		}

		for i := range trials {
			tester(trials[i])
		}
	})
}

func TestSerializationBasicMsgpack035(t *testing.T) {

	Convey("roaringarray.writeToMsgpack and .readFromMsgpack should serialize and unserialize when containing all 3 container types", t, func() {
		rb := BitmapOf(1, 2, 3, 4, 5, 100, 1000, 10000, 100000, 1000000)
		for i := 5000000; i < 5000000+2*(1<<16); i++ {
			rb.AddInt(i)
		}

		// confirm all three types present
		var bc, ac, rc bool
		for _, v := range rb.highlowcontainer.containers {
			switch cn := v.(type) {
			case *bitmapContainer:
				bc = true
				So(cn.containerType(), ShouldEqual, bitmapContype)
			case *arrayContainer:
				ac = true
				So(cn.containerType(), ShouldEqual, arrayContype)
			case *runContainer16:
				rc = true
				So(cn.containerType(), ShouldEqual, run16Contype)
			default:
				panic(fmt.Errorf("Unrecognized container implementation: %T", cn))
			}
		}
		if !bc {
			t.Errorf("no bitmapContainer found, change your test input so we test all three!")
		}
		if !ac {
			t.Errorf("no arrayContainer found, change your test input so we test all three!")
		}
		if !rc {
			t.Errorf("no runContainer16 found, change your test input so we test all three!")
		}

		var buf bytes.Buffer
		_, err := rb.WriteToMsgpack(&buf)
		if err != nil {
			t.Errorf("Failed writing")
		}

		newrb := NewBitmap()
		_, err = newrb.ReadFromMsgpack(&buf)
		if err != nil {
			t.Errorf("Failed reading")
		}
		c1, c2 := rb.GetCardinality(), newrb.GetCardinality()
		So(c2, ShouldEqual, c1)
		So(newrb.Equals(rb), ShouldBeTrue)
		//fmt.Printf("\n Basic3: good: match on card = %v", c1)
	})
}

func TestSerializationRunContainer32Msgpack050(t *testing.T) {

	Convey("runContainer32 writeToMsgpack and readFromMsgpack should save/load data", t, func() {
		seed := int64(42)
		p("seed is %v", seed)
		rand.Seed(seed)

		trials := []trial{
			{n: 10, percentFill: .2, ntrial: 1},
			/*			trial{n: 10, percentFill: .8, ntrial: 10},
						trial{n: 10, percentFill: .50, ntrial: 10},

							trial{n: 10, percentFill: .01, ntrial: 10},
							trial{n: 1000, percentFill: .50, ntrial: 10},
							trial{n: 1000, percentFill: .99, ntrial: 10},
			*/
		}

		tester := func(tr trial) {
			for j := 0; j < tr.ntrial; j++ {
				p("TestSerializationRunContainer32Msgpack050 on check# j=%v", j)

				ma := make(map[int]bool)

				n := tr.n
				a := []uint32{}

				draw := int(float64(n) * tr.percentFill)
				for i := 0; i < draw; i++ {
					r0 := rand.Intn(n)
					a = append(a, uint32(r0))
					ma[r0] = true
				}

				orig := newRunContainer32FromVals(false, a...)

				// serialize
				var buf bytes.Buffer
				_, err := orig.writeToMsgpack(&buf)
				if err != nil {
					panic(err)
				}

				// deserialize
				restored := &runContainer32{}
				_, err = restored.readFromMsgpack(&buf)
				if err != nil {
					panic(err)
				}

				// and compare
				So(restored.equals32(orig), ShouldBeTrue)
				orig.removeKey(1)

				// coverage
				var notEq = newRunContainer32Range(1, 1)
				So(notEq.equals32(orig), ShouldBeFalse)

				bc := newBitmapContainer()
				bc.iadd(1)
				bc.iadd(2)
				rc22 := newRunContainer32FromBitmapContainer(bc)
				So(rc22.cardinality(), ShouldEqual, 2)
			}
			p("done with msgpack serialization of runContainer32 check for trial %#v", tr)
		}

		for i := range trials {
			tester(trials[i])
		}

	})
}
