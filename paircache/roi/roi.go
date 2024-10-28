package roi

import (
	"bytes"
	"encoding/json"
	"github.com/ethereum/go-ethereum/log"
	"github.com/ethereum/go-ethereum/paircache"
	"math/big"
	"net/http"
)

type ROI struct {
	ChainId     int64   `json:"chainId"`
	Type        string  `json:"type"`
	ClusterId   int64   `json:"clusterId"`
	EntityId    int64   `json:"entityId"`
	BlockNumber uint64  `json:"blockNumber"`
	CallData    string  `json:"callData"`
	Profit      big.Int `json:"profit"`
	GasUsed     uint64  `json:"gasUsed"`
	To          string  `json:"to"`
}

func SendRois(rois []ROI) {
	// 将数据序列化为JSON
	jsonData, err := json.Marshal(rois)
	if err != nil {
		log.Error("解析rois为json失败", "err", err)
		return
	}
	log.Info("解析rois为json成功", "jsonData", string(jsonData))

	// 发送HTTP POST请求
	resp, err := http.Post(paircache.MevServiceUrl, "application/json", bytes.NewBuffer(jsonData))
	if err != nil {
		log.Error("发送rois到MevServiceUrl异常", "err", err)
		return
	}
	defer resp.Body.Close()

	// 检查响应状态
	if resp.StatusCode == http.StatusOK {
		log.Info("发送rois到MevServiceUrl成功", "statusCode", resp.StatusCode)
	} else {
		log.Info("发送rois到MevServiceUrl失败", "statusCode", resp.StatusCode)
	}

}
