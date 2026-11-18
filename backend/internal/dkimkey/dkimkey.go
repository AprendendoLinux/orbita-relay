// Package dkimkey gera chaves DKIM e o texto do registro DNS.
package dkimkey

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"errors"
)

// Generate cria uma chave RSA 2048 e devolve a privada em PEM e a pública em base64 (DER PKIX).
func Generate() (privPEM, pubB64 string, err error) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return "", "", errors.New("falha ao gerar chave")
	}
	privPEM = string(pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)}))
	der, _ := x509.MarshalPKIXPublicKey(&key.PublicKey)
	return privPEM, base64.StdEncoding.EncodeToString(der), nil
}

// PublicFromPEM extrai a chave pública (base64) de uma chave privada em PEM.
func PublicFromPEM(privPEM string) (string, error) {
	b, _ := pem.Decode([]byte(privPEM))
	if b == nil {
		return "", errors.New("PEM inválido")
	}
	key, err := x509.ParsePKCS1PrivateKey(b.Bytes)
	if err != nil {
		return "", err
	}
	der, err := x509.MarshalPKIXPublicKey(&key.PublicKey)
	if err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(der), nil
}

// Record monta o valor do registro TXT.
func Record(pubB64 string) string { return "v=DKIM1; k=rsa; p=" + pubB64 }
