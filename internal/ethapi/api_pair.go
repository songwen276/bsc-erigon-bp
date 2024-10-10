package ethapi

import (
	"bufio"
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/gopool"
	"github.com/ethereum/go-ethereum/paircache"
	"github.com/ethereum/go-ethereum/paircache/pairtypes"
	solsha3 "github.com/miguelmota/go-solidity-sha3"
	"math/big"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/core"
	"github.com/ethereum/go-ethereum/log"
	"github.com/ethereum/go-ethereum/rpc"
)

var LatestBlockNumber = rpc.BlockNumberOrHashWithNumber(rpc.LatestBlockNumber)

type Wei struct {
	BitSize int
	Data    string
}

type ROI struct {
	Triangle pairtypes.Triangle
	CallData string
	Profit   big.Int
}

type ArbitrageQueryParam struct {
	Start  *big.Int
	End    *big.Int
	Pieces *big.Int
}

type CallBatchArgs struct {
	Args           TransactionArgs
	BlockNrOrHash  *rpc.BlockNumberOrHash
	Overrides      *StateOverride
	BlockOverrides *BlockOverrides
}

type Results struct {
	GetDatasSince time.Duration          `json:"getDatasSince"`
	SelectSince   time.Duration          `json:"selectSince"`
	TotalSince    time.Duration          `json:"totalSince"`
	ResultMap     map[string]interface{} `json:"resultMap"`
}

func EncodePackedBsc(values []interface{}) (string, error) {
	var encoded string
	for _, value := range values {
		switch v := value.(type) {
		case string:
			encoded = encoded + v
		case *Wei:
			wei := *v
			encoded = encoded + wei.Data[len(wei.Data)-wei.BitSize/4:]
		case common.Address:
			addrStr := v.Hex()[2:]
			encoded = encoded + addrStr
		default:
			return "", fmt.Errorf("unsupported type: %T", value)
		}
	}
	return encoded, nil
}

func getWei(roi *big.Int, bitSize int) *Wei {
	return &Wei{
		BitSize: bitSize,
		Data:    hex.EncodeToString(solsha3.Int256(roi)),
	}
}

func getRoisDirect(s *BlockChainAPI, triangular *pairtypes.ITriangularArbitrageTriangular, param *ArbitrageQueryParam, ctx context.Context) (hexutil.Bytes, error) {
	data, _ := paircache.Encoder("arbitrageQuery", triangular, param.Start, param.End, param.Pieces)
	bytes := hexutil.Bytes(data)
	args := TransactionArgs{From: &paircache.From, To: &paircache.To, Data: &bytes}
	return s.FlagCall(ctx, args, &LatestBlockNumber, nil, nil)
}

func getRoisTest(s *BlockChainAPI, triangular *pairtypes.ITriangularArbitrageTriangular, param *ArbitrageQueryParam, ctx context.Context) ([]*big.Int, error) {
	data, _ := paircache.Encoder("arbitrageQuery", triangular, param.Start, param.End, param.Pieces)
	bytes := hexutil.Bytes(data)
	args := TransactionArgs{From: &paircache.From, To: &paircache.To, Data: &bytes}
	call, err := s.FlagCall(ctx, args, &LatestBlockNumber, nil, nil)
	if err != nil {
		return nil, err
	} else {
		roiStr := hex.EncodeToString(call)
		lenth := len(roiStr) / 64
		rois := make([]*big.Int, lenth-2)
		for j := 0; j < lenth; j++ {
			subStr := roiStr[64*j : 64*(j+1)]
			log.Info("CallReturn EncodeToString", "roiStr", subStr)
			if j > 1 {
				roi, _ := new(big.Int).SetString(subStr, 16)
				rois[j-2] = roi
			}
		}
		return rois, err
	}
}

func getRois(s *BlockChainAPI, triangular *pairtypes.ITriangularArbitrageTriangular, param *ArbitrageQueryParam, ctx context.Context) ([]*big.Int, error) {
	data, _ := paircache.Encoder("arbitrageQuery", triangular, param.Start, param.End, param.Pieces)
	bytes := hexutil.Bytes(data)
	args := TransactionArgs{From: &paircache.From, To: &paircache.To, Data: &bytes}
	call, err := s.FlagCall(ctx, args, &LatestBlockNumber, nil, nil)
	if err != nil {
		return nil, err
	} else {
		roiStr := hex.EncodeToString(call)
		lenth := len(roiStr) / 64
		rois := make([]*big.Int, lenth-2)
		for j := 0; j < lenth; j++ {
			if j > 1 {
				roi, _ := new(big.Int).SetString(roiStr[64*j:64*(j+1)], 16)
				rois[j-2] = roi
			}
		}
		return rois, err
	}
}

func getArbitrageQueryParam(start *big.Int, index, step int) *ArbitrageQueryParam {
	if index >= 10 {
		index = 9
	}
	// 计算 startNew = start + step * index
	stepBigInt := big.NewInt(int64(step))
	indexBigInt := big.NewInt(int64(index))
	startNew := new(big.Int).Add(start, new(big.Int).Mul(stepBigInt, indexBigInt))

	// 计算 end = startNew + step
	end := new(big.Int).Add(startNew, stepBigInt)

	// 返回查询参数
	return &ArbitrageQueryParam{
		Start:  startNew,
		End:    end,
		Pieces: big.NewInt(10), // 相当于 BigInteger.TEN
	}
}

func resolveROI(rois []*big.Int) int {
	var i int
	// 排除rois前6个元素，剩下元素每8个一组，循环每组中首元素判断是否为0
	for i = 0; i < (len(rois)-6)/8; i++ {
		if rois[i*8+6].Cmp(big.NewInt(0)) == 0 {
			return i
		}
	}
	return i
}

func directResolveIndex(s *BlockChainAPI, triangular *pairtypes.ITriangularArbitrageTriangular, param *ArbitrageQueryParam, ctx context.Context) (int, error) {
	data, _ := paircache.Encoder("arbitrageQuery", triangular, param.Start, param.End, param.Pieces)
	bytes := hexutil.Bytes(data)
	args := TransactionArgs{From: &paircache.From, To: &paircache.To, Data: &bytes}
	call, err := s.FlagCall(ctx, args, &LatestBlockNumber, nil, nil)
	var i int
	if err != nil {
		return i, err
	}

	// 截取掉前8个长度为32个字节的元素，获取roi利润字节部分数据，同样这些数据每32个字节长度代表一个元素，并将元素每8个分成一组（正常数据能得到10组数据，每组索引0-9）
	roiCall := call[32*8:]
	lenth := len(roiCall) / 32 / 8

	// 从第一组开始循环，将组内首个字节元素转换为big.int类型，判断其值是否等于0，等于0代表无利润了，返回该组的索引
	for i = 0; i < lenth; i++ {
		if new(big.Int).SetBytes(roiCall[i*8*32:i*8*32+32]).Cmp(big.NewInt(0)) == 0 {
			return i, nil
		}
	}
	return i, nil
}

func SubmitTestCall(wg *sync.WaitGroup, s *BlockChainAPI, results chan interface{}, triangle *pairtypes.Triangle) {
	t := *triangle
	gopool.Submit(func() {
		defer wg.Done()
		workerTest(s, results, t)
	})
}

func SubmitCall(wg *sync.WaitGroup, s *BlockChainAPI, results chan interface{}, triangle *pairtypes.Triangle) {
	t := *triangle
	gopool.Submit(func() {
		defer wg.Done()
		pairWorker(s, results, t)
	})
}

func FlagDoCall(ctx context.Context, b Backend, args TransactionArgs, blockNrOrHash rpc.BlockNumberOrHash, overrides *StateOverride, blockOverrides *BlockOverrides, timeout time.Duration, globalGasCap uint64) (*core.ExecutionResult, error) {
	defer func(start time.Time) { log.Debug("Executing EVM call finished", "runtime", time.Since(start)) }(time.Now())

	state, header, err := b.StateAndHeaderByNumberOrHash(ctx, blockNrOrHash)
	if state == nil || err != nil {
		return nil, err
	}
	state.Flag = 1

	return doCall(ctx, b, args, state, header, overrides, blockOverrides, timeout, globalGasCap)
}

func (s *BlockChainAPI) FlagCall(ctx context.Context, args TransactionArgs, blockNrOrHash *rpc.BlockNumberOrHash, overrides *StateOverride, blockOverrides *BlockOverrides) (hexutil.Bytes, error) {
	if blockNrOrHash == nil {
		latest := rpc.BlockNumberOrHashWithNumber(rpc.LatestBlockNumber)
		blockNrOrHash = &latest
	}
	result, err := FlagDoCall(ctx, s.b, args, *blockNrOrHash, overrides, blockOverrides, s.b.RPCEVMTimeout(), s.b.RPCGasCap())
	if err != nil {
		return nil, err
	}
	// If the result contains a revert reason, try to unpack and return it.
	if len(result.Revert()) > 0 {
		return nil, newRevertError(result.Revert())
	}
	return result.Return(), result.Err
}

func workerDirect(s *BlockChainAPI, results chan<- interface{}, triangle pairtypes.Triangle) {
	// 设置上下文，用于控制每个任务方法执行超时时间
	ctx := context.Background()
	triangular := &pairtypes.ITriangularArbitrageTriangular{
		Token0:  common.HexToAddress(triangle.Token0),
		Router0: common.HexToAddress(triangle.Router0),
		Pair0:   common.HexToAddress(triangle.Pair0),
		Token1:  common.HexToAddress(triangle.Token1),
		Router1: common.HexToAddress(triangle.Router1),
		Pair1:   common.HexToAddress(triangle.Pair1),
		Token2:  common.HexToAddress(triangle.Token2),
		Router2: common.HexToAddress(triangle.Router2),
		Pair2:   common.HexToAddress(triangle.Pair2),
	}

	param := getArbitrageQueryParam(big.NewInt(0), 0, 10000)
	index, err := directResolveIndex(s, triangular, param, ctx)
	if err != nil {
		results <- err
		return
	}
	log.Info("10000step", "start", param.Start, "end", param.End, "step", param.Pieces, "index", index)

	param = getArbitrageQueryParam(param.Start, index, 1000)
	index, err = directResolveIndex(s, triangular, param, ctx)
	if err != nil {
		results <- err
		return
	}
	log.Info("1000step", "start", param.Start, "end", param.End, "step", param.Pieces, "index", index)

	param = getArbitrageQueryParam(param.Start, index, 100)
	index, err = directResolveIndex(s, triangular, param, ctx)
	if err != nil {
		results <- err
		return
	}
	log.Info("100step", "start", param.Start, "end", param.End, "step", param.Pieces, "index", index)

	param = getArbitrageQueryParam(param.Start, index, 10)
	index, err = directResolveIndex(s, triangular, param, ctx)
	if err != nil {
		results <- err
		return
	}
	log.Info("10step", "start", param.Start, "end", param.End, "step", param.Pieces, "index", index)

	point := new(big.Int).Add(param.Start, big.NewInt(int64(index)))
	if point.Cmp(big.NewInt(0)) == 0 {
		results <- nil
		return
	}
	param.Start = point
	param.End = point
	param.Pieces = big.NewInt(1)

	call, err := getRoisDirect(s, triangular, param, ctx)
	if err != nil {
		results <- err
		return
	}
	roisBytes := call[32*2:]
	roisStr := hex.EncodeToString(roisBytes)
	var rois []string
	for i := 0; i < len(roisStr)/64; i++ {
		rois[i] = roisStr[i*64 : (i+1)*64]
	}

	roi13 := new(big.Int).SetBytes(roisBytes[32*12 : 32*13])
	if call == nil || roi13.Cmp(big.NewInt(5000000)) < 0 {
		results <- nil
		return
	}

	snapshotsHash := solsha3.SoliditySHA3(rois[3], rois[4], rois[5])
	subHex := hex.EncodeToString(snapshotsHash)[0:2]

	parameters := []interface{}{
		hex.EncodeToString(solsha3.Uint32(big.NewInt(0))),
		subHex,
		rois[0][24:],
		rois[6][40:],
		rois[1][24:],
		rois[7][40:],
		rois[2][24],
		rois[10][40:],
		triangular.Token0,
		rois[11][40:],
		triangular.Pair0,
		rois[12][40:],
		triangular.Token1,
		rois[13][40:],
		triangular.Pair1,
		triangular.Token2,
		triangular.Pair2,
	}

	calldata, err := EncodePackedBsc(parameters)
	if err != nil {
		results <- err
		return
	}

	ROI := &ROI{
		Triangle: triangle,
		CallData: calldata,
		Profit:   *roi13,
	}

	results <- ROI
	return
}

func workerTest(s *BlockChainAPI, results chan<- interface{}, triangle pairtypes.Triangle) {
	// 设置上下文，用于控制每个任务方法执行超时时间
	ctx := context.Background()
	triangular := &pairtypes.ITriangularArbitrageTriangular{
		Token0:  common.HexToAddress(triangle.Token0),
		Router0: common.HexToAddress(triangle.Router0),
		Pair0:   common.HexToAddress(triangle.Pair0),
		Token1:  common.HexToAddress(triangle.Token1),
		Router1: common.HexToAddress(triangle.Router1),
		Pair1:   common.HexToAddress(triangle.Pair1),
		Token2:  common.HexToAddress(triangle.Token2),
		Router2: common.HexToAddress(triangle.Router2),
		Pair2:   common.HexToAddress(triangle.Pair2),
	}

	param := getArbitrageQueryParam(big.NewInt(0), 0, 10000)
	rois, err := getRoisTest(s, triangular, param, ctx)
	log.Info("10000step", "start", param.Start, "end", param.End, "step", param.Pieces, "rois", rois)
	if err != nil {
		results <- err
		return
	}

	index := resolveROI(rois)
	param = getArbitrageQueryParam(param.Start, index, 1000)
	rois, err = getRoisTest(s, triangular, param, ctx)
	log.Info("1000step", "start", param.Start, "end", param.End, "step", param.Pieces, "rois", rois)
	if err != nil {
		results <- err
		return
	}
	index = resolveROI(rois)

	param = getArbitrageQueryParam(param.Start, index, 100)
	rois, err = getRoisTest(s, triangular, param, ctx)
	log.Info("100step", "start", param.Start, "end", param.End, "step", param.Pieces, "rois", rois)
	if err != nil {
		results <- err
		return
	}
	index = resolveROI(rois)

	param = getArbitrageQueryParam(param.Start, index, 10)
	rois, err = getRoisTest(s, triangular, param, ctx)
	log.Info("10step", "start", param.Start, "end", param.End, "step", param.Pieces, "rois", rois)
	if err != nil {
		results <- err
		return
	}
	index = resolveROI(rois)
	point := new(big.Int).Add(param.Start, big.NewInt(int64(index)))
	if point.Cmp(big.NewInt(0)) == 0 {
		results <- nil
		return
	}
	param.Start = point
	param.End = point
	param.Pieces = big.NewInt(1)

	rois, err = getRoisTest(s, triangular, param, ctx)
	log.Info("point", "start", param.Start, "end", param.End, "step", param.Pieces, "rois", rois)
	if err != nil {
		results <- err
		return
	}

	if rois == nil || rois[13] == nil || rois[13].Cmp(big.NewInt(5000000)) < 0 {
		results <- nil
		return
	}

	snapshotsHash := solsha3.SoliditySHA3(solsha3.Int256(rois[3]), solsha3.Int256(rois[4]), solsha3.Int256(rois[5]))
	subHex := hex.EncodeToString(snapshotsHash)[0:2]

	parameters := []interface{}{
		hex.EncodeToString(solsha3.Uint32(big.NewInt(0))),
		subHex,
		common.BigToAddress(rois[0]),
		getWei(rois[6], 96),
		common.BigToAddress(rois[1]),
		getWei(rois[7], 96),
		common.BigToAddress(rois[2]),
		getWei(rois[10], 96),
		triangular.Token0,
		getWei(rois[11], 96),
		triangular.Pair0,
		getWei(rois[12], 96),
		triangular.Token1,
		getWei(rois[13], 96),
		triangular.Pair1,
		triangular.Token2,
		triangular.Pair2,
	}

	calldata, err := EncodePackedBsc(parameters)
	if err != nil {
		results <- err
		return
	}

	ROI := &ROI{
		Triangle: triangle,
		CallData: calldata,
		Profit:   *rois[13],
	}

	results <- ROI
	return
}

func pairWorker(s *BlockChainAPI, results chan<- interface{}, triangle pairtypes.Triangle) {
	// 设置上下文，用于控制每个任务方法执行超时时间
	ctx := context.Background()
	triangular := &pairtypes.ITriangularArbitrageTriangular{
		Token0:  common.HexToAddress(triangle.Token0),
		Router0: common.HexToAddress(triangle.Router0),
		Pair0:   common.HexToAddress(triangle.Pair0),
		Token1:  common.HexToAddress(triangle.Token1),
		Router1: common.HexToAddress(triangle.Router1),
		Pair1:   common.HexToAddress(triangle.Pair1),
		Token2:  common.HexToAddress(triangle.Token2),
		Router2: common.HexToAddress(triangle.Router2),
		Pair2:   common.HexToAddress(triangle.Pair2),
	}

	param := getArbitrageQueryParam(big.NewInt(0), 0, 10000)
	rois, err := getRois(s, triangular, param, ctx)
	if err != nil {
		results <- err
		return
	}

	index := resolveROI(rois)
	param = getArbitrageQueryParam(param.Start, index, 1000)
	rois, err = getRois(s, triangular, param, ctx)
	if err != nil {
		results <- err
		return
	}
	index = resolveROI(rois)

	param = getArbitrageQueryParam(param.Start, index, 100)
	rois, err = getRois(s, triangular, param, ctx)
	if err != nil {
		results <- err
		return
	}
	index = resolveROI(rois)

	param = getArbitrageQueryParam(param.Start, index, 10)
	rois, err = getRois(s, triangular, param, ctx)
	if err != nil {
		results <- err
		return
	}
	index = resolveROI(rois)
	point := new(big.Int).Add(param.Start, big.NewInt(int64(index)))
	if point.Cmp(big.NewInt(0)) == 0 {
		results <- nil
		return
	}
	param.Start = point
	param.End = point
	param.Pieces = big.NewInt(1)

	rois, err = getRois(s, triangular, param, ctx)
	if err != nil {
		results <- err
		return
	}

	if rois == nil || rois[13] == nil || rois[13].Cmp(big.NewInt(5000000)) < 0 {
		results <- nil
		return
	}

	snapshotsHash := solsha3.SoliditySHA3(solsha3.Int256(rois[3]), solsha3.Int256(rois[4]), solsha3.Int256(rois[5]))
	subHex := hex.EncodeToString(snapshotsHash)[0:2]

	parameters := []interface{}{
		hex.EncodeToString(solsha3.Uint32(big.NewInt(0))),
		subHex,
		common.BigToAddress(rois[0]),
		getWei(rois[6], 96),
		common.BigToAddress(rois[1]),
		getWei(rois[7], 96),
		common.BigToAddress(rois[2]),
		getWei(rois[10], 96),
		triangular.Token0,
		getWei(rois[11], 96),
		triangular.Pair0,
		getWei(rois[12], 96),
		triangular.Token1,
		getWei(rois[13], 96),
		triangular.Pair1,
		triangular.Token2,
		triangular.Pair2,
	}

	calldata, err := EncodePackedBsc(parameters)
	if err != nil {
		results <- err
		return
	}

	ROI := &ROI{
		Triangle: triangle,
		CallData: calldata,
		Profit:   *rois[13],
	}

	results <- ROI
	return
}

func (s *BlockChainAPI) CallBatch() (string, error) {
	// 读取任务测试数据
	log.Info("开始执行CallBatch")
	var triangles []*pairtypes.Triangle
	oriTriangular := &pairtypes.Triangle{
		ID:      1,
		Token0:  "0xeBBAefF6217d22E7744394061D874015709b8141",
		Router0: "0x0BFbCF9fa4f9C56B0F40a671Ad40E0805A091865",
		Pair0:   "0x170a4d2A29b30c6551f6a4C0CB527e7A9Cb7D526",
		Token1:  "0xbb4CdB9CBd36B01bD1cBaEBF2De08d9173bc095c",
		Router1: "0xdB1d10011AD0Ff90774D0C6Bb92e5C5c8b4461F7",
		Pair1:   "0xCB99FE720124129520f7a09Ca3CBEF78D58Ed934",
		Token2:  "0xe9e7CEA3DedcA5984780Bafc599bD69ADd087D56",
		Router2: "0x10ED43C718714eb63d5aA57B78B54704E256024E",
		Pair2:   "0xc1fE0336456a8D4550ab0E1e528a684Bcf7bD3F8",
	}
	triangles = append(triangles, oriTriangular)

	// 初始化构造当前区块公共数据
	start := time.Now()
	results := make(chan interface{}, len(triangles))

	// 提交任务到协程池，所有协程完成后关闭结果读取通道
	var wg sync.WaitGroup
	for _, triangle := range triangles {
		wg.Add(1)
		SubmitTestCall(&wg, s, results, triangle)
	}
	wg.Wait()
	close(results)
	selectSince := time.Since(start)
	log.Info("所有eth_call查询任务执行完成花费时长", "runtime", selectSince, "所在的区块号", s.BlockNumber())

	// 读取任务结果通道数据进行处理
	rois := make([]ROI, 0, 5000)
	resultMap := make(map[string]interface{}, len(triangles))
	i := 1
	// 处理结果
	for result := range results {
		itoa := strconv.Itoa(i)
		switch v := result.(type) {
		case *ROI:
			rois = append(rois, *v)
			resultMap[itoa] = *v
		case error:
			resultMap[itoa] = v.Error()
		default:
			resultMap[itoa] = v
		}
		i += 1
	}

	if len(rois) > 0 {
		// 按 Profit 字段对rois进行降序排序
		log.Info("排序前的rois", "rois", rois)
		sort.Slice(rois, func(i, j int) bool {
			return rois[i].Profit.Cmp(&rois[j].Profit) > 0
		})
		log.Info("降序排序rois成功", "rois", rois)

		// 将排序后的rois去重过滤，保证每个pair只能出现一次，重复时将Profit较小的ROI都删除，只保留Profit最大的ROI
		// 去重，保证 Pair0, Pair1, Pair2 中的值只出现一次
		uniquePairs := make(map[string]bool)
		var filteredROIs []ROI
		for _, roi := range rois {
			if uniquePairs[roi.Triangle.Pair0] || uniquePairs[roi.Triangle.Pair1] || uniquePairs[roi.Triangle.Pair2] {
				// 如果任何一个 pair 已经出现过，跳过该结构体（删除）
				continue
			}

			// 如果不存在，则将该结构体加入结果集，并标记 pairs 为已出现
			filteredROIs = append(filteredROIs, roi)
			uniquePairs[roi.Triangle.Pair0] = true
			uniquePairs[roi.Triangle.Pair1] = true
			uniquePairs[roi.Triangle.Pair2] = true
		}
		log.Info("排序去重获rois成功", "filteredROIs", filteredROIs)

		// 计算预估总gas
		var gasTotal hexutil.Uint64
		for _, filteredROI := range filteredROIs {
			decodeString, _ := hex.DecodeString(filteredROI.CallData)
			bytes := hexutil.Bytes(decodeString)
			args := TransactionArgs{From: &paircache.From, To: &paircache.To, Data: &bytes}
			gas, err := s.EstimateGas(context.Background(), args, &LatestBlockNumber, nil)
			if err != nil {
				log.Error("存在roi的预估gas计算异常", "err", err)
			}
			gasTotal = gasTotal + gas
		}
		log.Info("计算预估总gas成功", "gasTotal", gasTotal)
	}

	totalSince := time.Since(start)
	r := Results{GetDatasSince: 0, SelectSince: selectSince, TotalSince: totalSince, ResultMap: resultMap}

	// 创建文件
	file, err := os.Create("/bc/bsc/build/bin/results.json")
	if err != nil {
		return "", err
	}
	defer file.Close()

	// 将 map 编码为 JSON
	encoder := json.NewEncoder(file)
	encoder.SetIndent("", "  ") // 设置缩进格式
	if err := encoder.Encode(r); err != nil {
		return "", err
	}
	log.Info("结果输出到文件完成，结束")
	return "ok", nil
}

// PairCallBatch executes Call
func (s *BlockChainAPI) PairCallBatch(triangles []pairtypes.Triangle) error {
	// 初始化构造当前区块公共数据
	start := time.Now()
	log.Info("开始执行PairCallBatch")
	results := make(chan interface{}, len(triangles))

	// 提交任务到协程池，所有协程完成后关闭结果读取通道
	var wg sync.WaitGroup
	for _, triangle := range triangles {
		wg.Add(1)
		SubmitCall(&wg, s, results, &triangle)
	}
	wg.Wait()
	close(results)
	selectSince := time.Since(start)
	log.Info("所有eth_call查询任务执行完成花费时长", "runtime", selectSince, "所在的区块号", s.BlockNumber())

	// 读取任务结果通道数据进行处理
	rois := make([]ROI, 0, 5000)
	resultMap := make(map[string]interface{}, len(triangles))
	i := 1
	// 处理结果
	for result := range results {
		itoa := strconv.Itoa(i)
		switch v := result.(type) {
		case *ROI:
			rois = append(rois, *v)
		case error:
			resultMap[itoa] = v.Error()
		default:
			resultMap[itoa] = v
		}
		i += 1
	}

	if len(rois) > 0 {
		// 按 Profit 字段对rois进行降序排序
		log.Info("排序前的rois", "rois", rois)
		sort.Slice(rois, func(i, j int) bool {
			return rois[i].Profit.Cmp(&rois[j].Profit) > 0
		})
		log.Info("降序排序rois成功", "rois", rois)

		// 将排序后的rois去重过滤，保证每个pair只能出现一次，重复时将Profit较小的ROI都删除，只保留Profit最大的ROI
		// 去重，保证 Pair0, Pair1, Pair2 中的值只出现一次
		uniquePairs := make(map[string]bool)
		var filteredROIs []ROI
		for _, roi := range rois {
			if uniquePairs[roi.Triangle.Pair0] || uniquePairs[roi.Triangle.Pair1] || uniquePairs[roi.Triangle.Pair2] {
				// 如果任何一个 pair 已经出现过，跳过该结构体（删除）
				continue
			}

			// 如果不存在，则将该结构体加入结果集，并标记 pairs 为已出现
			filteredROIs = append(filteredROIs, roi)
			uniquePairs[roi.Triangle.Pair0] = true
			uniquePairs[roi.Triangle.Pair1] = true
			uniquePairs[roi.Triangle.Pair2] = true
		}
		log.Info("排序去重获rois成功", "filteredROIs", filteredROIs)

		// 计算预估总gas
		var gasTotal hexutil.Uint64
		for _, filteredROI := range filteredROIs {
			decodeString, _ := hex.DecodeString(filteredROI.CallData)
			bytes := hexutil.Bytes(decodeString)
			args := TransactionArgs{From: &paircache.From, To: &paircache.To, Data: &bytes}
			gas, err := s.EstimateGas(context.Background(), args, &LatestBlockNumber, nil)
			if err != nil {
				log.Error("存在roi的预估gas计算异常", "err", err)
			}
			gasTotal = gasTotal + gas
		}
		log.Info("计算预估总gas成功", "gasTotal", gasTotal)
	}

	totalSince := time.Since(start)
	log.Info("处理结果完成", "共耗时", totalSince)

	return nil
}

func GetEthCallData() ([]CallBatchArgs, error) {
	// 打开测试数据文件
	file, err := os.Open("/bc/bsc/build/bin/testdata.json")
	if err != nil {
		fmt.Println("Error opening file:", err)
		return nil, err
	}
	defer file.Close()

	// 创建一个缓冲读取器
	scanner := bufio.NewScanner(file)

	datas := make([]CallBatchArgs, 0, 10000)
	for scanner.Scan() {
		line := scanner.Text()
		batchArgs := CallBatchArgs{Overrides: nil, BlockOverrides: nil}
		// 从目标字符串之后开始提取内容
		index1 := strings.Index(line, "\"params\":[")
		if index1 != -1 {
			// 提取目标字符串之后的内容
			param1 := line[index1+len("\"params\":[") : len(line)-12]
			err := json.Unmarshal([]byte(param1), &batchArgs.Args)
			if err != nil {
				return nil, err
			}
		}
		index2 := strings.Index(line, "},\"")
		if index2 != -1 {
			// 提取目标字符串之后的内容
			param2 := line[index2+len("},\"") : len(line)-4]
			var num rpc.BlockNumber
			num.UnmarshalJSON([]byte(param2))
			number := rpc.BlockNumberOrHashWithNumber(num)
			batchArgs.BlockNrOrHash = &number
		}
		datas = append(datas, batchArgs)
	}
	return datas, nil
}

// CallBatch batch executes Call
// func (s *BlockChainAPI) CallBatch() (string, error) {
// 	// 读取任务测试数据
// 	log.Info("开始执行CallBatch")
// 	datas, err := GetEthCallData()
// 	if err != nil {
// 		return "", err
// 	}
// 	// getDatasSince := time.Since(start)
// 	// log.Info("获取所有测试数据花费时长", "runtime", getDatasSince)
//
// 	// 根据任务数创建结果读取通道
// 	results := make(chan interface{}, len(datas))
//
// 	// 提交任务到协程池，所有协程完成后关闭结果读取通道
// 	start := time.Now()
// 	var wg sync.WaitGroup
// 	for _, job := range datas {
// 		wg.Add(1)
// 		args := job.Args
// 		gopool.Submit(func() {
// 			defer wg.Done()
// 			worker(s, results, args, &pair.LatestBlockNumber)
// 		})
// 	}
// 	wg.Wait()
// 	close(results)
// 	selectSince := time.Since(start)
// 	log.Info("所有eth_call查询任务执行完成花费时长", "runtime", selectSince, "所在的区块号", s.BlockNumber())
//
// 	// 读取任务结果通道数据进行处理
// 	resultMap := make(map[string]interface{}, len(datas))
// 	i := 1
// 	// 处理结果
// 	for result := range results {
// 		itoa := strconv.Itoa(i)
// 		switch v := result.(type) {
// 		case hexutil.Bytes:
// 			bytes := result.(hexutil.Bytes)
// 			if err != nil {
// 				resultMap[itoa] = err.Error()
// 			} else {
// 				dateStr := hex.EncodeToString(bytes)
// 				resultMap["itoa"] = dateStr
// 				lenth := len(dateStr) / 64
// 				roi := make([]*big.Int, lenth-2)
// 				for j := 0; j < lenth; j++ {
// 					subDataStr := dateStr[64*j : 64*(j+1)-1]
// 					resultMap["itoabytes"+strconv.Itoa(j)] = subDataStr
// 					if j > 1 {
// 						setString, _ := new(big.Int).SetString(subDataStr, 16)
// 						roi[j-2] = setString
// 					}
// 				}
// 				log.Info("解析的roi成功", "roi", roi)
// 			}
// 		case error:
// 			resultMap[itoa] = v.Error()
// 		default:
// 			resultMap[itoa] = v
// 		}
// 		i += 1
// 	}
// 	totalSince := time.Since(start)
// 	r := Results{GetDatasSince: 0, SelectSince: selectSince, TotalSince: totalSince, ResultMap: resultMap}
//
// 	// 创建文件
// 	file, err := os.Create("/bc/bsc/build/bin/results.json")
// 	if err != nil {
// 		return "", err
// 	}
// 	defer file.Close()
//
// 	// 将 map 编码为 JSON
// 	encoder := json.NewEncoder(file)
// 	encoder.SetIndent("", "  ") // 设置缩进格式
// 	if err := encoder.Encode(r); err != nil {
// 		return "", err
// 	}
// 	log.Info("结果输出到文件完成，结束")
// 	return "ok", nil
// }
