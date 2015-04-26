package main

import (
	"bufio"
	"bytes"
	"io"
)

type TokenReader struct {
	r      *bufio.Reader
	buffer *bytes.Buffer
	next   string
}

func NewTokenReader(r io.Reader) *TokenReader {
	return &TokenReader{
		r:      bufio.NewReader(r),
		buffer: new(bytes.Buffer),
	}
}

func (tr *TokenReader) NextToken() (string, error) {
	for {
		if tr.next != "" {
			out := tr.next
			tr.next = ""
			return out, nil
		}
		b, err := tr.r.ReadByte()
		if err != nil {
			if err == io.EOF && tr.buffer.Len() > 0 {
				return tr.buffer.String(), nil
			}
			return "", err
		}

		if b == ' ' || b == ';' || b == '\n' || b == '\t' || b == '=' {
			if b == ';' || b == '=' {
				tr.next = string(b)
			}
			if tr.buffer.Len() > 0 {
				out := tr.buffer.String()
				tr.buffer.Reset()
				return out, nil
			}
			continue
		}

		tr.buffer.WriteByte(b)
	}
	return "", nil
}
