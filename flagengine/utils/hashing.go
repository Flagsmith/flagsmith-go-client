package utils

import (
	"crypto/md5"
	"math/big"
	"strings"
)

// GetHashedPercentageForObjectIds returns a number in range [0:100) based on hashes of ids.
func getHashedPercentageForObjectIds(ids []string, iterations int) float64 {
	strs := make([]string, len(ids)*iterations)
	for i := 0; i < len(strs); i++ {
		strs[i] = ids[i%len(ids)]
	}
	toHash := strings.Join(strs, ",")
	hash := md5.Sum([]byte(toHash))
	var value float64
	var hashValue big.Int
	hashValue.SetBytes(hash[:])

	value = (float64(hashValue.Mod(&hashValue, big.NewInt(9999)).Int64()) / 9998.0) * 100.0
	if value == 100 {
		return GetHashedPercentageForObjectIds(ids, iterations+1)
	}

	return value
}

func GetHashedPercentageForObjectIds(ids []string, iterations int) float64 {
	fn := getHashedPercentageForObjectIds
	if mockHashedPercentageForObjectIds != nil {
		fn = mockHashedPercentageForObjectIds
	}
	return fn(ids, iterations)
}

var mockHashedPercentageForObjectIds func([]string, int) float64

func SetMockHashedPercentageForObjectIds(fn func([]string, int) float64) {
	mockHashedPercentageForObjectIds = fn
}
