package main

import (
	"fmt"
	"io"
)

func PrintProtobuf(w io.Writer, pb *Protobuf) {
	fmt.Fprintf(w, "package %s;\n\n", pb.Package)
	for _, mes := range pb.Messages {
		PrintMessage(w, mes, 0)
	}
}

func writeIndent(w io.Writer, indent string, count int) {
	for i := 0; i < count; i++ {
		fmt.Fprint(w, indent)
	}
}

func PrintMessage(w io.Writer, mes *Message, indent int) {
	writeIndent(w, "  ", indent)
	fmt.Fprintf(w, "message %s {\n", mes.Name)
	for _, f := range mes.Fields {
		writeIndent(w, "  ", indent+1)
		fmt.Fprintf(w, "%s %s %s = %d;\n", f.Attribute, f.Type, f.Name, f.Number)
	}
	fmt.Fprintln(w)
	for _, subm := range mes.SubMessages {
		PrintMessage(w, subm, indent+1)
	}

	writeIndent(w, "  ", indent)
	fmt.Fprintln(w, "}")
}
