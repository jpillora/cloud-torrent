package eventsource

import (
	"bytes"
	"io"
	"io/ioutil"
	"strconv"
	"testing"
)

var benchmarkData []byte
var benchmarkEvents []Event

func initBenchmarkData() {
	var buf bytes.Buffer
	e := NewEncoder(&buf)

	for i := int64(0); i < 1000; i++ {
		event := Event{
			Data: strconv.AppendInt(nil, i, 10),
		}

		benchmarkEvents = append(benchmarkEvents, event)
		e.Encode(event)
	}

	benchmarkData = buf.Bytes()
}

func BenchmarkDecoder(b *testing.B) {
	if benchmarkData == nil {
		b.StopTimer()
		initBenchmarkData()
		b.StartTimer()
	}
	var buf bytes.Buffer
	dec := NewDecoder(&buf)
	for i := 0; i < b.N; i++ {
		buf.Write(benchmarkData)

		var err error
		for err != io.EOF {
			var event Event
			err = dec.Decode(&event)

			if err != nil && err != io.EOF {
				b.Fatal("Decode:", err)
			}
		}
	}
	b.SetBytes(int64(len(benchmarkData)))
}

func BenchmarkEncoder(b *testing.B) {
	if benchmarkData == nil {
		b.StopTimer()
		initBenchmarkData()
		b.StartTimer()
	}
	enc := NewEncoder(ioutil.Discard)
	for i := 0; i < b.N; i++ {
		for _, e := range benchmarkEvents {
			if err := enc.Encode(e); err != nil {
				b.Fatal("Encode:", err)
			}
		}
	}
	b.SetBytes(int64(len(benchmarkData)))
}
