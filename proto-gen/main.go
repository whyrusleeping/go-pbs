package main

import (
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"
)

type Field struct {
	Name      string
	Number    int
	Type      string
	Attribute string
}

type Message struct {
	Name        string
	Fields      []*Field
	SubMessages []*Message
}

type Protobuf struct {
	Package  string
	Messages []*Message
}

func (f *Field) ParseField(r *TokenReader) error {
	typ, err := r.NextToken()
	if err != nil {
		return err
	}

	f.Type = typ

	name, err := r.NextToken()
	if err != nil {
		return err
	}

	f.Name = name

	eq, err := r.NextToken()
	if err != nil {
		return err
	}

	if eq != "=" {
		return errors.New("expected equals sign after field name")
	}

	fieldnum, err := r.NextToken()
	if err != nil {
		return err
	}

	fnum, err := strconv.Atoi(fieldnum)
	if err != nil {
		return err
	}

	f.Number = fnum

	semi, err := r.NextToken()
	if err != nil {
		return err
	}

	if semi != ";" {
		return errors.New("expected a semicolon after field number")
	}
	return nil
}

func ParseMessage(r *TokenReader) (*Message, error) {
	m := new(Message)
	mesname, err := r.NextToken()
	if err != nil {
		return nil, err
	}

	openBracket, err := r.NextToken()
	if err != nil {
		return nil, err
	}

	if openBracket != "{" {
		return nil, errors.New("expected opening bracket after message name")
	}

	m.Name = mesname

	for {
		tok, err := r.NextToken()
		if err != nil {
			return nil, err
		}

		switch tok {
		case "}":
			return m, nil
		case "repeated", "required":
			// its a field!
			f := &Field{Attribute: tok}
			err := f.ParseField(r)
			if err != nil {
				return nil, err
			}

			m.Fields = append(m.Fields, f)

		case "message":
			// its a submessage!
			subm, err := ParseMessage(r)
			if err != nil {
				return nil, err
			}

			m.SubMessages = append(m.SubMessages, subm)
		default:
			fmt.Println("Unrecognized token: ", tok)
		}
	}
}

func ParseProtoFile(r io.Reader) (*Protobuf, error) {
	pb := new(Protobuf)
	read := NewTokenReader(r)
	for {
		tok, err := read.NextToken()
		if err != nil {
			if err == io.EOF {
				return pb, nil
			}
			return nil, err
		}

		switch tok {
		case "package":
			pkgname, err := read.NextToken()
			if err != nil {
				return nil, err
			}
			pb.Package = pkgname
			semi, err := read.NextToken()
			if err != nil {
				return nil, err
			}

			if semi != ";" {
				return nil, errors.New("expected semicolon after package name")
			}
		case "message":
			message, err := ParseMessage(read)
			if err != nil {
				return nil, err
			}

			pb.Messages = append(pb.Messages, message)

		default:
			fmt.Println("Unrecognized token: ", tok)
		}
	}
	return nil, nil
}

func main() {
	if len(os.Args) < 2 {
		fmt.Println("Please specify a protobuf file to compile")
		return
	}
	arg := os.Args[1]
	fi, err := os.Open(arg)
	if err != nil {
		fmt.Println(err)
		return
	}
	proto, err := ParseProtoFile(fi)
	if err != nil {
		fmt.Println(err)
		return
	}
	PrintGoStreamProto(os.Stdout, proto)
}
