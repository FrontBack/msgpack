package decoding

import "github.com/FrontBack/msgpack/def"

func (d *decoder) isCodeNil(v byte) bool {
	return def.Nil == v
}
