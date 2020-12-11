package rz2

import (
	"bytes"
	"encoding/binary"
)

// EncodeAmg converts array data to binary data
func EncodeAmg(data []float64) ([]byte, error) {
	buf := new(bytes.Buffer)
	buf.Grow(65 * 4)
	for _, v := range data {
		err := binary.Write(buf, endian, float32(v))
		if err != nil {
			return nil, err
		}
	}
	return buf.Bytes(), nil
}

// DecodeAmg converts binary data to array data
func DecodeAmg(b []byte) ([]float32, error) {
	buf := bytes.NewReader(b)
	size := buf.Len() / 4
	rtn := make([]float32, size)
	for i := 0; i < size; i++ {
		var val float32
		err := binary.Read(buf, endian, &val)
		if err != nil {
			return nil, err
		}
		rtn[i] = val
	}
	return rtn, nil
}

func ConvertAmg(b []byte) []float64 {
	convert := func(b1, b2 byte) int {
		return int(((0x07 & b2) << 8) | (0xff & b1))
	}
	rtn := make([]float64, 65)
	rtn[0] = float64(convert(b[0], b[1])) * 0.0625
	for x := 0; x < 8; x++ {
		for y := 0; y < 8; y++ {
			index := ((x<<3)+y)<<1 + 2
			t := convert(b[index], b[index+1])
			if (b[index+1] & 0x08) > 0 {
				rtn[x*8+y+1] = -0.25 * float64(t)
			} else {
				rtn[x*8+y+1] = 0.25 * float64(t)
			}
		}
	}
	return rtn
}
