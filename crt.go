package rz2

import (
	"crypto/tls"
	"crypto/x509"
	"io/ioutil"
)

// NewTLSConfig creates *tls.Config for sakura2
func NewTLSConfig(cafile, crtfile, keyfile string) (*tls.Config, error) {
	certpool := x509.NewCertPool()
	ca, err := ioutil.ReadFile(cafile)
	if err != nil {
		return nil, err
	}
	certpool.AppendCertsFromPEM(ca)
	cer, err := tls.LoadX509KeyPair(crtfile, keyfile)
	if err != nil {
		return nil, err
	}
	return &tls.Config{
		RootCAs:            certpool,
		ClientAuth:         tls.NoClientCert,
		ClientCAs:          nil,
		InsecureSkipVerify: true,
		Certificates:       []tls.Certificate{cer},
	}, nil
}
