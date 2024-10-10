package core

import (
	"github.com/ethereum/go-ethereum/paircache/pairtypes"
	"math/rand"
	"time"
)

func (bc *BlockChain) SetEthApi(ethAPI pairtypes.PairAPI) {
	bc.ethAPI = ethAPI
}

func selectRandomElements(slice []pairtypes.Triangle, count int) []pairtypes.Triangle {
	r := rand.New(rand.NewSource(time.Now().UnixNano()))
	selected := make([]pairtypes.Triangle, count)
	for i := 0; i < count; i++ {
		selected[i] = slice[r.Intn(len(slice))]
	}
	return selected
}
