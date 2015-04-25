package main

import (
	"fmt"
	pbs "github.com/whyrusleeping/go-pbs"
	"io"
	"math/rand"
	"time"
)

var chatters = []string{"jbenet", "whyrusleeping", "mafintosh", "tperson", "odg", "substack"}

var messages = []string{"Hey guys", "ipfs is cool", "protobufs are 1337", "i like cats"}

func NewChatProtocol() *ChatProtocol {
	cproto := new(ChatProtocol)
	cproto.Messages = make(chan *ChatProtocol_Message)
	cproto.Online = make(chan string)
	cproto.closeCh = make(chan struct{})
	cproto.errors = make(chan error)
	return cproto
}
func ChatConsumer(r io.Reader) {
	cproto := NewChatProtocol()

	err := pbs.StreamDecode(r, cproto)
	if err != nil {
		panic(err)
	}

	for {
		select {
		case mes, ok := <-cproto.Messages:
			if !ok {
				return
			}
			fmt.Printf("%s:  %s\n", mes.GetFrom(), mes.GetText())
		case join, ok := <-cproto.Online:
			if !ok {
				return
			}
			fmt.Printf("--> %s joined chat\n", join)
		case err, ok := <-cproto.Errors():
			if !ok {
				return
			}

			fmt.Println("ERROR: ", err)
		}
	}
}

func ChatProducer(w io.WriteCloser) {
	cproto := NewChatProtocol()

	err := pbs.StreamEncode(w, cproto)
	if err != nil {
		panic(err)
	}

	for {
		select {
		case <-time.After(time.Second * time.Duration(rand.Intn(6)+1)):
			who := chatters[rand.Intn(len(chatters))]
			what := messages[rand.Intn(len(messages))]
			mes := new(ChatProtocol_Message)
			mes.From = &who
			mes.Text = &what
			cproto.Messages <- mes

		case <-time.After(time.Second * time.Duration(rand.Intn(6)+4)):
			who := chatters[rand.Intn(len(chatters))]
			cproto.Online <- who
		case err := <-cproto.Errors():
			fmt.Println("PRODUCER ERROR: ", err)
		}
	}
}

func main() {
	r, w := io.Pipe()
	go ChatProducer(w)

	ChatConsumer(r)
}
