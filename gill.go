package rz2

func ConvertGill(b []byte) (int64, string, error) {
	return 0, string(b), nil
	// var send_time int64
	// var data_size int32
	// buf := bytes.NewReader(b[:8])
	// binary.Read(buf, binary.BigEndian, &send_time)
	// buf = bytes.NewReader(b[8:12])
	// binary.Read(buf, binary.BigEndian, &data_size)
	// data := make([]int, data_size)
	// var d int16
	// for i := 0; i < int(data_size); i++ {
	// 	buf = bytes.NewReader(b[12+2*i:12+2*i+2])
	// 	binary.Read(buf, binary.BigEndian, &d)
	// 	data[i] = int(d)
	// }
	// return send_time, data, nil
}
