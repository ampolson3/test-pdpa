package crypto

import (
	"encoding/base64"
	"errors"
	"fmt"
	"os"
)

// KEKFromEnv builds the key-encryption key every process of an environment shares:
//
//	KEK_PROVIDER=transit (default): OPENBAO_ADDR, OPENBAO_TOKEN, OPENBAO_TRANSIT_MOUNT (default "transit")
//	KEK_PROVIDER=local (development only): LOCAL_KEK_BASE64 = 32 random bytes, base64
func KEKFromEnv() (KEK, error) {
	switch p := os.Getenv("KEK_PROVIDER"); p {
	case "", "transit":
		addr, token := os.Getenv("OPENBAO_ADDR"), os.Getenv("OPENBAO_TOKEN")
		if addr == "" || token == "" {
			return nil, errors.New("crypto: OPENBAO_ADDR and OPENBAO_TOKEN are required (or KEK_PROVIDER=local for development)")
		}
		return &Transit{Addr: addr, Token: token, Mount: os.Getenv("OPENBAO_TRANSIT_MOUNT")}, nil
	case "local":
		key, err := base64.StdEncoding.DecodeString(os.Getenv("LOCAL_KEK_BASE64"))
		if err != nil || len(key) != 32 {
			return nil, errors.New("crypto: LOCAL_KEK_BASE64 must be 32 bytes, base64")
		}
		return NewLocalKEKFromKey(key), nil
	default:
		return nil, fmt.Errorf("crypto: unknown KEK_PROVIDER %q", p)
	}
}

// NewLocalKEKFromKey is LocalKEK with a fixed master key, so separate processes (api, worker) of a
// development environment can read each other's data. Development only.
func NewLocalKEKFromKey(key []byte) *LocalKEK {
	return &LocalKEK{versions: [][]byte{append([]byte(nil), key...)}}
}
