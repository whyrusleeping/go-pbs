package main

import (
	"fmt"
	"io"
	"strings"
)

// PrintGoStreamProto writes out go source code for the given protobuf
// Top level messages in the protobuf will be implemented as StreamMessages
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

// printImports writes out the default imports
func printImports(w io.Writer) {
	for _, i := range imports {
		fmt.Fprintf(w, "import \"%s\"\n", i)
	}
	fmt.Fprintln(w)
}

// makeGoName converts a protobuf name to an acceptable go name
// this currently consists of simply capitalizing the first letter
func makeGoName(s string) string {
	return strings.ToUpper(s[:1]) + s[1:]
}

var typeMap = map[string]string{
	"string": "string",
	"bytes":  "[]byte",
	"int32":  "int32",
	"uint32": "uint32",
	"int64":  "int64",
	"uint64": "uint64",
	"bool":   "bool",
}

// parseGoType returns the go representation of the passed in protobuf type
func parseGoType(typ string, prefix string, rep bool) string {
	t, ok := typeMap[typ]
	if ok {
		if rep || t[:2] == "[]" {
			return t
		} else {
			return "*" + t
		}
	}

	return "*" + prefix + makeGoName(typ)
}

// formatGoProtoField returns a string representing the golang member variable
// representation of the passed in field
func formatGoProtoField(f *Field, prefix string, stream bool) string {
	var typ string
	if f.Attribute == "repeated" {
		if stream {
			typ = "chan "
		} else {
			typ = "[]"
		}
	}
	typ += parseGoType(f.Type, prefix, f.Attribute == "repeated")
	name := makeGoName(f.Name)

	tag := fmt.Sprintf("`protobuf:\"%s,%d,%s,name=%s\"`", f.Type, f.Number, f.Attribute[:3], f.Name)
	return fmt.Sprintf("%s %s %s", name, typ, tag)
}

// printGoProtoMessage generates go code for the given message and writes it out
// to the given writer
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

// printInterfaceAssertion prints a line that will do a compile time type assertion
// on the given message type to make sure it matches either the pbs.StreamMessage
// interface or the proto.Message
func printInterfaceAssertion(w io.Writer, mes *Message, name string, stream bool) {
	var typ string
	if stream {
		typ = "pbs.StreamMessage"
	} else {
		typ = "proto.Message"
	}
	fmt.Fprintf(w, "var _ %s = (*%s)(nil)\n\n", typ, name)
}

// printGoStreamMethods writes out methods that implement the pbs.StreamMessage
// interface for the given message
func printGoStreamMethods(w io.Writer, mes *Message, name string) {
	fmt.Fprintf(w, "func (m *%s) Errors() chan error { return m.errors }\n\n", name)
	fmt.Fprintf(w, "func (m *%s) Closed() <-chan struct{} { return m.closeCh }\n\n", name)
	fmt.Fprintf(w, "func (m *%s) Close() error {\n", name)
	for _, f := range mes.Fields {
		if f.Attribute == "repeated" {
			fmt.Fprintf(w, "\tclose(m.%s)\n", makeGoName(f.Name))
		}
	}
	fmt.Fprintf(w, "\tclose(m.errors)\n")
	fmt.Fprintf(w, "\tclose(m.closeCh)\n")
	fmt.Fprintln(w, "\treturn nil")
	fmt.Fprintln(w, "}\n")
}

// printMessageConstructor writes a constructor function for the given message type
func printMessageConstructor(w io.Writer, mes *Message, name string, stream bool) {
	fmt.Fprintf(w, "func New%s() *%s {\n", name, name)
	fmt.Fprintf(w, "\treturn &%s{\n", name)
	if stream {
		fmt.Fprintf(w, "\t\terrors: make(chan error, 1),\n")
		fmt.Fprintf(w, "\t\tcloseCh: make(chan struct{}),\n")
		for _, f := range mes.Fields {
			if f.Attribute == "repeated" {
				fmt.Fprintf(w, "\t\t%s: make(chan %s),\n", makeGoName(f.Name), parseGoType(f.Type, name+"_", true))
			}
		}
	}
	fmt.Fprintf(w, "\t}\n}\n")
}

// printProtoMethods writes out methods for the given message type to implement
// the proto.Message interface
func printProtoMethods(w io.Writer, name string) {
	fmt.Fprintf(w, "func (*%s) ProtoMessage() {}\n\n", name)
	fmt.Fprintf(w, "func (m *%s) String() string {return proto.CompactTextString(m)}\n\n", name)
	fmt.Fprintf(w, "func (m *%s) Reset() {*m = *New%s()}\n\n", name, name)
}
