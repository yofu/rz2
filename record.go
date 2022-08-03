package rz2

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"
)

func ConvertUnixtime(unixmilli int64) time.Time {
	sec := unixmilli / 1000
	nsec := (unixmilli - 1000*sec) * 1000000
	return time.Unix(sec, nsec)
}

type ClientRecord struct {
	Time int64
	Size int32
	Data []byte
	Type string
}

func processClientRecord(b []byte) (ClientRecord, error) {
	if len(b) < 12 {
		return ClientRecord{}, fmt.Errorf("not enough data: %d", len(b))
	}
	var send_time int64
	var databytes int32
	buf := bytes.NewReader(b[:8])
	binary.Read(buf, binary.BigEndian, &send_time)
	buf = bytes.NewReader(b[8:12])
	binary.Read(buf, binary.BigEndian, &databytes)

	if len(b)-12 < int(databytes) {
		return ClientRecord{}, fmt.Errorf("not enough data: %d < %d", len(b)-12, databytes)
	}
	return ClientRecord{
		Time: send_time,
		Size: databytes,
		Data: b[12:],
	}, nil
}

func ReadClientRecord(fn string) ([]ClientRecord, error) {
	f, err := os.Open(fn)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	records := make([]ClientRecord, 0)
	for {
		b := make([]byte, 12)
		n, err := f.Read(b)
		if err != nil {
			if err == io.EOF {
				return records, nil
			}
			return records, err
		}
		if n < 12 {
			return records, fmt.Errorf("reading header: %d != 12", n)
		}
		var send_time int64
		var databytes int32
		buf := bytes.NewReader(b[:8])
		binary.Read(buf, binary.BigEndian, &send_time)
		buf = bytes.NewReader(b[8:12])
		binary.Read(buf, binary.BigEndian, &databytes)

		bb := make([]byte, databytes)
		n, err = f.Read(bb)
		if err != nil {
			if err == io.EOF {
				return records, nil
			}
			return records, err
		}
		if n < int(databytes) {
			return records, fmt.Errorf("reading content: %d != %d", n, int(databytes))
		}
		r := ClientRecord{
			Time: send_time,
			Size: databytes,
			Data: bb,
		}
		records = append(records, r)
	}
}

type ServerRecord struct {
	ServerTime int64
	Topic      string
	Content    []byte
}

func ReadServerRecord(fn string) ([]ServerRecord, error) {
	f, err := os.Open(fn)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	records := make([]ServerRecord, 0)
	bufct := make([]byte, 8)
	bufsize := make([]byte, 4)
	readbyte := 0
	for {
		// Read current time
		n, err := f.Read(bufct)
		readbyte += n
		// fmt.Println(bufct)
		if n == 0 || err != nil {
			if err == io.EOF {
				// fmt.Println("1")
				return records, nil
			}
			return records, err
		}
		if n != 8 {
			return records, fmt.Errorf("reading current time: %d != 8", n)
		}
		var ct int64
		buf := bytes.NewReader(bufct)
		binary.Read(buf, binary.LittleEndian, &ct)

		// Read topic
		btopic := make([]byte, 0)
		for {
			tmpbuf := make([]byte, 1)
			n, err := f.Read(tmpbuf)
			readbyte += n
			if n == 0 || err != nil {
				if err == io.EOF {
					// fmt.Println("2")
					return records, nil
				}
				return records, err
			}
			if tmpbuf[0] == byte(0) {
				break
			}
			btopic = append(btopic, tmpbuf[0])
		}
		topic := string(btopic)

		// Read buffer size
		n, err = f.Read(bufsize)
		readbyte += n
		if n == 0 || err != nil {
			if err == io.EOF {
				// fmt.Println("3")
				return records, nil
			}
			return records, err
		}
		if n != 4 {
			return records, fmt.Errorf("reading data size: %d != 4", n)
		}
		var size int32
		buf = bytes.NewReader(bufsize)
		binary.Read(buf, binary.BigEndian, &size)

		// fmt.Println(ct, topic, size)
		// Read data
		data := make([]byte, size)
		n, err = f.Read(data)
		readbyte += n
		// fmt.Println(ct, topic, size, n, readbyte)
		if ct == 1582846003456 {
			_, strain, _ := ConvertStrain(data, 103, false)
			for i := 0; i < len(strain); i++ {
				fmt.Println(i+1, strain[i], data[20+i*4:20+(i+1)*4])
			}
		}
		if n == 0 || err != nil {
			if err == io.EOF {
				// fmt.Println("4")
				return records, nil
			}
			return records, err
		}
		sk := int64(size) - int64(n)
		if sk > 0 {
			return records, fmt.Errorf("reading data: %d != %d", n, size)
		}
		r := ServerRecord{
			ServerTime: ct,
			Topic:      topic,
			Content:    data,
		}
		records = append(records, r)
	}
}

type Recorder struct {
	sync.Mutex
	dest *os.File
}

func NewRecorder(dest *os.File) *Recorder {
	return &Recorder{
		dest: dest,
	}
}

func (r *Recorder) Close() error {
	if r.dest == nil {
		return fmt.Errorf("dest is nil")
	}
	return r.dest.Close()
}

func (r *Recorder) SetDest(dest *os.File) {
	r.Lock()
	if r.dest != nil {
		r.dest.Close()
	}
	r.dest = dest
	r.Unlock()
}

func TimeStampDest(directory string) (*os.File, error) {
	now := time.Now()
	w, err := os.Create(filepath.Join(directory, fmt.Sprintf("%s.dat", now.Format("2006-01-02-15-04-05"))))
	if err != nil {
		return nil, err
	}
	return w, nil
}

func (r *Recorder) Record(msg mqtt.Message) error {
	r.Lock()
	buf := new(bytes.Buffer)
	buf.Grow(8 + len([]byte(msg.Topic())) + 1 + 4 + len(msg.Payload()))
	// Current Time [ms]: 8byte
	ct := time.Now().UnixNano() / 1000000 // ms
	err := binary.Write(buf, endian, ct)
	if err != nil {
		return err
	}
	// Topic
	_, err = buf.WriteString(msg.Topic())
	if err != nil {
		return err
	}
	err = buf.WriteByte(byte(0))
	if err != nil {
		return err
	}
	// Data size: 4byte
	err = binary.Write(buf, endian, int32(len(msg.Payload())))
	if err != nil {
		return err
	}
	// Data
	_, err = buf.Write(msg.Payload())
	if err != nil {
		return err
	}
	_, err = buf.WriteTo(r.dest)
	r.Unlock()
	return err
}
