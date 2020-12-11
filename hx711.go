package rz2

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"math"
)

var (
	pow23 int32 = 1 << 23
	pow24 int32 = 1 << 24
)

func ConvertStrain(b []byte, factor float64, convert bool) ([]int64, []float64, error) {
	var send_time, start_time int64
	var databytes int32

	buf := bytes.NewReader(b[:8])
	binary.Read(buf, binary.BigEndian, &send_time)
	buf = bytes.NewReader(b[8:12])
	binary.Read(buf, binary.BigEndian, &databytes)
	buf = bytes.NewReader(b[12:20])
	binary.Read(buf, binary.BigEndian, &start_time)

	datacountmod := (databytes - 8) % 4
	if datacountmod != 0 {
		return nil, nil, fmt.Errorf("data size error: %d", databytes)
	}

	datacount := (databytes - 8) / 4
	dt := float64(send_time-start_time) / float64(datacount)

	lsb := 1.0 / 128.0 / math.Pow(2.0, 24)

	mtime := make([]int64, 0)
	strain := make([]float64, 0)
	var value int32
	for i := 0; i < int(datacount); i++ {
		buf = bytes.NewReader(b[20+i*4 : 20+(i+1)*4])
		binary.Read(buf, binary.BigEndian, &value)
		value &= 0xffffff
		mtime = append(mtime, start_time+int64(dt*float64(i)))
		if value > pow23 {
			value -= pow24
		}
		if convert {
			strain = append(strain, float64(value)*lsb*4.0/factor*1e6)
		} else {
			strain = append(strain, float64(value))
		}
	}

	return mtime, strain, nil
}
