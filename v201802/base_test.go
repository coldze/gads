package v201802

import (
	"crypto/rand"
	"testing"
)

func rand_str(str_size int) string {
	alphanum := "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz"
	var bytes = make([]byte, str_size)
	rand.Read(bytes)
	for i, b := range bytes {
		bytes[i] = alphanum[b%byte(len(alphanum))]
	}
	return string(bytes)
}

func rand_word(str_size int) string {
	alphanum := "abcdefghijklmnopqrstuvwxyz"
	var bytes = make([]byte, str_size)
	rand.Read(bytes)
	for i, b := range bytes {
		bytes[i] = alphanum[b%byte(len(alphanum))]
	}
	return string(bytes)
}

func testAuthSetup(t *testing.T) Auth {
	config, err := NewCredentialsFromParams(Credentials{})
	if err != nil {
		t.Fatal(err)
	}
	config.Auth.Testing = t
	return config.Auth
}
