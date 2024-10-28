package core

import (
	"encoding/hex"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/log"
	"github.com/ethereum/go-ethereum/paircache"
	"github.com/ethereum/go-ethereum/paircache/pairtypes"
	"time"
)

func pairCall(receipts types.Receipts, blockTime *time.Time, blockNumber uint64) {
	// 判断三角开关开启
	if paircache.PairCallSwitch {
		pairCache := paircache.GetPairCache()
		pairOccurTimes := 0
		pairTriangleIdSetMap := make(map[string]*pairtypes.Set)

		// 根据receipts交易收据获取pair及其对应的trangleid集合
		for _, receipt := range receipts {
			for _, reLog := range receipt.Logs {
				// marshalLog, _ := json.Marshal(reLog)
				// log.Info("收据日志打印", "blockNumber", blockNumber, "logBlockNum", reLog.BlockNumber, "区块对应的收据receipt.Logs", string(marshalLog))
				topics := reLog.Topics
				if len(topics) > 0 {
					topic0Str := "0x" + hex.EncodeToString(topics[0][:])
					topicOper := pairCache.TopicMap[topic0Str]
					if topicOper != "" {
						var address string
						if topicOper == "Balancer" {
							address = "0x" + hex.EncodeToString(topics[1][0:20])
						} else {
							address = "0x" + hex.EncodeToString(reLog.Address[:])
						}
						pairOccurTimes++
						address = common.HexToAddress(address).Hex()
						pairTriangleIdSet := pairCache.GetPairTriangleIdSet(address)
						pairTriangleIdSetMap[address] = pairTriangleIdSet
						// log.Info("交易收据日志打印", "blockNumber", blockNumber, "logBlockNum", reLog.BlockNumber, "Log.Index", reLog.Index, "topic", topic0Str, "topicOper", topicOper, "address", address, "pairTriangleIdSet", pairTriangleIdSet.GetData().Keys())
					}
				}
			}
		}
		log.Info("获取pair统计信息成功", "blockNumber", blockNumber, "pairTriangleIdSetNum", len(pairTriangleIdSetMap), "addrOccurTimes", pairOccurTimes)

		// 过滤获取triangles
		var triangles []pairtypes.Triangle
		filterMap := make(map[string]bool)
		// 循环获取每个pair的triangleIdSet
		for _, triangleIdSet := range pairTriangleIdSetMap {
			// 不同的pair可能具有同一个triangleId，所以再循环每组triangleIdSet时去重
			for _, triangleId := range triangleIdSet.GetData().Keys() {
				if filterMap[triangleId] {
					continue
				}
				// 查询triangle
				if triangle, exists := pairCache.GetTriangle(triangleId); exists {
					triangles = append(triangles, triangle)
					filterMap[triangleId] = true
				}
			}
		}

		// log.Info("获取triangles信息成功", "triangles", triangles)
		lenth := len(triangles)
		if lenth > 0 {
			if lenth <= paircache.TrianglefilterNum {
				select {
				case paircache.TriangleChannel <- &pairtypes.TransferTriangle{BlockNumber: blockNumber, BlockTime: blockTime, Triangles: triangles}:
				default:
					log.Warn("通道已满，直接跳过")
				}
			} else {
				select {
				case paircache.TriangleChannel <- &pairtypes.TransferTriangle{BlockNumber: blockNumber, BlockTime: blockTime, Triangles: paircache.SelectRandomElements(triangles, paircache.TrianglefilterNum)}:
				default:
					log.Warn("通道已满，直接跳过")
				}
			}
			// 等待处理完成
			// <-paircache.DoneChannel
		}
	}
}
