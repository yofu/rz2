package rz2

import (
	"bytes"
	"encoding/binary"
	"fmt"
)

func ConvertGill(b []byte) (int64, []float64, error) {
	if len(b) < 28 {
		return 0, nil, fmt.Errorf("not enough byte size")
	}
	var send_time int64
	var data_size int32
	buf := bytes.NewReader(b[:8])
	binary.Read(buf, binary.BigEndian, &send_time)
	buf = bytes.NewReader(b[8:12])
	binary.Read(buf, binary.BigEndian, &data_size)
	var angle float64
	var speed float64
	buf = bytes.NewReader(b[12:20])
	binary.Read(buf, binary.BigEndian, &angle)
	buf = bytes.NewReader(b[20:28])
	binary.Read(buf, binary.BigEndian, &speed)
	return send_time, []float64{angle, speed}, nil
}
