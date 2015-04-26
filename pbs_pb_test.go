package pbs_test

import "github.com/golang/protobuf/proto"
import "github.com/whyrusleeping/go-pbs"

type TestMessage struct {
	Tsubm chan *TestMessage_TestSubMessage `protobuf:"TestSubMessage,1,rep,name=tsubm"`
	Repint chan int32 `protobuf:"int32,2,rep,name=repint"`
	Repbytes chan []byte `protobuf:"bytes,8,rep,name=repbytes"`
	Repstring chan string `protobuf:"string,9,rep,name=repstring"`
	A *int32 `protobuf:"int32,3,opt,name=a"`
	B *string `protobuf:"string,4,opt,name=b"`
	C *int64 `protobuf:"int64,5,req,name=c"`
	D *bool `protobuf:"bool,6,opt,name=d"`
	E []byte `protobuf:"bytes,7,opt,name=e"`
	errors chan error
	closeCh chan struct{}
}

func NewTestMessage() *TestMessage {
	return &TestMessage{
		errors: make(chan error, 1),
		closeCh: make(chan struct{}),
		Tsubm: make(chan *TestMessage_TestSubMessage),
		Repint: make(chan int32),
		Repbytes: make(chan []byte),
		Repstring: make(chan string),
	}
}
func (m *TestMessage) Errors() chan error { return m.errors }

func (m *TestMessage) Closed() <-chan struct{} { return m.closeCh }

func (m *TestMessage) Close() error {
	close(m.Tsubm)
	close(m.Repint)
	close(m.Repbytes)
	close(m.Repstring)
	close(m.errors)
	close(m.closeCh)
	return nil
}

func (*TestMessage) ProtoMessage() {}

func (m *TestMessage) String() string {return proto.CompactTextString(m)}

func (m *TestMessage) Reset() {*m = *NewTestMessage()}

var _ pbs.StreamMessage = (*TestMessage)(nil)

type TestMessage_TestSubMessage struct {
	X *string `protobuf:"string,1,opt,name=x"`
	Y []uint32 `protobuf:"uint32,2,rep,name=y"`
}

func NewTestMessage_TestSubMessage() *TestMessage_TestSubMessage {
	return &TestMessage_TestSubMessage{
	}
}
func (*TestMessage_TestSubMessage) ProtoMessage() {}

func (m *TestMessage_TestSubMessage) String() string {return proto.CompactTextString(m)}

func (m *TestMessage_TestSubMessage) Reset() {*m = *NewTestMessage_TestSubMessage()}

var _ proto.Message = (*TestMessage_TestSubMessage)(nil)

