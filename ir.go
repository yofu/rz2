package rz2

import (
	"bytes"
	"encoding/binary"
	"fmt"
)

func ConvertIR(b []byte) (int64, int8, error) {
	if len(b) != 13 {
		return 0, 0, fmt.Errorf("invalid data: %d", len(b))
	}
	var send_time int64
	var data int8
	buf := bytes.NewReader(b[:8])
	binary.Read(buf, binary.BigEndian, &send_time)
	buf = bytes.NewReader(b[12:13])
	binary.Read(buf, binary.BigEndian, &data)
	return send_time, data, nil
}
