package rz2

import (
	"bytes"
	"encoding/binary"
)

func ConvertSht(b []byte) (int64, []float64, []float64, error) {
	// if len(b) != 13 {
	// 	return 0, nil, fmt.Errorf("invalid data: %d", len(b))
	// }
	var send_time int64
	var data_size int32
	buf := bytes.NewReader(b[:8])
	binary.Read(buf, binary.BigEndian, &send_time)
	buf = bytes.NewReader(b[8:12])
	binary.Read(buf, binary.BigEndian, &data_size)
	tmp_data := make([]float64, data_size/2)
	hum_data := make([]float64, data_size/2)
	var d float32
	tmp := true
	for i := 0; i < int(data_size); i++ {
		buf = bytes.NewReader(b[12+4*i:12+4*i+4])
		binary.Read(buf, binary.BigEndian, &d)
		if tmp {
			tmp_data[i/2] = float64(d)
		} else {
			hum_data[i/2] = float64(d)
		}
		tmp = !tmp
	}
	return send_time, tmp_data, hum_data, nil
}

