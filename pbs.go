package pbs

import (
	"bufio"
	"errors"
	"fmt"
	"github.com/golang/protobuf/proto"
	"io"
	"reflect"
	"strconv"
	"strings"
)

const (
	Varint      = 0
	Int64       = 1
	LengthDelim = 2
	StartGroup  = 3
	EndGroup    = 4
	Bit32       = 5
)

type StreamMessage interface {
	proto.Message
	io.Closer
	Errors() chan error
	Closed() <-chan struct{}
}

func splitTypeAndField(b byte) (Type, Field byte) {
	return (b & 0x7), b >> 3
}

func readLengthDelim(r *bufio.Reader) ([]byte, error) {
	l, err := readVarint(r)
	if err != nil {
		return nil, err
	}

	buf := make([]byte, l)
	_, err = io.ReadFull(r, buf)
	if err != nil {
		return nil, err
	}
	return buf, nil
}

func readVarint(r *bufio.Reader) (int, error) {
	var sum int
	for i := uint(0); i < 4; i++ {
		b, err := r.ReadByte()
		if err != nil {
			return 0, err
		}

		cont := b & 128
		val := b & 127

		sum += int(val << (7 * i))
		if cont == 0 {
			break
		}
	}
	return sum, nil
}

func (db *decBuffer) decodeField(tag byte, field byte, data []byte) error {
	finfo := db.props.FieldMapping[field]

	val := reflect.ValueOf(db.val).Elem()

	f := val.Field(finfo.GoField)
	switch {
	case f.Type().Kind() == reflect.Chan:
		// This is a 'repeated' field
		// we just encode this value and send it along

		e := f.Type().Elem()
		if e.Kind() == reflect.Ptr {
			e = e.Elem()
		}

		newVal := reflect.New(e)
		switch pv := newVal.Interface().(type) {
		case proto.Message:
			err := proto.Unmarshal(data, pv)
			if err != nil {
				return err
			}
			f.Send(newVal)
		case *string:
			*pv = string(data)
			f.Send(newVal.Elem())
		default:
			fmt.Println(reflect.TypeOf(pv))
			fmt.Println(data)
			return fmt.Errorf("Unrecognized type in protobuf field decode")
		}
		/*
			case f.Type().Elem().Kind() == reflect.String:
				f.Set(reflect.New(f.Type().Elem()))
				f.Elem().SetString(string(data))
		*/
	default:
		/*
			fmt.Println("UNKNOWN")
			fmt.Println(f)
			return errors.New("TODO: fix this unsupported type")
		*/
		buf := proto.NewBuffer(append([]byte{tag}, data...))
		err := buf.Unmarshal(db.val)
		if err != nil {
			return err
		}
	}
	return nil
}

type decBuffer struct {
	props *Props
	val   StreamMessage
}

func StreamDecode(r io.Reader, sm StreamMessage) error {
	props, err := GetProperties(sm)
	if err != nil {
		return err
	}

	go func() {
		defer sm.Close()
		read := bufio.NewReader(r)

		db := decBuffer{
			props: props,
			val:   sm,
		}
		for {
			b, err := read.ReadByte()
			if err != nil {
				if err == io.EOF {
					return
				}
				sm.Errors() <- err
				return
			}

			typ, f := splitTypeAndField(b)
			switch typ {
			case LengthDelim:
				val, err := readLengthDelim(read)
				if err != nil {
					if err == io.EOF {
						return
					}
					sm.Errors() <- err
					return
				}
				err = db.decodeField(b, f, val)
				if err != nil {
					sm.Errors() <- err
					return
				}
			default:
				fmt.Println("unrecognized type: ", typ)
				return
			}
		}
	}()
	return nil
}

func combineTypeAndField(typ, field byte) byte {
	return (field << 3) | (typ & 0x7)
}

func writeLengthDelimited(w io.Writer, data []byte) error {
	length := proto.EncodeVarint(uint64(len(data)))
	n, err := w.Write(length)
	if err != nil {
		return err
	}
	if n != len(length) {
		return errors.New("failed to write length")
	}

	n, err = w.Write([]byte(data))
	if err != nil {
		return err
	}
	if n != len(data) {
		return errors.New("failed to write data")
	}
	return nil
}

func writeTag(w io.Writer, tag byte) error {
	n, err := w.Write([]byte{tag})
	if err != nil {
		return err
	}
	if n != 1 {
		return errors.New("failed to write tag")
	}
	return nil
}

func StreamEncode(w io.Writer, sm StreamMessage) error {
	props, err := GetProperties(sm)
	if err != nil {
		return err
	}
	_ = props

	val := reflect.ValueOf(sm).Elem()

	for protoField, fprop := range props.FieldMapping {
		field := val.Field(fprop.GoField)
		if !fprop.Repeated {
			switch field.Elem().Kind() {
			case reflect.String:
				tag := combineTypeAndField(LengthDelim, protoField)
				err := writeTag(w, tag)
				if err != nil {
					return err
				}

				str := field.Elem().Interface().(string)
				err = writeLengthDelimited(w, []byte(str))
				if err != nil {
					return err
				}
			default:
				fmt.Println(field.Elem().Kind())
			}
		}
	}

	go func() {
		closeCase := reflect.SelectCase{
			Chan: reflect.ValueOf(sm.Closed()),
			Dir:  reflect.SelectRecv,
		}

		types := []reflect.Type{nil}
		fields := []byte{0}

		cases := []reflect.SelectCase{closeCase}
		for protoField, fprop := range props.FieldMapping {
			if fprop.Repeated {
				field := val.Field(fprop.GoField)
				if field.Kind() != reflect.Chan {
					panic("repeated field was not a channel")
				}

				cases = append(cases, reflect.SelectCase{
					Chan: field,
					Dir:  reflect.SelectRecv,
				})

				fields = append(fields, protoField)
				types = append(types, field.Type().Elem())
			}
		}

		for {
			chosen, val, ok := reflect.Select(cases)
			if !ok {
				fmt.Println("not okay, returning from select")
				return
			}

			if chosen == 0 {
				fmt.Println("got close signal, returning")
				return
			}

			switch val := val.Interface().(type) {
			case proto.Message:
				data, err := proto.Marshal(val)
				if err != nil {
					fmt.Println(err)
					return
				}

				tag := combineTypeAndField(LengthDelim, fields[chosen])
				err = writeTag(w, tag)
				if err != nil {
					fmt.Println(err)
					return
				}

				err = writeLengthDelimited(w, data)
				if err != nil {
					fmt.Println(err)
					return
				}
			case string:
				tag := combineTypeAndField(LengthDelim, fields[chosen])
				err = writeTag(w, tag)
				if err != nil {
					fmt.Println(err)
					return
				}

				err := writeLengthDelimited(w, []byte(val))
				if err != nil {
					fmt.Println(err)
					return
				}
			default:
				fmt.Println(reflect.TypeOf(val))
				fmt.Println("UNRECOGNIZED REPEATED FIELD TYPE")
			}

			_ = val
		}
	}()

	return nil
}

type FieldInfo struct {
	// The field index in the Go struct
	GoField  int
	Repeated bool
	Type     string
}

type Props struct {
	// A mapping from the protobuf field number to field info
	FieldMapping map[byte]FieldInfo
}

func GetProperties(i proto.Message) (*Props, error) {
	t := reflect.TypeOf(i).Elem()

	props := &Props{FieldMapping: make(map[byte]FieldInfo)}
	for i := 0; i < t.NumField(); i++ {
		field := FieldInfo{GoField: i}

		tag := t.Field(i).Tag.Get("protobuf")
		if len(tag) == 0 {
			continue
		}

		parts := strings.Split(tag, ",")
		if len(parts) < 3 {
			return nil, errors.New("not enough values in protobuf field tag")
		}

		n, err := strconv.Atoi(parts[1])
		if err != nil {
			return nil, err
		}

		if parts[2] == "rep" {
			field.Repeated = true
		}

		field.Type = parts[0]
		props.FieldMapping[byte(n)] = field
	}
	return props, nil
}
