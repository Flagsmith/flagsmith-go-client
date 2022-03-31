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
		return getHashedPercentageForObjectIds(ids, iterations+1)
	}

	return value
}

func GetHashedPercentageForObjectIds(ids []string, iterations int) float64 {
	return hashedPercentageForObjectIdsFunc(ids, iterations)
}

var hashedPercentageForObjectIdsFunc = getHashedPercentageForObjectIds

func MockSetHashedPercentageForObjectIds(fn func([]string, int) float64) {
	hashedPercentageForObjectIdsFunc = fn
}
