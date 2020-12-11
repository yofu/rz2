package rz2

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	"math"
	"math/cmplx"

	"github.com/mjibson/go-dsp/fft"
)

const (
	freq355 = 62.5
	bufsize = 256
)

var (
	pow19  int = 1 << 19
	pow20  int = 1 << 20
	endian     = binary.LittleEndian
)

const factor = 980.665 / 256000.0

// convertAcc converts ADXL355's binary data to acceleration (acc01)
func ConvertAcc(b []byte) []float64 {
	size := len(b) / 3
	rtn := make([]float64, size)
	for i := 0; i < size; i++ {
		tmp := int(b[3*i]) * 4096
		tmp += int(b[3*i+1]) * 16
		tmp += int(b[3*i+2]) / 16
		if tmp > pow19 {
			tmp -= pow20
		}
		rtn[i] = float64(tmp) * factor
	}
	return rtn
}

func ConvertAccPacketWithTime(b []byte) (int64, []float64, int, error) {
	if len(b) <= 12 {
		return 0, nil, 0, fmt.Errorf("not enough data")
	}
	var send_time int64
	buf := bytes.NewReader(b[:8])
	binary.Read(buf, binary.BigEndian, &send_time)
	data, xind, err := ConvertAccPacket(b[12:])
	return send_time, data, xind, err
}

// ConvertAcc converts ADXL355's binary data to acceleration (acc02)
func ConvertAccPacket(b []byte) ([]float64, int, error) {
	rtn := make([]float64, 0)
	packet := 0
	xind := 0
	currentxind := 0
	ind := 0
	for {
		if ind+4 > len(b) {
			break
		}
		// read packet size
		var size int32
		buf := bytes.NewReader(b[ind : ind+4])
		binary.Read(buf, binary.BigEndian, &size)
		if size >= math.MaxInt16 || size < 0 {
			return rtn, xind, fmt.Errorf("size overflow: %d", size)
		}
		ind += 4
		if len(b) < ind+3*int(size) {
			return rtn, xind, fmt.Errorf("not enough size: %d < %d", len(b), ind+3*int(size))
		}
		// read packet content
		var tmpxind int
		tmpdata := make([]float64, size)
		for i := 0; i < int(size); i++ {
			tmp := int(b[ind+3*i]) * 4096
			tmp += int(b[ind+3*i+1]) * 16
			tmp += int(b[ind+3*i+2]) / 16
			if tmp > pow19 {
				tmp -= pow20
			}
			tmpdata[i] = float64(tmp) * factor
			if i < 3 && b[ind+3*i+2]&0x1 != 0 {
				tmpxind = i
			}
		}
		if packet == 0 {
			rtn = append(rtn, tmpdata...)
			xind = tmpxind
			currentxind = (xind + (3 - int(size)%3)) % 3
		} else {
			dxind := tmpxind - currentxind
			if dxind < 0 {
				dxind += 3
			}
			rtn = append(rtn, tmpdata[dxind:]...)
			currentxind = (currentxind + (3 - (len(tmpdata)-dxind)%3)) % 3
		}
		packet++
		ind += int(size) * 3
	}
	return rtn, xind, nil
}

func separateAcc(data []float64) ([]float64, []float64, []float64) {
	dx := make([]float64, len(data)/3)
	dy := make([]float64, len(data)/3)
	dz := make([]float64, len(data)/3)
	avex := 0.0
	avey := 0.0
	avez := 0.0
	for i, d := range data {
		switch i % 3 {
		case 0:
			avex += d
		case 1:
			avey += d
		case 2:
			avez += d
		}
	}
	avex /= float64(len(dx))
	avey /= float64(len(dy))
	avez /= float64(len(dz))
	for i := 0; i < len(data); i++ {
		switch i % 3 {
		case 0:
			dx[i/3] = data[i] - avex
		case 1:
			dy[i/3] = data[i] - avey
		case 2:
			dz[i/3] = data[i] - avez
		}
	}
	return dx, dy, dz
}

// Spectrum returns FFT-ed data of acceleration
func Spectrum(data []float64, ns, ew, ud int) [][]float64 {
	dx, dy, dz := separateAcc(data)
	ffts := make([][]complex128, 3)
	d := [][]float64{dx, dy, dz}
	ffts[0] = fft.FFTReal(d[ns])
	ffts[1] = fft.FFTReal(d[ew])
	ffts[2] = fft.FFTReal(d[ud])
	df := freq355 / float64(len(ffts[0]))
	rtn := make([][]float64, len(ffts[0]))
	for i := 0; i < len(ffts[0]); i++ {
		rtn[i] = []float64{df * float64(i), cmplx.Abs(ffts[0][i]) / freq355, cmplx.Abs(ffts[1][i]) / freq355, cmplx.Abs(ffts[2][i]) / freq355}
	}
	return rtn
}

// EncodeAcceleration converts acceleration to binary data
func EncodeAcceleration(data []float64) ([]byte, error) {
	buf := new(bytes.Buffer)
	buf.Grow(bufsize * 3 * 4)
	for _, v := range data {
		err := binary.Write(buf, endian, float32(v))
		if err != nil {
			return nil, err
		}
	}
	return buf.Bytes(), nil
}

// DecodeAcceleration converts binary data to acceleration
func DecodeAcceleration(b []byte) ([]float32, error) {
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

// EncodeSpectrum converts FFT-ed data to binary data
func EncodeSpectrum(data [][]float64) ([]byte, error) {
	buf := new(bytes.Buffer)
	buf.Grow(bufsize/2*3*4 + 6)
	// Data size
	err := binary.Write(buf, endian, int16(bufsize))
	if err != nil {
		return nil, err
	}
	// df
	err = binary.Write(buf, endian, float32(freq355/float32(bufsize)))
	if err != nil {
		return nil, err
	}
	// Amp.
	for _, v := range data[:bufsize/2] {
		err := binary.Write(buf, endian, float32(v[1]))
		if err != nil {
			return nil, err
		}
		err = binary.Write(buf, endian, float32(v[2]))
		if err != nil {
			return nil, err
		}
		err = binary.Write(buf, endian, float32(v[3]))
		if err != nil {
			return nil, err
		}
	}
	return buf.Bytes(), nil
}

// DecodeSpectrum converts binary data to FFT-ed data
func DecodeSpectrum(b []byte) (int16, float32, [][]float32, error) {
	if len(b) < 6 {
		return 0, 0, nil, fmt.Errorf("not enough length")
	}
	var size int16
	var df float32
	buf := bytes.NewReader(b)
	err := binary.Read(buf, endian, &size)
	if err != nil {
		return 0, 0, nil, err
	}
	err = binary.Read(buf, endian, &df)
	if err != nil {
		return 0, 0, nil, err
	}
	rtn := make([][]float32, size/2)
	for i := 0; i < int(size)/2; i++ {
		tmp := make([]float32, 3)
		err := binary.Read(buf, endian, tmp)
		if err != nil {
			return 0, 0, nil, err
		}
		rtn[i] = tmp
	}
	return size, df, rtn, nil
}

// AccSensor represents ADXL355
type AccSensor struct {
	number     int
	macaddress string
	buffer     []float64
	count      int
	ns         int
	ew         int
	ud         int
}

func NewAccSensor(num int, address string, ns, ew, ud int) *AccSensor {
	return &AccSensor{
		number:     num,
		macaddress: address,
		buffer:     make([]float64, bufsize*3),
		count:      0,
		ns:         ns,
		ew:         ew,
		ud:         ud,
	}
}

func (s *AccSensor) Initialize() {
	s.buffer = make([]float64, bufsize*3)
	s.count = 0
}

func (s *AccSensor) MacAddress() string {
	return s.macaddress
}

func (s *AccSensor) Limit() int {
	return bufsize*3 - s.count
}

func (s *AccSensor) Add(acc []float64, size int) {
	for i := 0; i < size; i++ {
		s.buffer[s.count+i] = acc[i]
	}
	s.count += size
}

func (s *AccSensor) WriteTo(w io.Writer) (int64, error) {
	var otp bytes.Buffer
	for i := 0; i < bufsize; i++ {
		otp.WriteString(fmt.Sprintf("%f %f %f\n", s.buffer[3*i+s.ns], s.buffer[3*i+s.ew], s.buffer[3*i+s.ud]))
	}
	return otp.WriteTo(w)
}

func (s *AccSensor) Acceleration() ([]byte, error) {
	return EncodeAcceleration(s.buffer)
}

func (s *AccSensor) SpectrumData() ([]byte, error) {
	return EncodeSpectrum(Spectrum(s.buffer, s.ns, s.ew, s.ud))
}

func (s *AccSensor) JMASeismicIntensityScale() ([]byte, error) {
	jma, err := JMASeismicIntensityScale(s.buffer)
	if err != nil {
		return nil, err
	}
	buf := new(bytes.Buffer)
	buf.Grow(4)
	err = binary.Write(buf, endian, float32(jma))
	if err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}
