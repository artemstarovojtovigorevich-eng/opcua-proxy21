package cert

import (
	"crypto/ecdsa"
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"math/big"
	"net"
	"net/url"
	"os"
	"time"
)

type CertManager struct {
	CertFile string
	KeyFile  string
	AppURI   string
}

func NewCertManager(certFile, keyFile, appURI string) *CertManager {
	return &CertManager{
		CertFile: certFile,
		KeyFile:  keyFile,
		AppURI:   appURI,
	}
}

func (cm *CertManager) LoadOrGenerate(generate bool) ([]byte, *rsa.PrivateKey, error) {
	if generate || (cm.CertFile != "" && cm.KeyFile != "") {
		if generate {
			certPEM, keyPEM, err := Generate(cm.AppURI, 2048, 24*time.Hour)
			if err != nil {
				return nil, nil, fmt.Errorf("failed to generate cert: %v", err)
			}
			if err := os.WriteFile(cm.CertFile, certPEM, 0644); err != nil {
				return nil, nil, fmt.Errorf("failed to write %s: %v", cm.CertFile, err)
			}
			if err := os.WriteFile(cm.KeyFile, keyPEM, 0644); err != nil {
				return nil, nil, fmt.Errorf("failed to write %s: %v", cm.KeyFile, err)
			}
		}
		c, err := tls.LoadX509KeyPair(cm.CertFile, cm.KeyFile)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to load certificate: %s", err)
		}
		pk, ok := c.PrivateKey.(*rsa.PrivateKey)
		if !ok {
			return nil, nil, fmt.Errorf("invalid private key")
		}
		return c.Certificate[0], pk, nil
	}
	return nil, nil, nil
}

func Generate(host string, rsaBits int, validFor time.Duration) (certPEM, keyPEM []byte, err error) {
	if rsaBits == 0 {
		rsaBits = 2048
	}

	priv, err := rsa.GenerateKey(rand.Reader, rsaBits)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to generate private key: %s", err)
	}

	notBefore := time.Now()
	notAfter := notBefore.Add(validFor)

	serialNumberLimit := new(big.Int).Lsh(big.NewInt(1), 128)
	serialNumber, err := rand.Int(rand.Reader, serialNumberLimit)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to generate serial number: %s", err)
	}

	template := x509.Certificate{
		SerialNumber: serialNumber,
		Subject: pkix.Name{
			CommonName: "OPC UA Client",
		},
		NotBefore: notBefore,
		NotAfter:  notAfter,

		KeyUsage:              x509.KeyUsageContentCommitment | x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature | x509.KeyUsageDataEncipherment | x509.KeyUsageCertSign,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth, x509.ExtKeyUsageClientAuth},
		BasicConstraintsValid: true,
	}

	if ip := net.ParseIP(host); ip != nil {
		template.IPAddresses = append(template.IPAddresses, ip)
	} else {
		template.DNSNames = append(template.DNSNames, host)
	}
	if uri, err := url.Parse(host); err == nil {
		template.URIs = append(template.URIs, uri)
	}

	derBytes, err := x509.CreateCertificate(rand.Reader, &template, &template, publicKey(priv), priv)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create certificate: %s", err)
	}

	return pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: derBytes}), pem.EncodeToMemory(pemBlockForKey(priv)), nil
}

func publicKey(priv interface{}) interface{} {
	switch k := priv.(type) {
	case *rsa.PrivateKey:
		return &k.PublicKey
	case *ecdsa.PrivateKey:
		return &k.PublicKey
	default:
		return nil
	}
}

func pemBlockForKey(priv interface{}) *pem.Block {
	switch k := priv.(type) {
	case *rsa.PrivateKey:
		return &pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(k)}
	case *ecdsa.PrivateKey:
		b, err := x509.MarshalECPrivateKey(k)
		if err != nil {
			panic(fmt.Sprintf("Unable to marshal ECDSA private key: %v", err))
		}
		return &pem.Block{Type: "EC PRIVATE KEY", Bytes: b}
	default:
		return nil
	}
}
