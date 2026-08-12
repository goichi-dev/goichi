package utils

import (
	"crypto/md5"
	"crypto/rand"
	"crypto/sha1"
	"fmt"
	"time"
)

func UUID4() string {
	b := make([]byte, 16)
	_, err := rand.Read(b)
	if err != nil {
		return ""
	}

	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80

	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x",
		b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}

func UUID5() string {
	namespace := []byte("goichi")
	randomData := make([]byte, 16)
	rand.Read(randomData)

	hash := sha1.Sum(append(namespace, randomData...))
	b := hash[0:16]

	b[6] = (b[6] & 0x0f) | 0x50
	b[8] = (b[8] & 0x3f) | 0x80

	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x",
		b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}

func UUID6() string {
	b := make([]byte, 16)
	_, err := rand.Read(b)
	if err != nil {
		return ""
	}

	now := time.Now().UnixNano() / 1000000
	b[0] = byte((now >> 40) & 0xff)
	b[1] = byte((now >> 32) & 0xff)
	b[2] = byte((now >> 24) & 0xff)
	b[3] = byte((now >> 16) & 0xff)
	b[4] = byte((now >> 8) & 0xff)
	b[5] = byte(now & 0xff)

	b[6] = (b[6] & 0x0f) | 0x60
	b[8] = (b[8] & 0x3f) | 0x80

	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x",
		b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}

func UUID7() string {
	b := make([]byte, 16)
	_, err := rand.Read(b)
	if err != nil {
		return ""
	}

	now := time.Now().UnixNano() / 1000000
	b[0] = byte((now >> 40) & 0xff)
	b[1] = byte((now >> 32) & 0xff)
	b[2] = byte((now >> 24) & 0xff)
	b[3] = byte((now >> 16) & 0xff)
	b[4] = byte((now >> 8) & 0xff)
	b[5] = byte(now & 0xff)

	b[6] = (b[6] & 0x0f) | 0x70
	b[8] = (b[8] & 0x3f) | 0x80

	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x",
		b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}

func UUID8() string {
	randomData := make([]byte, 16)
	rand.Read(randomData)
	hash := md5.Sum(randomData)
	b := hash[0:16]

	b[6] = (b[6] & 0x0f) | 0x80
	b[8] = (b[8] & 0x3f) | 0x80

	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x",
		b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}
