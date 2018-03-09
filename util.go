package stratum

import (
	"bytes"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"runtime"
	"strconv"
)

func BinToHex(bin []byte) (string, error) {
	ret := make([]byte, len(bin)*2)
	_ = hex.Encode(ret, bin)
	return string(ret), nil
}

func BinToStr(bin []byte) string {
	return fmt.Sprintf("%v", bin)
}

func HexToBin(hex string, size int) ([]byte, error) {
	result := &bytes.Buffer{}

	i := 0
	for i < len(hex) && size > 0 {
		b := hex[i : i+2]
		val, err := strconv.ParseUint(b, 16, 8)
		if err != nil {
			return nil, err
		}
		if err := binary.Write(result, binary.LittleEndian, uint8(val)); err != nil {
			return nil, err
		}
		i += 2
		size--
	}
	return result.Bytes(), nil
}

// MyCaller returns the caller of the function that called it
// Used for debugging stratum locks
// Taken from https://stackoverflow.com/a/35213181/1761555
func MyCaller() string {

	// we get the callers as uintptrs - but we just need 1
	fpcs := make([]uintptr, 1)

	// skip 3 levels to get to the caller of whoever called Caller()
	n := runtime.Callers(3, fpcs)
	if n == 0 {
		return "n/a" // proper error her would be better
	}

	// get the info of the actual function that's in the pointer
	fun := runtime.FuncForPC(fpcs[0] - 1)
	if fun == nil {
		return "n/a"
	}

	// return its name
	return fun.Name()
}
