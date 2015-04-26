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
	"sync"
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

func writeLengthDelimited(w io.Writer, field byte, data []byte) error {
	tag := combineTypeAndField(LengthDelim, field)
	err := writeTag(w, tag)
	if err != nil {
		return err
	}

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

func writeVarint(w io.Writer, field byte, v uint64) error {
	tag := combineTypeAndField(Varint, field)
	err := writeTag(w, tag)
	if err != nil {
		return err
	}

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

func writeProtoVal(w io.Writer, sm StreamMessage, field byte, val interface{}) error {
	switch val := val.(type) {
	case proto.Message:
		data, err := proto.Marshal(val)
		if err != nil {
			return err
		}

		err = writeLengthDelimited(w, field, data)
		if err != nil {
			return err
		}
	case string:
		err := writeLengthDelimited(w, field, []byte(val))
		if err != nil {
			return err
		}
	case []byte:
		err := writeLengthDelimited(w, field, val)
		if err != nil {
			return err
		}
	case int32:
		err := writeVarint(w, field, uint64(val))
		if err != nil {
			return err
		}
	case int64:
		err := writeVarint(w, field, uint64(val))
		if err != nil {
			return err
		}
	case bool:
		var boolval uint64
		if val {
			boolval = 1
		}

		err := writeVarint(w, field, boolval)
		if err != nil {
			return err
		}
	default:
		fmt.Println("UNRECOGNIZED REPEATED FIELD TYPE", reflect.TypeOf(val))
		return errors.New("unrecognized repeated field type")
	}

	return nil
}

// StreamEncode will perform a streaming encode of the given protobuf
// StreamMessage and write out the given writer. This function will
// return when all non-channel fields have been encoded, and goroutines
// will be spawned for the encoding of the channeled values. Those goroutines
// will receive on the channels and send values along as they get them until
// the StreamMessage is closed
func StreamEncode(w io.Writer, sm StreamMessage) error {
	// Parse out the protobuf struct tags
	props, err := GetProperties(sm)
	if err != nil {
		return err
	}

	val := reflect.ValueOf(sm).Elem()

	se := &streamEncoder{out: w, sm: sm}

	for protoField, fprop := range props.FieldMapping {
		field := val.Field(fprop.GoField)
		if fprop.Repeated {
			// Sanity check
			if field.Kind() != reflect.Chan {
				return errors.New("repeated field was not a channel")
			}

			go se.handleChannelIn(protoField, field)

		} else {
			if field.Kind() == reflect.Ptr {
				field = field.Elem()
			}
			err := writeProtoVal(w, sm, protoField, field.Interface())
			if err != nil {
				return err
			}
		}
	}

	return nil
}

// streamEncoder is a helper struct to ensure that concurrent writes
// dont get intermingled.
type streamEncoder struct {
	out io.Writer
	sm  StreamMessage
	lk  sync.Mutex
}

func (se *streamEncoder) handleChannelIn(field byte, ch reflect.Value) {
	for {
		val, ok := ch.Recv()
		if !ok {
			return
		}

		se.lk.Lock()
		writeProtoVal(se.out, se.sm, field, val.Interface())
		se.lk.Unlock()
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
