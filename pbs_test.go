package pbs_test

import (
	"bytes"
	"sync"
	"testing"

	"github.com/golang/protobuf/proto"
	. "github.com/whyrusleeping/go-pbs"
	tpb "github.com/whyrusleeping/go-pbs/testproto"
)

var _ = Varint

func generateTestMessage() *TestMessage {
	tm := NewTestMessage()
	tm.A = proto.Int32(-195)
	tm.B = proto.String("pbs is fun")
	tm.C = proto.Int64(1 << 37)
	tm.D = proto.Bool(true)
	tm.E = []byte("pbs is still fun")

	return tm
}

func TestStreamEncode(t *testing.T) {
	buf := new(bytes.Buffer)
	tm := generateTestMessage()
	err := StreamEncode(buf, tm)
	if err != nil {
		t.Fatal(err)
	}

	repbytes := [][]byte{
		[]byte("hello world"),
		[]byte("goodbye sun"),
	}

	repints := []int32{4, 1, 9, 5}

	repstrings := []string{"cat", "dog", "fish", "cow"}

	for _, b := range repbytes {
		tm.Repbytes <- b
	}

	for _, i := range repints {
		tm.Repint <- i
	}

	for _, s := range repstrings {
		tm.Repstring <- s
	}

	tm.Close()

	select {
	case err, ok := <-tm.errors:
		if ok {
			t.Fatal(err)
		}
	default:
	}

	outm := new(tpb.TestMessage)
	err = proto.Unmarshal(buf.Bytes(), outm)
	if err != nil {
		t.Fatal(err)
	}

	if *tm.A != *outm.A {
		t.Fatal("A value incorrect")
	}

	if *tm.B != *outm.B {
		t.Fatal("B value incorrect")
	}

	if *tm.C != *outm.C {
		t.Fatal("C value incorrect")
	}

	if *tm.D != *outm.D {
		t.Fatal("D value incorrect")
	}

	if !bytes.Equal(tm.E, outm.E) {
		t.Fatal("E value incorrect")
	}

	if len(outm.Repbytes) != len(repbytes) {
		t.Fatal("got different number of repeated bytes")
	}
	for i, v := range repbytes {
		if !bytes.Equal(v, outm.Repbytes[i]) {
			t.Fatal("value mismatch for repbytes")
		}
	}

	if len(outm.Repint) != len(repints) {
		t.Fatal("got different number of repeated ints")
	}
	for i, v := range repints {
		if v != outm.Repint[i] {
			t.Fatal("value mismatch for repints")
		}
	}

	if len(outm.Repstring) != len(repstrings) {
		t.Fatal("got different number of repeated strings", len(outm.Repstring), len(repstrings))
	}
	for i, v := range repstrings {
		if v != outm.Repstring[i] {
			t.Fatal("value mismatch for repstrings")
		}
	}
}

func TestDecode(t *testing.T) {
	tm := NewTestMessage()
	buf := new(bytes.Buffer)
	t.Skip("skipping")
	outmes := NewTestMessage()
	err := StreamDecode(buf, outmes)
	if err != nil {
		t.Fatal(err)
	}

	var outbytes [][]byte
	var outints []int32
	var outstrings []string

	wg := sync.WaitGroup{}
	wg.Add(3)
	go func() {
		for b := range outmes.Repbytes {
			outbytes = append(outbytes, b)
		}
		wg.Done()
	}()

	go func() {
		for i := range outmes.Repint {
			outints = append(outints, i)
		}
		wg.Done()
	}()

	go func() {
		for s := range outmes.Repstring {
			outstrings = append(outstrings, s)
		}
		wg.Done()
	}()

	wg.Wait()

	select {
	case err, ok := <-outmes.Errors():
		if ok {
			t.Fatal(err)
		}
	default:
	}

	if *tm.A != *outmes.A {
		t.Fatal("A value incorrect")
	}

	if *tm.B != *outmes.B {
		t.Fatal("B value incorrect")
	}
}
