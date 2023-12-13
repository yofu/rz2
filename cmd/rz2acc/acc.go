package main

import (
	"bytes"
	"encoding/binary"
	"fmt"

	"github.com/yofu/rz2"
)

func accStats(payload []byte) (string, error) {
	var send_time int64
	buf := bytes.NewReader(payload[:8])
	binary.Read(buf, binary.BigEndian, &send_time)
	acc, xind, err := rz2.ConvertAccPacket(payload[12:])
	if err != nil {
		return "", err
	}
	acc = acc[xind : len(acc)-3+xind]
	return fmt.Sprintf("%s, %v", rz2.ConvertUnixtime(send_time).Format("15:04:05.000"), acc), nil
}
