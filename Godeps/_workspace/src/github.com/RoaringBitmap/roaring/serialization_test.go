package roaring

// to run just these tests: go test -run TestSerialization*

import (
	"bytes"
	"os"
	"testing"
)

func TestBase64(t *testing.T) {
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

func TestSerializationBasic(t *testing.T) {
	rb := BitmapOf(1, 2, 3, 4, 5, 100, 1000)
	if BoundSerializedSizeInBytes(rb.GetCardinality(), 1001) < rb.GetSerializedSizeInBytes() {
		t.Errorf("Bad BoundSerializedSizeInBytes")
	}
	l := int(rb.GetSerializedSizeInBytes())
	buf := new(bytes.Buffer)
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

func TestSerializationToFile(t *testing.T) {
	rb := BitmapOf(1, 2, 3, 4, 5, 100, 1000)
	fname := "myfile.bin"
	fout, err := os.OpenFile(fname, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0660)
	if err != nil {
		t.Errorf("Can't open a file for writing")
	}
	defer fout.Close()
	_, err = rb.WriteTo(fout)
	if err != nil {
		t.Errorf("Failed writing")
	}
	newrb := NewBitmap()
	fin, err := os.Open(fname)

	if err != nil {
		t.Errorf("Failed reading")
	}
	defer func() {
		fin.Close()
		err := os.Remove(fname)
		if err != nil {
			t.Errorf("could not delete ", fname)
		}
	}()
	_, err = newrb.ReadFrom(fin)
	if !rb.Equals(newrb) {
		t.Errorf("Cannot retrieve serialized version")
	}
}

func TestSerializationFromJava(t *testing.T) {
	fname := "testdata/bitmapwithoutruns.bin"
	newrb := NewBitmap()
	fin, err := os.Open(fname)

	if err != nil {
		t.Errorf("Failed reading")
	}
	defer func() {
		fin.Close()
	}()
	_, err = newrb.ReadFrom(fin)
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

func TestSerializationBasic2(t *testing.T) {
	rb := BitmapOf(1, 2, 3, 4, 5, 100, 1000, 10000, 100000, 1000000)
	buf := new(bytes.Buffer)
	if BoundSerializedSizeInBytes(rb.GetCardinality(), 1000001) < rb.GetSerializedSizeInBytes() {
		t.Errorf("Bad BoundSerializedSizeInBytes")
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

func TestSerializationBasic3(t *testing.T) {
	rb := BitmapOf(1, 2, 3, 4, 5, 100, 1000, 10000, 100000, 1000000)
	for i := 5000000; i < 5000000+2*(1<<16); i++ {
		rb.AddInt(i)
	}
	if BoundSerializedSizeInBytes(rb.GetCardinality(), 5000000+2*(1<<16)+1) < rb.GetSerializedSizeInBytes() {
		t.Errorf("Bad BoundSerializedSizeInBytes")
	}

	l := int(rb.GetSerializedSizeInBytes())
	buf := new(bytes.Buffer)
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
