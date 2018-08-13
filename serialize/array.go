package serialize

import (
	"fmt"
	"math"
	"reflect"
	"unsafe"

	"github.com/shamaton/msgpack/def"
)

type serializer struct {
	common
}

func AsArray(v interface{}) ([]byte, error) {
	s := serializer{}

	// TODO : recover

	rv := reflect.ValueOf(v)
	if rv.Kind() == reflect.Ptr {
		rv = rv.Elem()
		if rv.Kind() == reflect.Ptr {
			rv = rv.Elem()
		}
	}
	size, err := s.calcSize(rv)
	if err != nil {
		return nil, err
	}

	s.d = make([]byte, size)
	last, err := s.create(rv, 0)
	if err != nil {
		return nil, err
	}
	if size != last {
		return nil, fmt.Errorf("failed serialization size=%d, lastIdx=%d", size, last)
	}
	return s.d, err
}

func (s *serializer) calcUint(v uint64) int {
	if v <= math.MaxInt8 {
		// format code only
		return 0
	} else if v <= math.MaxUint8 {
		return def.Byte1
	} else if v <= math.MaxUint16 {
		return def.Byte2
	} else if v <= math.MaxUint32 {
		return def.Byte4
	}
	return def.Byte8
}

func (s *serializer) calcInt(v int64) int {
	if v >= 0 {
		return s.calcUint(uint64(v))
	} else if s.isNegativeFixInt64(v) {
		// format code only
		return 0
	} else if v >= math.MinInt8 {
		return def.Byte1
	} else if v >= math.MinInt16 {
		return def.Byte2
	} else if v >= math.MinInt32 {
		return def.Byte4
	}
	return def.Byte8
}

func (s *serializer) calcSliceLength(l int) (int, error) {
	// format size
	if l <= 0x0f {
		// format code only
		return 0, nil
	} else if l <= math.MaxUint16 {
		return def.Byte2, nil
	} else if l <= math.MaxUint32 {
		return def.Byte4, nil
	}
	// not supported error
	return 0, fmt.Errorf("not support this array length : %d", l)
}

func (s *serializer) calcString(v string) int {
	// NOTE : unsafe
	strBytes := *(*[]byte)(unsafe.Pointer(&v))
	l := len(strBytes)
	if l < 32 {
		return l
	} else if l <= math.MaxUint8 {
		return def.Byte1 + l
	} else if l <= math.MaxUint16 {
		return def.Byte2 + l
	}
	return def.Byte4 + l
	// NOTE : length over uint32
}

func (s *serializer) calcSize(rv reflect.Value) (int, error) {
	ret := def.Byte1

	switch rv.Kind() {
	case reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uint:
		v := rv.Uint()
		ret += s.calcUint(v)

	case reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64, reflect.Int:
		v := rv.Int()
		if v >= 0 {
			ret += s.calcUint(uint64(v))
		} else if s.isNegativeFixInt64(v) {
			// format code only
		} else if v >= math.MinInt8 {
			ret += +def.Byte1
		} else if v >= math.MinInt16 {
			ret += +def.Byte2
		} else if v >= math.MinInt32 {
			ret += +def.Byte4
		} else {
			ret += def.Byte8
		}

	case reflect.Float32, reflect.Float64:
		v := rv.Float()
		if math.SmallestNonzeroFloat32 <= v && v <= math.MaxFloat32 {
			ret += def.Byte4
		} else {
			ret += def.Byte8
		}

	case reflect.String:
		ret += s.calcString(rv.String())

	case reflect.Bool:
		// do nothing

	case reflect.Array, reflect.Slice:
		if rv.IsNil() {
			return ret, nil
		}
		// TODO : isNil
		l := rv.Len()

		// format size
		if l <= 0x0f {
			// format code only
		} else if l <= math.MaxUint16 {
			ret += def.Byte2
		} else if l <= math.MaxUint32 {
			ret += def.Byte4
		} else {
			// not supported error
			return 0, fmt.Errorf("not support this array length : %d", l)
		}

		// objects size
		for i := 0; i < l; i++ {
			s, err := s.calcSize(rv.Index(i))
			if err != nil {
				return 0, err
			}
			ret += s
		}
	}

	return ret, nil
}

func (s *serializer) create(rv reflect.Value, offset int) (int, error) {

	switch rv.Kind() {
	case reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uint:
		v := rv.Uint()
		offset = s.writeUint(v, offset)

	case reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64, reflect.Int:
		v := rv.Int()
		if v >= 0 {
			offset = s.writeUint(uint64(v), offset)
		} else if s.isNegativeFixInt64(v) {
			offset = s.writeSize1Int64(v, offset)
		} else if v >= math.MinInt8 {
			offset = s.writeSize1Int(def.Int8, offset)
			offset = s.writeSize1Int64(v, offset)
		} else if v >= math.MinInt16 {
			offset = s.writeSize1Int(def.Int16, offset)
			offset = s.writeSize2Int64(v, offset)
		} else if v >= math.MinInt32 {
			offset = s.writeSize1Int(def.Int32, offset)
			offset = s.writeSize4Int64(v, offset)
		} else {
			offset = s.writeSize1Int(def.Int64, offset)
			offset = s.writeSize8Int64(v, offset)
		}

	case reflect.Float32, reflect.Float64:
		v := rv.Float()
		if math.SmallestNonzeroFloat32 <= v && v <= math.MaxFloat32 {
			offset = s.writeSize1Int(def.Float32, offset)
			offset = s.writeSize4Uint64(uint64(math.Float32bits(float32(v))), offset)
		} else {
			offset = s.writeSize1Int(def.Float64, offset)
			offset = s.writeSize8Uint64(math.Float64bits(v), offset)
		}

	case reflect.Bool:
		if rv.Bool() {
			offset = s.writeSize1Int(def.True, offset)
		} else {
			offset = s.writeSize1Int(def.False, offset)
		}

	case reflect.String:
		str := rv.String()

		// NOTE : unsafe
		strBytes := *(*[]byte)(unsafe.Pointer(&str))
		l := len(strBytes)
		if l < 32 {
			offset = s.writeSize1Int(def.FixStr+l, offset)
			offset = s.writeBytes(strBytes, offset)
		} else if l <= math.MaxUint8 {
			offset = s.writeSize1Int(def.Str8, offset)
			offset = s.writeSize1Int(l, offset)
			offset = s.writeBytes(strBytes, offset)
		} else if l <= math.MaxUint16 {
			offset = s.writeSize1Int(def.Str16, offset)
			offset = s.writeSize2Int(l, offset)
			offset = s.writeBytes(strBytes, offset)
		} else {
			offset = s.writeSize1Int(def.Str32, offset)
			offset = s.writeSize4Int(l, offset)
			offset = s.writeBytes(strBytes, offset)
		}

	case reflect.Array, reflect.Slice:
		if rv.IsNil() {
			offset = s.writeSize1Int(def.Nil, offset)
			return offset, nil
		}
		l := rv.Len()

		// format
		if l <= 0x0f {
			offset = s.writeSize1Int(def.FixArray+l, offset)
		} else if l <= math.MaxUint16 {
			offset = s.writeSize1Int(def.Array16, offset)
			offset = s.writeSize2Int(l, offset)
		} else if l <= math.MaxUint32 {
			offset = s.writeSize1Int(def.Array32, offset)
			offset = s.writeSize4Int(l, offset)
		} else {
			// not supported error
			return 0, fmt.Errorf("not support this array length : %d", l)
		}

		// objects
		for i := 0; i < l; i++ {
			var err error
			offset, err = s.create(rv.Index(i), offset)
			if err != nil {
				return 0, err
			}
		}
	}
	return offset, nil
}

func (s *serializer) writeUint(v uint64, offset int) int {
	if v <= math.MaxInt8 {
		offset = s.writeSize1Uint64(v, offset)
	} else if v <= math.MaxUint8 {
		offset = s.writeSize1Int(def.Uint8, offset)
		offset = s.writeSize1Uint64(v, offset)
	} else if v <= math.MaxUint16 {
		offset = s.writeSize1Int(def.Uint16, offset)
		offset = s.writeSize2Uint64(v, offset)
	} else if v <= math.MaxUint32 {
		offset = s.writeSize1Int(def.Uint32, offset)
		offset = s.writeSize4Uint64(v, offset)
	} else {
		offset = s.writeSize1Int(def.Uint64, offset)
		offset = s.writeSize8Uint64(v, offset)
	}
	return offset
}

func (s *serializer) writeInt(v int64, offset int) int {
	if v >= 0 {
		offset = s.writeUint(uint64(v), offset)
	} else if s.isNegativeFixInt64(v) {
		offset = s.writeSize1Int64(v, offset)
	} else if v >= math.MinInt8 {
		offset = s.writeSize1Int(def.Int8, offset)
		offset = s.writeSize1Int64(v, offset)
	} else if v >= math.MinInt16 {
		offset = s.writeSize1Int(def.Int16, offset)
		offset = s.writeSize2Int64(v, offset)
	} else if v >= math.MinInt32 {
		offset = s.writeSize1Int(def.Int32, offset)
		offset = s.writeSize4Int64(v, offset)
	} else {
		offset = s.writeSize1Int(def.Int64, offset)
		offset = s.writeSize8Int64(v, offset)
	}
	return offset
}

func (s *serializer) writeSliceLength(l int, offset int) int {
	// format size
	if l <= 0x0f {
		offset = s.writeSize1Int(def.FixArray+l, offset)
	} else if l <= math.MaxUint16 {
		offset = s.writeSize1Int(def.Array16, offset)
		offset = s.writeSize2Int(l, offset)
	} else if l <= math.MaxUint32 {
		offset = s.writeSize1Int(def.Array16, offset)
		offset = s.writeSize4Int(l, offset)
	}
	return offset
}
