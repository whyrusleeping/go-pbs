package main

import (
	"fmt"
	"github.com/gogo/protobuf/proto"
	pbs "github.com/whyrusleeping/go-pbs"
	"io"
)

type StreamCompany struct {
	Name      *string                `protobuf:"bytes,1,req,name=name"`
	Employees chan *Company_Employee `protobuf:"bytes,2,rep,name=employees"`
	Country   *string                `protobuf:"bytes,3,opt,name=country"`

	errors  chan error
	closeCh chan struct{}
}

func NewStreamCompany() *StreamCompany {
	return &StreamCompany{
		Employees: make(chan *Company_Employee),
		errors:    make(chan error, 1),
		closeCh:   make(chan struct{}),
	}
}

func (sc *StreamCompany) ProtoMessage()  {}
func (sc *StreamCompany) String() string { return "TODO" }
func (sc *StreamCompany) Reset() {
	sc.Name = nil
	sc.Country = nil
	sc.Employees = make(chan *Company_Employee)
}

func (sc *StreamCompany) Close() error {
	close(sc.Employees)
	close(sc.closeCh)
	close(sc.errors)
	return nil
}

func (sc *StreamCompany) Closed() <-chan struct{} {
	return sc.closeCh
}

func (sc *StreamCompany) Errors() chan error {
	return sc.errors
}

func makeEmployee(name string, age uint32) *Company_Employee {
	return &Company_Employee{
		Name: &name,
		Age:  &age,
	}
}

func testDataPB() *StreamCompany {
	sc := new(StreamCompany)
	sc.Country = proto.String("AMERICA")
	sc.Name = proto.String("Corp Inc")
	sc.closeCh = make(chan struct{})
	sc.errors = make(chan error, 1)
	sc.Employees = make(chan *Company_Employee)

	go func() {
		for _, e := range []*Company_Employee{
			makeEmployee("steve", 56),
			makeEmployee("sarah", 17),
			makeEmployee("amanda", 19),
			makeEmployee("hank", 32),
			makeEmployee("carol", 40),
			makeEmployee("steven", 9),
		} {
			sc.Employees <- e
		}
		sc.Close()
	}()
	return sc
}

func main() {
	r, w := io.Pipe()

	// Go stream encode some protos
	go func() {
		sc := testDataPB()
		err := pbs.StreamEncode(w, sc)
		if err != nil {
			panic(err)
		}
		<-sc.Closed()
		w.Close()
	}()

	sc := NewStreamCompany()
	err := pbs.StreamDecode(r, sc)
	if err != nil {
		panic(err)
	}

	for e := range sc.Employees {
		fmt.Println(e)
	}
}
