package main

import (
	"bytes"
	"flag"
	"fmt"
	"log"
	"math/cmplx"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"text/template"
	"time"

	"github.com/mjibson/go-dsp/fft"
	"github.com/yofu/rz2"
)

func SearchDatFile(t time.Time, dir string) ([]string, error) {
	stat, err := os.Stat(dir)
	if err != nil {
		return nil, err
	}
	if !stat.IsDir() {
		return nil, fmt.Errorf("%s is not directory", dir)
	}
	f, err := os.Open(dir)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	lis, err := f.Readdirnames(-1)
	if err != nil {
		return nil, err
	}
	sort.Strings(lis)
	pattern := t.Format("2006-01-02-15")
	ind := 0
	for i, fn := range lis {
		if strings.HasPrefix(fn, pattern) {
			ind = i
			break
		}
	}
	switch ind {
	case 0:
		if len(lis) < 2 {
			return lis, nil
		}
		return lis[ind : ind+2], nil
	case len(lis) - 1:
		return lis[ind-1 : ind+1], nil
	default:
		if len(lis) < ind+2 {
			return lis[ind-1:], nil
		}
		return lis[ind-1 : ind+2], nil
	}
}

func CalcFFT(acc []float64, subave bool) []complex128 {
	if subave {
		// Base line correction
		ave := 0.0
		for i := 0; i < len(acc); i++ {
			ave += acc[i]
		}
		ave /= float64(len(acc))
		for i := 0; i < len(acc); i++ {
			acc[i] -= ave
		}
	}
	// FFT
	return fft.FFTReal(acc)
}

var (
	macaddress = map[string]string{
		"moncli01": "b8:27:eb:fd:5a:ea",
		"flab01":   "b8:27:eb:a5:88:33",
		"flab02":   "b8:27:eb:55:4d:0f",
		"flab03":   "b8:27:eb:19:f7:09",
		"flab04":   "b8:27:eb:db:e7:f8",
		"rz7":      "b8:27:eb:e0:61:11",
		"rz8":      "b8:27:eb:00:a2:74",
		"rz9":      "b8:27:eb:93:82:33",
		"rz10":     "b8:27:eb:54:78:27",
	}
)

type AccUnit struct {
	name       string
	macaddress string
	time       []int
	xacc       []float64
	yacc       []float64
	zacc       []float64
	freq       float64
	df         float64
	fftstart   int
	fftsize    int
	startind   int
	fftx       []complex128
	ffty       []complex128
	fftz       []complex128
	ampx       []float64
	ampy       []float64
	ampz       []float64
}

func NewAccUnit(name string, fftstart, fftsize int) *AccUnit {
	return &AccUnit{
		name:       name,
		macaddress: macaddress[name],
		time:       make([]int, 0),
		xacc:       make([]float64, 0),
		yacc:       make([]float64, 0),
		zacc:       make([]float64, 0),
		fftstart:   fftstart,
		fftsize:    fftsize,
		ampx:       make([]float64, fftsize),
		ampy:       make([]float64, fftsize),
		ampz:       make([]float64, fftsize),
	}
}

func (unit *AccUnit) WriteAcc(fn string) error {
	var otp bytes.Buffer
	for i := 0; i < len(unit.time); i++ {
		otp.WriteString(fmt.Sprintf("%d %+15.12f %+15.12f %+15.12f\n", unit.time[i], unit.xacc[i], unit.yacc[i], unit.zacc[i]))
	}
	w, err := os.Create(fn)
	if err != nil {
		return err
	}
	defer w.Close()
	_, err = otp.WriteTo(w)
	if err != nil {
		return err
	}
	return nil
}

func (unit *AccUnit) CalcFFT(subave bool, smooth int) error {
	unit.startind = 0
	for i := 0; i < len(unit.time); i++ {
		if unit.time[i] >= unit.fftstart {
			break
		}
		unit.startind++
	}
	if len(unit.time) < unit.startind+unit.fftsize {
		return fmt.Errorf("not enough data. read next .dat file")
	}
	time2 := make([]int, unit.fftsize)
	xacc2 := make([]float64, unit.fftsize)
	yacc2 := make([]float64, unit.fftsize)
	zacc2 := make([]float64, unit.fftsize)
	for i := 0; i < unit.fftsize; i++ {
		time2[i] = unit.time[unit.startind+i]
		xacc2[i] = unit.xacc[unit.startind+i]
		yacc2[i] = unit.yacc[unit.startind+i]
		zacc2[i] = unit.zacc[unit.startind+i]
	}
	unit.freq = float64(unit.fftsize) / float64(unit.time[unit.startind+unit.fftsize]-unit.time[unit.startind]) * 1000
	unit.df = unit.freq / float64(unit.fftsize)
	fmt.Printf("FREQUENCY       : %.3f\ndf              : %.3f\n", unit.freq, unit.df)
	unit.fftx = CalcFFT(xacc2, subave)
	unit.ffty = CalcFFT(yacc2, subave)
	unit.fftz = CalcFFT(zacc2, subave)
	for i := 0; i < unit.fftsize; i++ {
		unit.ampx[i] = cmplx.Abs(unit.fftx[i])
		unit.ampy[i] = cmplx.Abs(unit.ffty[i])
		unit.ampz[i] = cmplx.Abs(unit.fftz[i])
	}
	if smooth > 0 {
		fmt.Printf("SMOOTH: %d\n", smooth)
		ampx := make([]float64, unit.fftsize)
		ampy := make([]float64, unit.fftsize)
		ampz := make([]float64, unit.fftsize)
		for i := 0; i < unit.fftsize; i++ {
			var start, end int
			switch {
			case i < smooth:
				start = 0
				end = i + smooth
			case i >= unit.fftsize-smooth-1:
				start = i - smooth
				end = unit.fftsize - 1
			default:
				start = i - smooth
				end = i + smooth
			}
			for j := start; j <= end; j++ {
				ampx[i] += unit.ampx[j]
			}
			ampx[i] /= float64(end - start + 1)
			for j := start; j <= end; j++ {
				ampy[i] += unit.ampy[j]
			}
			ampy[i] /= float64(end - start + 1)
			for j := start; j <= end; j++ {
				ampz[i] += unit.ampz[j]
			}
			ampz[i] /= float64(end - start + 1)
		}
		for i := 0; i < unit.fftsize; i++ {
			unit.ampx[i] = ampx[i]
			unit.ampy[i] = ampy[i]
			unit.ampz[i] = ampz[i]
		}
	}
	return nil
}

func (unit *AccUnit) OutputFFT(otpfn string) error {
	var otp bytes.Buffer
	for i := 0; i < unit.fftsize; i++ {
		otp.WriteString(fmt.Sprintf("%d %+15.12f %+15.12f %+15.12f %.3f %+15.12f %+15.12f %+15.12f\n", unit.time[unit.startind+i], unit.xacc[unit.startind+i], unit.yacc[unit.startind+i], unit.zacc[unit.startind+i], unit.df*float64(i), unit.ampx[i], unit.ampy[i], unit.ampz[i]))
	}
	w, err := os.Create(otpfn)
	if err != nil {
		return err
	}
	defer w.Close()
	_, err = otp.WriteTo(w)
	if err != nil {
		return err
	}
	return nil
}

func OutputFFT(otpfn string, units ...*AccUnit) error {
	var otp bytes.Buffer
	for i := 0; i < units[0].fftsize; i++ {
		for j := 0; j < len(units); j++ {
			otp.WriteString(fmt.Sprintf("%d %+15.12f %+15.12f %+15.12f %.3f %+15.12f %+15.12f %+15.12f ", units[j].time[units[j].startind+i], units[j].xacc[units[j].startind+i], units[j].yacc[units[j].startind+i], units[j].zacc[units[j].startind+i], units[j].df*float64(i), units[j].ampx[i], units[j].ampy[i], units[j].ampz[i]))
		}
		otp.WriteString("\n")
	}
	w, err := os.Create(otpfn)
	if err != nil {
		return err
	}
	defer w.Close()
	_, err = otp.WriteTo(w)
	if err != nil {
		return err
	}
	return nil
}

var (
	accunits = make(map[string]*AccUnit, 0)
)

func ReadData(datdir string, fns ...string) error {
	for _, fn := range fns {
		records, _ := rz2.ReadServerRecord(filepath.Join(datdir, fn))
		// if err != nil {
		// 	return err
		// }
		var unit *AccUnit
		for _, rec := range records {
			lis := strings.Split(rec.Topic, "/")
			if _, ok := accunits[lis[0]]; !ok {
				continue
			}
			unit = accunits[lis[0]]
			switch lis[2] {
			case "acc02":
				send_time, acc, xind, err := rz2.ConvertAccPacketWithTime(rec.Content)
				if err != nil {
					fmt.Println(err)
				}
				size := (len(acc) - xind) / 3
				var dt float64
				var lt int64 = 0
				if unit.time != nil && len(unit.time) > 0 {
					lt = int64(unit.time[len(unit.time)-1])
				}
				if lt == 0 {
					dt = 8.0
				} else {
					dt = float64(send_time-lt) / float64(size)
				}
				for i := 0; i < size; i++ {
					ct := int(float64(send_time) - float64(size-1-i)*dt)
					unit.time = append(unit.time, ct)
					unit.xacc = append(unit.xacc, acc[xind+3*i])
					unit.yacc = append(unit.yacc, acc[xind+3*i+1])
					unit.zacc = append(unit.zacc, acc[xind+3*i+2])
				}
				lt = send_time
			default:
			}
		}
	}
	return nil
}

type Plot struct {
	Filename string
}

func Gnuplot(filename string) error {
	plt := &Plot{
		Filename: filename,
	}
	tmpl, err := template.ParseFiles("template.plt")
	if err != nil {
		return err
	}
	cmd := exec.Command("C:\\gnuplot\\bin\\gnuplot.exe", "-p", "-")
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return err
	}
	go func() {
		defer stdin.Close()
		err := tmpl.Execute(stdin, plt)
		if err != nil {
			log.Fatal(err)
		}
	}()
	_, err = cmd.CombinedOutput()
	if err != nil {
		return err
	}
	return nil
}

func main() {
	starttime := flag.String("start", "1970-01-01T00:00:00.000", "start time for FFT")
	fftsize := flag.Int("fftsize", 0, "FFT size")
	subave := flag.Bool("subave", true, "subtract average or not")
	datdir := flag.String("dir", "..\\dat", "dat directory")
	smooth := flag.Int("smooth", 0, "smooting")
	plotonly := flag.Bool("p", false, "plot only")
	flag.Parse()

	loc, _ := time.LoadLocation("Asia/Tokyo")
	t, err := time.ParseInLocation("2006-01-02T15:04:05.000", *starttime, loc)
	if err != nil {
		log.Fatal(err)
	}
	fftstart := int(t.UnixNano() / 1000000)

	filename := fmt.Sprintf("%s-%d-%d", t.Format("2006-01-02-15-04-05"), *fftsize, *smooth)
	if !*plotonly {
		datfn, err := SearchDatFile(t, *datdir)
		if err != nil {
			log.Fatal(err)
		}
		fmt.Printf("INPUT FILE      : %v\n", datfn)
		fmt.Printf("FFT START TIME  : %s, %d\n", t.Format("2006-01-02T15:04:05.000"), fftstart)
		fmt.Printf("FFT SIZE        : %d\n", *fftsize)
		fmt.Printf("SUBTRACT AVERAGE: %t\n", *subave)

		unames := []string{"flab01", "flab02", "flab03", "flab04"}
		// unames := []string{"rz8", "rz9"}
		// unames := []string{"moncli01"}
		for _, uname := range unames {
			accunits[macaddress[uname]] = NewAccUnit(uname, fftstart, *fftsize)
		}

		err = ReadData(*datdir, datfn...)
		if err != nil {
			log.Fatal(err)
		}

		for _, uname := range unames {
			err = accunits[macaddress[uname]].CalcFFT(*subave, *smooth)
			if err != nil {
				log.Fatal(err)
			}
		}

		// err = OutputFFT(fmt.Sprintf("%s.dat", filename),
		// 	accunits[macaddress["rz8"]],
		// 	accunits[macaddress["rz9"]])
		err = OutputFFT(fmt.Sprintf("%s.dat", filename),
			accunits[macaddress["flab01"]],
			accunits[macaddress["flab02"]],
			accunits[macaddress["flab03"]],
			accunits[macaddress["flab04"]])
		// err = OutputFFT(fmt.Sprintf("%s.dat", filename),
		// 	accunits[macaddress["moncli01"]])
		if err != nil {
			log.Fatal(err)
		}
	}

	err = Gnuplot(filename)
	if err != nil {
		log.Fatal(err)
	}
}
