package main

import (
	"fmt"
	"io"
	"strings"
)

func PrintGoStreamProto(w io.Writer, pb *Protobuf) {
	fmt.Printf("package %s\n\n", pb.Package)
	printImports(w)
	for _, mes := range pb.Messages {
		printGoProtoMessage(w, mes, "", true)
	}
}

var imports = []string{
	"github.com/golang/protobuf/proto",
	"github.com/whyrusleeping/go-pbs",
}

func printImports(w io.Writer) {
	for _, i := range imports {
		fmt.Fprintf(w, "import \"%s\"\n", i)
	}
	fmt.Fprintln(w)
}

func makeGoName(s string) string {
	return strings.ToUpper(s[:1]) + s[1:]
}

func parseGoType(typ string, prefix string) string {
	switch typ {
	case "string":
		return "string"
	case "bytes":
		return "[]byte"
	default:
		return "*" + prefix + makeGoName(typ)
	}
}

func formatGoProtoField(f *Field, prefix string, stream bool) string {
	var typ string
	if f.Attribute == "repeated" {
		if stream {
			typ = "chan "
		} else {
			typ = "[]"
		}
	}
	typ += parseGoType(f.Type, prefix)
	name := makeGoName(f.Name)

	tag := fmt.Sprintf("`protobuf:\"%s,%d,%s,name=%s\"`", f.Type, f.Number, f.Attribute[:3], f.Name)
	return fmt.Sprintf("%s %s %s", name, typ, tag)
}

func printGoProtoMessage(w io.Writer, mes *Message, prefix string, stream bool) {
	name := prefix + mes.Name
	fmt.Fprintf(w, "type %s struct {\n", name)
	for _, f := range mes.Fields {
		fmt.Fprintln(w, "\t"+formatGoProtoField(f, name+"_", stream))
	}
	if stream {
		fmt.Fprintln(w, "\terrors chan error")
		fmt.Fprintln(w, "\tcloseCh chan struct{}")
	}
	fmt.Fprintln(w, "}\n")

	printMessageConstructor(w, mes, name, stream)

	if stream {
		printGoStreamMethods(w, mes, name)
	}
	printProtoMethods(w, name)
	printInterfaceAssertion(w, mes, name, stream)

	for _, subm := range mes.SubMessages {
		printGoProtoMessage(w, subm, name+"_", false)
	}
}

func printInterfaceAssertion(w io.Writer, mes *Message, name string, stream bool) {
	var typ string
	if stream {
		typ = "pbs.StreamMessage"
	} else {
		typ = "proto.Message"
	}
	fmt.Fprintf(w, "var _ %s = (*%s)(nil)\n", typ, name)
}

func printGoStreamMethods(w io.Writer, mes *Message, name string) {
	fmt.Fprintf(w, "func (m *%s) Errors() chan error { return m.errors }\n\n", name)
	fmt.Fprintf(w, "func (m *%s) Closed() <-chan struct{} { return m.closeCh }\n\n", name)
	fmt.Fprintf(w, "func (m *%s) Close() error {\n", name)
	for _, f := range mes.Fields {
		fmt.Fprintf(w, "\tclose(m.%s)\n", makeGoName(f.Name))
	}
	fmt.Fprintf(w, "\tclose(m.errors)\n")
	fmt.Fprintf(w, "\tclose(m.closeCh)\n")
	fmt.Fprintln(w, "\treturn nil")
	fmt.Fprintln(w, "}\n")
}

func printMessageConstructor(w io.Writer, mes *Message, name string, stream bool) {
	fmt.Fprintf(w, "func New%s() *%s {\n", name, name)
	fmt.Fprintf(w, "\treturn &%s{\n", name)
	if stream {
		fmt.Fprintf(w, "\t\terrors: make(chan error, 1),\n")
		fmt.Fprintf(w, "\t\tcloseCh: make(chan struct{}),\n")
		for _, f := range mes.Fields {
			if f.Attribute == "repeated" {
				fmt.Fprintf(w, "\t\t%s: make(chan %s),\n", makeGoName(f.Name), parseGoType(f.Type, name+"_"))
			}
		}
	}
	fmt.Fprintf(w, "\t}\n}\n")
}

func printProtoMethods(w io.Writer, name string) {
	fmt.Fprintf(w, "func (*%s) ProtoMessage() {}\n\n", name)
	fmt.Fprintf(w, "func (m *%s) String() string {return proto.CompactTextString(m)}\n\n", name)
	fmt.Fprintf(w, "func (m *%s) Reset() {*m = *New%s()}\n\n", name, name)
}
