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

func (db *decBuffer) decodeField(field byte, data []byte) error {
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
		case *[]byte:
			*pv = data
			f.Send(newVal.Elem())
		default:
			fmt.Println(reflect.TypeOf(pv))
			fmt.Println(data)
			return fmt.Errorf("Unrecognized type in protobuf field decode")
		}
	case f.Type().Elem().Kind() == reflect.String:
		f.Set(reflect.New(f.Type().Elem()))
		f.Elem().SetString(string(data))
	default:
		fmt.Println("UNKNOWN")
		fmt.Println(f)
		return errors.New("TODO: fix this unsupported type")
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
			case Varint:
				i, err := readVarint(read)
				if err != nil {
					sm.Errors() <- err
					return
				}

				field := reflect.ValueOf(sm).Elem().Field(props.FieldMapping[f].GoField)

				fmt.Println("proto field: ", f)
				fmt.Println("FIELD: ", field)
				elemType := field.Type()
				if elemType.Kind() == reflect.Chan {
					elemType = elemType.Elem()
				}
				fmt.Println("elemtype: ", elemType)

				nval := reflect.New(elemType.Elem())
				nval.Elem().SetInt(int64(i))

				if props.FieldMapping[f].Repeated {
					field.Send(nval.Elem())
				} else {
					field.Set(nval)
				}
			case LengthDelim:
				val, err := readLengthDelim(read)
				if err != nil {
					if err == io.EOF {
						return
					}
					sm.Errors() <- err
					return
				}
				err = db.decodeField(f, val)
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

func writeVarint(w io.Writer, v uint64) error {
	data := proto.EncodeVarint(v)
	n, err := w.Write(data)
	if err != nil {
		return err
	}
	if n != len(data) {
		return errors.New("failed to write enough bytes")
	}
	return nil
}

func StreamEncode(w io.Writer, sm StreamMessage) error {
	props, err := GetProperties(sm)
	if err != nil {
		return err
	}

	val := reflect.ValueOf(sm).Elem()

	for protoField, fprop := range props.FieldMapping {
		field := val.Field(fprop.GoField)
		val := field
		if val.Kind() == reflect.Ptr {
			val = val.Elem()
		}
		if !fprop.Repeated {
			switch val.Kind() {
			case reflect.String:
				tag := combineTypeAndField(LengthDelim, protoField)
				err := writeTag(w, tag)
				if err != nil {
					return err
				}

				str := val.Interface().(string)
				err = writeLengthDelimited(w, []byte(str))
				if err != nil {
					return err
				}
			case reflect.Int32:
				tag := combineTypeAndField(Varint, protoField)
				err := writeTag(w, tag)
				if err != nil {
					return err
				}

				err = writeVarint(w, uint64(val.Interface().(int32)))
				if err != nil {
					return err
				}
			case reflect.Int64:
				tag := combineTypeAndField(Varint, protoField)
				err := writeTag(w, tag)
				if err != nil {
					return err
				}

				err = writeVarint(w, uint64(val.Interface().(int64)))
				if err != nil {
					return err
				}
			case reflect.Bool:
				b := val.Interface().(bool)
				var boolval uint64
				if b {
					boolval = 1
				}

				tag := combineTypeAndField(Varint, protoField)
				err := writeTag(w, tag)
				if err != nil {
					return err
				}

				err = writeVarint(w, boolval)
				if err != nil {
					return err
				}
			case reflect.Slice:
				if val.Type().Elem().Kind() != reflect.Uint8 {
					return fmt.Errorf("cannot handle arrays of non byte type")
				}
				tag := combineTypeAndField(LengthDelim, protoField)
				err := writeTag(w, tag)
				if err != nil {
					return err
				}

				byteval := val.Interface().([]byte)
				err = writeLengthDelimited(w, byteval)
				if err != nil {
					return err
				}

			default:
				return fmt.Errorf("unhandled type: %s", val)
			}
		}
	}

	for protoField, fprop := range props.FieldMapping {
		if fprop.Repeated {
			field := val.Field(fprop.GoField)
			if field.Kind() != reflect.Chan {
				return errors.New("repeated field was not a channel")
			}

			go handleChannelIn(w, sm, protoField, field.Type().Elem(), field)
		}
	}

	return nil
}

func handleChannelIn(w io.Writer, sm StreamMessage, field byte, typ reflect.Type, ch reflect.Value) {
	for {
		val, ok := ch.Recv()
		if !ok {
			return
		}

		switch val := val.Interface().(type) {
		case proto.Message:
			data, err := proto.Marshal(val)
			if err != nil {
				sm.Errors() <- err
				return
			}

			tag := combineTypeAndField(LengthDelim, field)
			err = writeTag(w, tag)
			if err != nil {
				sm.Errors() <- err
				return
			}

			err = writeLengthDelimited(w, data)
			if err != nil {
				sm.Errors() <- err
				return
			}
		case string:
			tag := combineTypeAndField(LengthDelim, field)
			err := writeTag(w, tag)
			if err != nil {
				sm.Errors() <- err
				return
			}

			err = writeLengthDelimited(w, []byte(val))
			if err != nil {
				sm.Errors() <- err
				return
			}
		case []byte:
			tag := combineTypeAndField(LengthDelim, field)
			err := writeTag(w, tag)
			if err != nil {
				sm.Errors() <- err
				return
			}

			err = writeLengthDelimited(w, val)
			if err != nil {
				sm.Errors() <- err
				return
			}
		case int32:
			tag := combineTypeAndField(Varint, field)
			err := writeTag(w, tag)
			if err != nil {
				sm.Errors() <- err
				return
			}

			err = writeVarint(w, uint64(val))
			if err != nil {
				sm.Errors() <- err
				return
			}
		default:
			fmt.Println("UNRECOGNIZED REPEATED FIELD TYPE", reflect.TypeOf(val))
			sm.Errors() <- errors.New("unrecognized repeated field type")
			return
		}

		_ = val
	}
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
