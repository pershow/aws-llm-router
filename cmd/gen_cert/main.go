package main

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"log"
	"math/big"
	"net"
	"os"
	"time"
)

// 这个小工具用于在本地生成自签 TLS 证书和私钥：
//   certs/llmrouter.crt
//   certs/llmrouter.key
//
// 证书包含：
//   - CN=llmrouter.com
//   - DNS 名称: llmrouter.com
//   - IP: 127.0.0.1
//
// 用法（在项目根目录）:
//   go run ./cmd/gen_cert
func main() {
	const (
		host     = "llmrouter.com"
		certPath = "certs/llmrouter.crt"
		keyPath  = "certs/llmrouter.key"
	)

	if err := os.MkdirAll("certs", 0o700); err != nil {
		log.Fatalf("failed to create certs directory: %v", err)
	}

	// 生成 RSA 私钥
	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		log.Fatalf("failed to generate private key: %v", err)
	}

	// 生成证书模板
	serialNumberLimit := new(big.Int).Lsh(big.NewInt(1), 62)
	serialNumber, err := rand.Int(rand.Reader, serialNumberLimit)
	if err != nil {
		log.Fatalf("failed to generate serial number: %v", err)
	}

	template := x509.Certificate{
		SerialNumber: serialNumber,
		Subject: pkix.Name{
			CommonName: host,
		},
		NotBefore: time.Now().Add(-time.Hour),
		NotAfter:  time.Now().Add(365 * 24 * time.Hour),

		KeyUsage:              x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
		DNSNames:              []string{host},
		IPAddresses:           []net.IP{net.ParseIP("127.0.0.1")},
	}

	// 自签证书
	derBytes, err := x509.CreateCertificate(rand.Reader, &template, &template, &priv.PublicKey, priv)
	if err != nil {
		log.Fatalf("failed to create certificate: %v", err)
	}

	// 写证书文件
	certOut, err := os.Create(certPath)
	if err != nil {
		log.Fatalf("failed to open %s for writing: %v", certPath, err)
	}
	defer certOut.Close()

	if err := pem.Encode(certOut, &pem.Block{Type: "CERTIFICATE", Bytes: derBytes}); err != nil {
		log.Fatalf("failed to write certificate: %v", err)
	}

	// 写私钥文件
	keyOut, err := os.Create(keyPath)
	if err != nil {
		log.Fatalf("failed to open %s for writing: %v", keyPath, err)
	}
	defer keyOut.Close()

	if err := pem.Encode(keyOut, &pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(priv)}); err != nil {
		log.Fatalf("failed to write private key: %v", err)
	}

	log.Printf("自签证书已生成：\n  证书: %s\n  私钥: %s\n", certPath, keyPath)
}

